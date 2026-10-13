package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/agent/claude"
	"github.com/bhanuprakaash/reef/agent/codex"
	"github.com/bhanuprakaash/reef/channel"
	"github.com/bhanuprakaash/reef/store"
)

const _chatUsage = `usage: reef chat <feature> --role R --backend claude|codex [--name N]
  [--permission-mode M] [--debug] [--data DIR]
       reef chat --channel ID [--permission-mode M] [--debug] [--data DIR]`

func chatCmd(
	ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer,
) error {
	c := newCommand("reef chat", _chatUsage, 0, 1)
	role := c.fs.String("role", "", "")
	backend := c.fs.String("backend", "", "")
	name := c.fs.String("name", "", "")
	permissionMode := c.fs.String("permission-mode", "", "")
	debug := c.fs.Bool("debug", false, "")
	channelID := c.fs.Int64("channel", 0, "")
	st, pos, err := c.open(args)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	resuming := *channelID != 0
	if resuming == (len(pos) == 1) || (resuming && (*role != "" || *backend != "" || *name != "")) ||
		(!resuming && (*role == "" || *backend == "")) {
		return errors.New(_chatUsage)
	}
	dir, err := dataDir(*c.data)
	if err != nil {
		return err
	}
	if dir, err = filepath.Abs(dir); err != nil {
		return fmt.Errorf("resolve data dir: %w", err)
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find reef path: %w", err)
	}

	var (
		ch        store.Channel
		sessionID string
		preamble  string
	)
	if resuming {
		if ch, err = st.Channel(ctx, *channelID); err != nil {
			return err
		}
		lock, err := lockChat(dir, ch.ID)
		if err != nil {
			return err
		}
		defer func() { _ = lock.Close() }()
		last, err := resumeTurn(ctx, st, ch.ID)
		if err != nil {
			return err
		}
		*role, *backend, sessionID = last.Role, last.Backend, last.SessionID
	} else {
		f, err := st.FeatureByRef(ctx, pos[0])
		if err != nil {
			return err
		}
		if preamble, err = channel.Context(ctx, st, f.ID, *role); err != nil {
			return err
		}
		if *name == "" {
			*name = *role + " chat"
		}
		if ch, err = st.CreateChannel(ctx, store.Channel{
			ProjectID: f.ProjectID, FeatureID: &f.ID, Name: *name,
		}); err != nil {
			return err
		}
		lock, err := lockChat(dir, ch.ID)
		if err != nil {
			return err
		}
		defer func() { _ = lock.Close() }()
	}
	p, err := st.Project(ctx, ch.ProjectID)
	if err != nil {
		return err
	}
	tok, err := st.CreateToken(ctx, ch.ID, *role)
	if err != nil {
		return err
	}
	a, err := newAgent(*backend)
	if err != nil {
		return err
	}
	defer func() { _ = a.Interrupt() }()
	cfg := agent.Config{
		Cwd:            p.Path,
		PermissionMode: *permissionMode,
		MCPServers: map[string]agent.MCPServer{"reef": {
			Command: exe,
			Args:    []string{"mcp", "--data", dir},
			Env:     map[string]string{_tokenEnv: tok.Token},
		}},
	}
	if resuming {
		err = a.Resume(ctx, cfg, sessionID)
	} else {
		err = a.Start(ctx, cfg)
	}
	if err != nil {
		return err
	}

	runner := channel.NewRunner(st, a, ch.ID, *role, *backend, preamble)
	if *debug {
		runner.OnPrompt = func(prompt string) {
			_, _ = fmt.Fprintf(stderr, "--- prompt ---\n%s\n---\n", prompt)
		}
	}
	if err := converse(ctx, runner, stdin, stdout); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "\nchannel %d: resume with reef chat --channel %d\n", ch.ID, ch.ID)
	return err
}

// converse sends each line of stdin as a message and prints the agent's text. It
// returns nil at the end of stdin or when ctx is cancelled.
func converse(ctx context.Context, r *channel.Runner, stdin io.Reader, stdout io.Writer) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	// A blocked stdin read cannot be cancelled; the goroutine ends with the process.
	lines := make(chan string)
	go func() {
		defer close(lines)
		in := bufio.NewReader(stdin)
		for {
			line, err := in.ReadString('\n')
			if s := strings.TrimSpace(line); s != "" {
				select {
				case lines <- s:
				case <-ctx.Done():
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	show := func(ev agent.Event) {
		if ev.Kind == agent.KindText {
			_, _ = io.WriteString(stdout, ev.Text)
		}
	}
	if _, err := io.WriteString(stdout, "> "); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case line, ok := <-lines:
			if !ok {
				return nil
			}
			if err := r.Send(ctx, line, show); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return err
			}
			if _, err := io.WriteString(stdout, "\n\n> "); err != nil {
				return err
			}
		}
	}
}

// resumeTurn is the channel's newest turn that has a session id. It first fails any
// turn left running by a chat that crashed; the caller holds the chat lock.
func resumeTurn(ctx context.Context, st *store.Store, channelID int64) (store.Turn, error) {
	turns, err := st.Turns(ctx, store.TurnFilter{ChannelID: &channelID})
	if err != nil {
		return store.Turn{}, err
	}
	var last store.Turn
	for _, t := range turns {
		if t.Status == "running" {
			t.Status, t.Error = "failed", "chat exited"
			if _, err := st.EndTurn(ctx, t); err != nil {
				return store.Turn{}, err
			}
		}
		if t.SessionID != "" {
			last = t
		}
	}
	if last.ID == 0 {
		return store.Turn{}, fmt.Errorf("channel %d has no session to resume", channelID)
	}
	return last, nil
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

func chatLockPath(dir string, channelID int64) string {
	return filepath.Join(dir, fmt.Sprintf("chat-%d.lock", channelID))
}

// lockChat holds the channel's chat lock until the returned file is closed, so
// only one reef chat runs on a channel.
func lockChat(dir string, channelID int64) (*os.File, error) {
	lock, err := os.OpenFile(chatLockPath(dir, channelID), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open chat lock: %w", err)
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = lock.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf("channel %d is open in another reef chat", channelID)
		}
		return nil, fmt.Errorf("lock chat: %w", err)
	}
	return lock, nil
}

// chatOpen reports whether a reef chat holds the channel's lock.
func chatOpen(dir string, channelID int64) bool {
	lock, err := os.Open(chatLockPath(dir, channelID))
	if err != nil {
		return false
	}
	defer func() { _ = lock.Close() }()
	return errors.Is(syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB), syscall.EWOULDBLOCK)
}
