package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/channel"
	"github.com/bhanuprakaash/reef/store"
)

const _chatUsage = `usage: reef chat <feature> --role R --backend claude|codex [--name N]
  [--permission-mode M] [--debug] [--data DIR]
       reef chat --channel ID [--debug] [--data DIR]`

func chatCmd(
	ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer,
) error {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGHUP)
	defer stop()

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
	if resuming == (len(pos) == 1) ||
		(resuming && (*role != "" || *backend != "" || *name != "" || *permissionMode != "")) ||
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

	if !resuming {
		f, err := st.FeatureByRef(ctx, pos[0])
		if err != nil {
			return err
		}
		if *name == "" {
			*name = *role + " chat"
		}
		ch, err := st.CreateChannel(ctx, store.Channel{
			ProjectID: f.ProjectID, FeatureID: &f.ID, Name: *name, Role: *role,
			Backend: *backend, PermissionMode: *permissionMode,
		})
		if err != nil {
			return err
		}
		*channelID = ch.ID
	} else {
		ch, err := st.Channel(ctx, *channelID)
		if err != nil {
			return err
		}
		if _, err := channel.CheckCaps(ctx, st, &ch); err != nil {
			return err
		}
	}
	sess, err := channel.Open(ctx, st, dir, exe, *channelID)
	if err != nil {
		if !resuming {
			err = errors.Join(err, st.DeleteChannel(context.WithoutCancel(ctx), *channelID))
		}
		return err
	}
	defer func() { _ = sess.Close() }()

	if *debug {
		sess.SetOnPrompt(func(prompt string) {
			_, _ = fmt.Fprintf(stderr, "--- prompt ---\n%s\n---\n", prompt)
		})
	}
	if err := converse(ctx, st, sess, readLines(stdin), stdout); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "\nchannel %d: resume with reef chat --channel %d\n",
		*channelID, *channelID)
	return err
}

type chatSession interface {
	Say(ctx context.Context, body string, parent *int64) error
	Run(ctx context.Context, text string, parent *int64, onEvent func(agent.Event)) error
	Wrap(ctx context.Context, onEvent func(agent.Event)) (store.Proposal, error)
}

func converse(
	ctx context.Context, st *store.Store, sess chatSession, in *lineReader, stdout io.Writer,
) error {
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
		var p store.Proposal
		wrap := line == channel.WrapCommand
		err := sess.Say(ctx, line, nil)
		if err == nil && wrap {
			p, err = sess.Wrap(ctx, show)
		} else if err == nil {
			err = sess.Run(ctx, line, nil, show)
		}
		noProposal := errors.Is(err, channel.ErrNoProposal)
		if err != nil && !noProposal {
			if ctx.Err() != nil {
				return nil
			}
			if errors.Is(err, channel.ErrCapped) {
				_, err = fmt.Fprintf(stdout, "\n\n%v\n", err)
			}
			return err
		}
		if _, err := io.WriteString(stdout, "\n\n"); err != nil {
			return err
		}
		if wrap {
			stop, err := decideWrap(ctx, st, p, noProposal, in, stdout)
			if err != nil {
				return err
			}
			if stop || ctx.Err() != nil {
				return nil
			}
		}
		if _, err := io.WriteString(stdout, "> "); err != nil {
			return err
		}
	}
}

func decideWrap(
	ctx context.Context, st *store.Store, p store.Proposal, noProposal bool,
	in *lineReader, stdout io.Writer,
) (stop bool, err error) {
	if noProposal {
		_, err = io.WriteString(stdout, "no proposal made\n\n")
		return false, err
	}
	var b strings.Builder
	b.WriteString(proposalText(p))
	b.WriteString("\napprove? [y/N] ")
	if _, err := io.WriteString(stdout, b.String()); err != nil {
		return false, err
	}
	dropped := in.drop()
	if len(dropped) > 0 {
		if _, err := io.WriteString(stdout, "\n"); err != nil {
			return false, err
		}
		for _, l := range dropped {
			if l != "" {
				if _, err := fmt.Fprintf(stdout, "ignored while wrapping: %s\n", l); err != nil {
					return false, err
				}
			}
		}
		if _, err := io.WriteString(stdout, "approve? [y/N] "); err != nil {
			return false, err
		}
	}
	answer, ok := in.next(ctx)
	if !ok {
		_, err = fmt.Fprintf(stdout, "\nproposal %d left pending\n\n", p.ID)
		return true, err
	}
	if a := strings.ToLower(answer); a == "y" || a == "yes" {
		_, added, err := st.ApproveProposal(ctx, p.ID)
		if err != nil {
			if errors.Is(err, store.ErrInvalid) {
				_, err = fmt.Fprintf(stdout, "reef: %s\n\n", strings.TrimPrefix(err.Error(), "invalid: "))
				return false, err
			}
			return false, err
		}
		_, err = fmt.Fprintf(stdout, "approved: brief, notes and %d decisions saved\n\n", added)
		return false, err
	}
	if _, err := st.RejectProposal(ctx, p.ID); err != nil {
		if errors.Is(err, store.ErrInvalid) {
			_, err = fmt.Fprintf(stdout, "reef: %s\n\n", strings.TrimPrefix(err.Error(), "invalid: "))
			return false, err
		}
		return false, err
	}
	_, err = io.WriteString(stdout, "rejected: nothing changed\n\n")
	return false, err
}

func proposalText(p store.Proposal) string {
	var b strings.Builder
	fmt.Fprintf(&b, "proposal %d from %s\n\n%s", p.ID, p.Role, proposalBodyText(p))
	return b.String()
}

func proposalBodyText(p store.Proposal) string {
	var b strings.Builder
	fmt.Fprintf(&b, "brief:\n%s\n\nnotes:\n%s\n\ndecisions:\n", p.Brief, p.Notes)
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
