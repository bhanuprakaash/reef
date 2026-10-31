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
	"github.com/bhanuprakaash/reef/space"
	"github.com/bhanuprakaash/reef/store"
	"github.com/bhanuprakaash/reef/web"
)

const (
	_usage = `usage: reef <command> [args] [--data DIR]
commands:
  serve [--addr 127.0.0.1:7700]
  project add [--name N]
  work-item add "<title>" [--type T] [--description D]
  brief <work-item>
  decide <work-item> "<body>" [--why W]
  memory add <kind> "<body>" --scope S [--as ROLE] [--about X]... [--replaces ID]... [--why W] [--needs-approval]
  memory recall "<q>" [--scope S] [--limit N]
  memory show <id> | retire <id> | approve <id> | reject <id> | pending
  memory embed [none|openai|voyage|gemini [--key K]]
  team apply FILE.json | team list
  context <work-item> --role R
  wrap list [<work-item>] | show <id> | approve <id> | reject <id>
  token <work-item> --role R
  chat <work-item> --backend claude|codex | chat --space ID
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
	case "work-item":
		return workItemCmd(ctx, args[1:], stdout)
	case "brief":
		return briefCmd(ctx, args[1:], stdout)
	case "decide":
		return decideCmd(ctx, args[1:], stdout, stderr)
	case "team":
		return teamCmd(ctx, args[1:], stdout)
	case "memory":
		return memoryCmd(ctx, args[1:], stdout, stderr)
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
	if data, err = filepath.Abs(data); err != nil {
		return fmt.Errorf("resolve data dir: %w", err)
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find reef path: %w", err)
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
	failed, err := st.FailRunningTurns(ctx, func(spaceID int64) bool {
		return space.InUse(data, spaceID)
	})
	if err != nil {
		return fmt.Errorf("recover turns: %w", err)
	}
	if len(failed) > 0 {
		logger.Printf("marked %d running turns failed: server restarted", len(failed))
	}
	leaving, err := st.LeavingMembers(ctx, 0)
	if err != nil {
		return fmt.Errorf("list pending removals: %w", err)
	}
	for _, p := range leaving {
		if err := st.ApplyMemberRemoval(ctx, p); err != nil {
			logger.Printf("remove Bot %d from Space %d: %v", p.BotID, p.SpaceID, err)
		}
	}
	if err := st.PauseWaitingExchangeSpaces(ctx); err != nil {
		return fmt.Errorf("pause Spaces with waiting exchanges: %w", err)
	}
	if err := st.ClearStaleActiveTurns(ctx, func(spaceID int64) bool {
		return space.InUse(data, spaceID)
	}); err != nil {
		return fmt.Errorf("clear active turns: %w", err)
	}

	ui, err := web.Dist()
	if err != nil {
		return fmt.Errorf("open web ui: %w", err)
	}
	ms, err := openMemory(ctx, st)
	if err != nil {
		return fmt.Errorf("open memory: %w", err)
	}
	api := server.New(st, ms, data, exe)
	defer api.Close()
	srv := &http.Server{
		Handler:           web.Handler(ui, api),
		ReadHeaderTimeout: _readHeaderTimeout,
	}
	logger.Printf("listening on %s, data in %s", ln.Addr(), data)

	served := make(chan error, 1)
	go func() { served <- srv.Serve(ln) }()
	select {
	case err := <-served:
		return err
	case <-ctx.Done():
	}
	api.Close()
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
