// Command reef runs the Reef server.
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
	_usage             = "usage: reef serve [--addr 127.0.0.1:7700] [--data DIR]"
	_defaultAddr       = "127.0.0.1:7700"
	_shutdownTimeout   = time.Second
	_readHeaderTimeout = 10 * time.Second
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Args[1:], os.Stderr)
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, "reef:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, logOut io.Writer) error {
	if len(args) == 0 || args[0] != "serve" {
		return errors.New(_usage)
	}
	fs := flag.NewFlagSet("reef serve", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	addr := fs.String("addr", _defaultAddr, "")
	data := fs.String("data", "", "")
	if err := fs.Parse(args[1:]); err != nil {
		return fmt.Errorf("%w\n%s", err, _usage)
	}
	if fs.NArg() != 0 {
		return errors.New(_usage)
	}
	if err := requireLoopback(*addr); err != nil {
		return err
	}
	if *data == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return fmt.Errorf("find data dir: %w", err)
		}
		*data = filepath.Join(dir, "Reef")
	}

	lock, err := lockDataDir(*data)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Close() }()

	st, err := store.Open(*data)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer func() { _ = st.Close() }()

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		return err
	}

	logger := log.New(logOut, "", log.LstdFlags)
	failed, err := st.FailRunningTurns(ctx)
	if err != nil {
		return fmt.Errorf("recover turns: %w", err)
	}
	if len(failed) > 0 {
		logger.Printf("marked %d running turns failed: server restarted", len(failed))
	}

	srv := &http.Server{Handler: server.New(st), ReadHeaderTimeout: _readHeaderTimeout}
	logger.Printf("listening on %s, data in %s", ln.Addr(), *data)

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

// lockDataDir takes the data folder's lock until the returned file is closed, so
// only one reef serve runs on a folder.
func lockDataDir(dir string) (*os.File, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	lock, err := os.OpenFile(filepath.Join(dir, "reef.lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = lock.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf("data folder %s is in use by another reef", dir)
		}
		return nil, fmt.Errorf("lock data folder: %w", err)
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
