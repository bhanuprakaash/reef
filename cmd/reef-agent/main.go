// Command reef-agent runs one agent backend and prints its Events as JSON lines.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/agent/claude"
	"github.com/bhanuprakaash/reef/agent/codex"
)

const _usage = `usage: reef-agent --backend claude|codex [--cwd DIR] [--resume ID]
  [--permission-mode MODE] [--mcp FILE] "prompt"`

var errInterrupted = errors.New("interrupted")

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Args[1:], os.Stdin, os.Stdout, newAgent)
	stop()
	switch {
	case err == nil:
	case errors.Is(err, errInterrupted):
		os.Exit(130) // 128 + SIGINT, the shell convention
	default:
		fmt.Fprintln(os.Stderr, "reef-agent:", err)
		os.Exit(1)
	}
}

func newAgent(backend string) (agent.Agent, error) {
	switch backend {
	case "claude":
		return claude.New(), nil
	case "codex":
		return codex.New(), nil
	default:
		return nil, fmt.Errorf("unknown backend %q", backend)
	}
}

func run(
	ctx context.Context,
	args []string,
	stdin io.Reader,
	stdout io.Writer,
	newAgent func(backend string) (agent.Agent, error),
) error {
	fs := flag.NewFlagSet("reef-agent", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	backend := fs.String("backend", "", "")
	cwd := fs.String("cwd", "", "")
	resume := fs.String("resume", "", "")
	permissionMode := fs.String("permission-mode", "", "")
	mcpFile := fs.String("mcp", "", "")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%w\n%s", err, _usage)
	}
	if fs.NArg() != 1 {
		return errors.New(_usage)
	}
	if *cwd == "" {
		wd, err := os.Getwd()
		if err != nil {
			return err
		}
		*cwd = wd
	}

	a, err := newAgent(*backend)
	if err != nil {
		return err
	}
	cfg := agent.Config{Cwd: *cwd, PermissionMode: *permissionMode}
	if *mcpFile != "" {
		b, err := os.ReadFile(*mcpFile)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(b, &cfg.MCPServers); err != nil {
			return fmt.Errorf("parse %s: %w", *mcpFile, err)
		}
	}
	if *resume != "" {
		err = a.Resume(ctx, cfg, *resume)
	} else {
		err = a.Start(ctx, cfg)
	}
	enc := json.NewEncoder(stdout)
	var total agent.Usage
	interrupted := func() error {
		_ = a.Interrupt()
	drain:
		for {
			select {
			case ev, ok := <-a.Events():
				if !ok {
					break drain
				}
				if err := enc.Encode(ev); err != nil {
					return err
				}
				if ev.Kind == agent.KindUsage && ev.Usage != nil {
					total.Add(*ev.Usage)
				}
			default:
				break drain
			}
		}
		if err := enc.Encode(agent.Event{Kind: agent.KindUsage, Usage: &total}); err != nil {
			return err
		}
		return errInterrupted
	}
	if err != nil {
		if ctx.Err() != nil {
			return interrupted()
		}
		return err
	}
	defer func() { _ = a.Interrupt() }()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	// A blocked stdin read cannot be cancelled; the goroutine ends with the process.
	followUps := make(chan string)
	go func() {
		defer close(followUps)
		r := bufio.NewReader(stdin)
		for {
			line, err := r.ReadString('\n')
			if s := strings.TrimSpace(line); s != "" {
				select {
				case followUps <- s:
				case <-ctx.Done():
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	if err := a.Send(ctx, fs.Arg(0)); err != nil {
		return err
	}
	var idle <-chan string // set between a turn end and the next prompt
	for {
		select {
		case <-ctx.Done():
			return interrupted()
		case ev, ok := <-a.Events():
			if !ok {
				return errors.New("agent events closed unexpectedly")
			}
			if err := enc.Encode(ev); err != nil {
				return err
			}
			switch ev.Kind {
			case agent.KindUsage:
				if ev.Usage != nil {
					total.Add(*ev.Usage)
				}
			case agent.KindError:
				return fmt.Errorf("agent error: %s", ev.Text)
			case agent.KindTurnEnd:
				idle = followUps
			}
		case prompt, ok := <-idle:
			if !ok {
				return nil
			}
			idle = nil
			if err := a.Send(ctx, prompt); err != nil {
				return err
			}
		}
	}
}
