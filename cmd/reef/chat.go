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
	"sync"
	"syscall"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/agent/claude"
	"github.com/bhanuprakaash/reef/agent/codex"
	"github.com/bhanuprakaash/reef/channel"
	"github.com/bhanuprakaash/reef/store"
)

const (
	_wrapCommand = "/wrap"
	_wrapPrompt  = "Wrap up this channel. Call the propose_wrap tool once with the full " +
		"rewritten brief (goal, status, key decisions, open questions), your own notes on " +
		"this feature, and the decisions made in this channel that are not recorded yet. " +
		"Then reply with one line saying it is proposed."
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
		a         agent.Agent
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
		if a, err = newAgent(*backend); err != nil {
			return err
		}
	} else {
		if a, err = newAgent(*backend); err != nil {
			return err
		}
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
	defer func() { _ = st.DeleteToken(context.WithoutCancel(ctx), tok.Token) }()
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
		if !resuming {
			err = errors.Join(err, st.DeleteChannel(context.WithoutCancel(ctx), ch.ID))
		}
		return err
	}

	runner := channel.NewRunner(st, a, ch.ID, *role, *backend, preamble)
	if *debug {
		runner.OnPrompt = func(prompt string) {
			_, _ = fmt.Fprintf(stderr, "--- prompt ---\n%s\n---\n", prompt)
		}
	}
	if err := converse(ctx, st, ch.ID, runner, stdin, stdout); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "\nchannel %d: resume with reef chat --channel %d\n", ch.ID, ch.ID)
	return err
}

// converse sends each line of stdin as a message and prints the agent's text. The
// line /wrap asks the agent for a wrap-up proposal and then asks the user to
// approve it. It returns nil at the end of stdin or when ctx is cancelled.
func converse(
	ctx context.Context, st *store.Store, channelID int64, r *channel.Runner,
	stdin io.Reader, stdout io.Writer,
) error {
	in := readLines(stdin)

	show := func(ev agent.Event) {
		if ev.Kind == agent.KindText {
			_, _ = io.WriteString(stdout, ev.Text)
		}
	}
	if _, err := io.WriteString(stdout, "> "); err != nil {
		return err
	}
	for {
		line, ok := in.next(ctx)
		if !ok {
			return nil
		}
		if line == "" {
			continue
		}
		wrap := line == _wrapCommand
		text := line
		var seen int64
		if wrap {
			text = _wrapPrompt
			p, err := st.PendingProposal(ctx, channelID)
			if err != nil && !errors.Is(err, store.ErrNotFound) {
				return err
			}
			seen = p.ID
		}
		if err := r.SendAs(ctx, line, text, show); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		if _, err := io.WriteString(stdout, "\n\n"); err != nil {
			return err
		}
		if wrap {
			if err := decideWrap(ctx, st, channelID, seen, in, stdout); err != nil {
				return err
			}
			if ctx.Err() != nil {
				return nil
			}
		}
		if _, err := io.WriteString(stdout, "> "); err != nil {
			return err
		}
	}
}

// decideWrap prints the proposal the agent made in the turn that just ended, one
// newer than seen, and approves it when the next line is y or yes, else rejects it.
// Lines in in before the prompt is printed are dropped, not taken as the answer.
func decideWrap(
	ctx context.Context, st *store.Store, channelID, seen int64,
	in *lineReader, stdout io.Writer,
) error {
	p, err := st.PendingProposal(ctx, channelID)
	if errors.Is(err, store.ErrNotFound) || (err == nil && p.ID == seen) {
		_, err = io.WriteString(stdout, "no proposal made\n\n")
		return err
	}
	if err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString(proposalText(p))
	for _, l := range in.drop() {
		if l != "" {
			fmt.Fprintf(&b, "ignored while wrapping: %s\n", l)
		}
	}
	b.WriteString("\napprove? [y/N] ")
	if _, err := io.WriteString(stdout, b.String()); err != nil {
		return err
	}
	answer, ok := in.next(ctx)
	if !ok {
		_, err = fmt.Fprintf(stdout, "\nproposal %d left pending\n\n", p.ID)
		return err
	}
	if a := strings.ToLower(answer); a == "y" || a == "yes" {
		_, added, err := st.ApproveProposal(ctx, p.ID)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(stdout, "approved: brief, notes and %d decisions saved\n\n", added)
		return err
	}
	if _, err := st.RejectProposal(ctx, p.ID); err != nil {
		return err
	}
	_, err = io.WriteString(stdout, "rejected: nothing changed\n\n")
	return err
}

func proposalText(p store.Proposal) string {
	var b strings.Builder
	fmt.Fprintf(&b, "proposal %d from %s\n\nbrief:\n%s\n\nnotes:\n%s\n\ndecisions:\n",
		p.ID, p.Role, p.Brief, p.Notes)
	for _, d := range p.Decisions {
		fmt.Fprintf(&b, "- %s", d.Body)
		if d.Why != "" {
			fmt.Fprintf(&b, " (%s)", d.Why)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// lineReader reads stdin lines in the background, blank ones too, so a line is
// queued the moment it is typed: whatever is queued when the wrap-up prompt is
// printed was typed before it.
type lineReader struct {
	mu    sync.Mutex
	lines []string
	eof   bool
	wake  chan struct{}
}

// readLines starts reading r. A blocked read cannot be cancelled; the goroutine ends
// at the end of r or with the process.
func readLines(r io.Reader) *lineReader {
	l := &lineReader{wake: make(chan struct{}, 1)}
	go func() {
		in := bufio.NewReader(r)
		for {
			line, err := in.ReadString('\n')
			l.mu.Lock()
			if err != nil {
				l.eof = true
			}
			if s := strings.TrimSpace(line); s != "" || err == nil {
				l.lines = append(l.lines, s)
			}
			l.mu.Unlock()
			select {
			case l.wake <- struct{}{}:
			default:
			}
			if err != nil {
				return
			}
		}
	}()
	return l
}

// next returns the oldest queued line, waiting for one. It returns false at the end
// of stdin or when ctx is done.
func (l *lineReader) next(ctx context.Context) (string, bool) {
	for {
		if ctx.Err() != nil {
			return "", false
		}
		l.mu.Lock()
		if len(l.lines) > 0 {
			line := l.lines[0]
			l.lines = l.lines[1:]
			l.mu.Unlock()
			return line, true
		}
		eof := l.eof
		l.mu.Unlock()
		if eof {
			return "", false
		}
		select {
		case <-ctx.Done():
		case <-l.wake:
		}
	}
}

// drop removes and returns every queued line.
func (l *lineReader) drop() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	dropped := l.lines
	l.lines = nil
	return dropped
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

func lockChat(dir string, channelID int64) (*os.File, error) {
	return flock(chatLockPath(dir, channelID),
		fmt.Errorf("channel %d is open in another reef chat", channelID))
}

func chatOpen(dir string, channelID int64) bool {
	lock, err := os.Open(chatLockPath(dir, channelID))
	if err != nil {
		return false
	}
	defer func() { _ = lock.Close() }()
	return errors.Is(syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB), syscall.EWOULDBLOCK)
}
