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
	if *data == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return fmt.Errorf("find data dir: %w", err)
		}
		*data = filepath.Join(dir, "Reef")
	}

	st, err := store.Open(*data)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer st.Close()

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
	if err := srv.Shutdown(shutCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	return nil
}
