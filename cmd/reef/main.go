// Command reef runs the Reef server and its command line.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/bhanuprakaash/reef/server"
	"github.com/bhanuprakaash/reef/store"
)

const (
	_usage = `usage: reef <command> [args] [--data DIR]
commands:
  serve [--addr 127.0.0.1:7700]
  project add [--name N]
  feature add <slug> <name> [--goal G]
  brief <feature>
  decide <feature> "<body>" [--why W]
  task add [<feature>] "<title>"
  task list [<feature>] [--status S]
  context <feature> --role R
  wrap list [<feature>] | show <id> | approve <id> | reject <id>
  token <feature> --role R
  chat <feature> --role R --backend claude|codex | chat --channel ID
  mcp`
	_serveUsage        = "usage: reef serve [--addr 127.0.0.1:7700] [--data DIR]"
	_defaultAddr       = "127.0.0.1:7700"
	_shutdownTimeout   = time.Second
	_readHeaderTimeout = 10 * time.Second
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, "reef:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New(_usage)
	}
	switch args[0] {
	case "serve":
		return serve(ctx, args[1:], stderr)
	case "project":
		return projectCmd(ctx, args[1:], stdout)
	case "feature":
		return featureCmd(ctx, args[1:], stdout)
	case "brief":
		return briefCmd(ctx, args[1:], stdout)
	case "decide":
		return decideCmd(ctx, args[1:], stdout)
	case "task":
		return taskCmd(ctx, args[1:], stdout)
	case "context":
		return contextCmd(ctx, args[1:], stdout)
	case "wrap":
		return wrapCmd(ctx, args[1:], stdout)
	case "token":
		return tokenCmd(ctx, args[1:], stdout)
	case "chat":
		return chatCmd(ctx, args[1:], stdin, stdout, stderr)
	case "mcp":
		return mcpCmd(ctx, args[1:])
	}
	return errors.New(_usage)
}

func serve(ctx context.Context, args []string, logOut io.Writer) error {
	fs := flag.NewFlagSet("reef serve", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	addr := fs.String("addr", _defaultAddr, "")
	flagData := fs.String("data", "", "")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%w\n%s", err, _serveUsage)
	}
	if fs.NArg() != 0 {
		return errors.New(_serveUsage)
	}
	if err := requireLoopback(*addr); err != nil {
		return err
	}
	data, err := dataDir(*flagData)
	if err != nil {
		return err
	}

	lock, err := lockDataDir(data)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Close() }()

	st, err := store.Open(data)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer func() { _ = st.Close() }()

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		return err
	}

	logger := log.New(logOut, "", log.LstdFlags)
	failed, err := st.FailRunningTurns(ctx, func(channelID int64) bool {
		return chatOpen(data, channelID)
	})
	if err != nil {
		return fmt.Errorf("recover turns: %w", err)
	}
	if len(failed) > 0 {
		logger.Printf("marked %d running turns failed: server restarted", len(failed))
	}

	srv := &http.Server{Handler: server.New(st), ReadHeaderTimeout: _readHeaderTimeout}
	logger.Printf("listening on %s, data in %s", ln.Addr(), data)

	served := make(chan error, 1)
	go func() { served <- srv.Serve(ln) }()
	select {
	case err := <-served:
		return err
	case <-ctx.Done():
	}
	shutCtx, cancel := context.WithTimeout(context.Background(), _shutdownTimeout)
	defer cancel()
	shutErr := srv.Shutdown(shutCtx)
	if shutErr != nil {
		_ = srv.Close()
	}
	if err := <-served; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	if shutErr != nil {
		return fmt.Errorf("shutdown: %w", shutErr)
	}
	return nil
}

func dataDir(flagValue string) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find data dir: %w", err)
	}
	return filepath.Join(dir, "Reef"), nil
}

func lockDataDir(dir string) (*os.File, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	return flock(filepath.Join(dir, "reef.lock"),
		fmt.Errorf("data folder %s is in use by another reef", dir))
}

// flock takes an exclusive lock on the file at path until the returned file is
// closed. It returns busy when another process holds the lock.
func flock(path string, busy error) (*os.File, error) {
	lock, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = lock.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, busy
		}
		return nil, fmt.Errorf("lock %s: %w", path, err)
	}
	return lock, nil
}

// requireLoopback refuses an addr that other machines could reach, since the
// API has no login. An empty host binds every interface.
func requireLoopback(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("bad --addr %q: %w", addr, err)
	}
	if host == "localhost" {
		return nil
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("--addr %q is not a loopback address; use localhost, 127.0.0.1 or [::1]",
			addr)
	}
	return nil
}
