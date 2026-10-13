package main

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/channel"
	"github.com/bhanuprakaash/reef/store"
)

// wrapAgent answers each prompt with "ok". Like the real agent calling propose_wrap,
// it stores a proposal when it gets the wrap prompt and propose is set. A turn ends
// only after gate is closed, when gate is set.
type wrapAgent struct {
	st        *store.Store
	channelID int64
	propose   bool
	gate      chan struct{}
	sent      chan struct{}
	events    chan agent.Event
	prompts   []string
}

func (w *wrapAgent) Start(context.Context, agent.Config) error          { return nil }
func (w *wrapAgent) Resume(context.Context, agent.Config, string) error { return nil }
func (w *wrapAgent) Events() <-chan agent.Event                         { return w.events }
func (w *wrapAgent) Interrupt() error                                   { return nil }

func (w *wrapAgent) Send(ctx context.Context, prompt string) error {
	w.prompts = append(w.prompts, prompt)
	if w.sent != nil {
		w.sent <- struct{}{}
	}
	if w.gate != nil {
		<-w.gate
	}
	if w.propose && prompt == _wrapPrompt {
		_, err := w.st.CreateProposal(ctx, store.Proposal{
			ChannelID: w.channelID, Role: "architect", Brief: "new brief", Notes: "my notes",
			Decisions: []store.ProposedDecision{{Body: "use sessions", Why: "simple"}},
		})
		if err != nil {
			return err
		}
	}
	w.events <- agent.Event{Kind: agent.KindText, Text: "ok"}
	w.events <- agent.Event{Kind: agent.KindTurnEnd}
	return nil
}

// chatRun is converse reading from a pipe the test writes to.
type chatRun struct {
	in     *io.PipeWriter
	out    *syncBuffer
	done   chan error
	cancel context.CancelFunc
}

func startChat(t *testing.T, st *store.Store, ch store.Channel, w *wrapAgent) *chatRun {
	t.Helper()
	w.st, w.channelID, w.events = st, ch.ID, make(chan agent.Event, 8)
	pr, pw := io.Pipe()
	t.Cleanup(func() { _ = pw.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	c := &chatRun{in: pw, out: &syncBuffer{}, done: make(chan error, 1), cancel: cancel}
	r := channel.NewRunner(st, w, ch.ID, "architect", "claude", "")
	go func() { c.done <- converse(ctx, st, ch.ID, r, pr, c.out) }()
	return c
}

func (c *chatRun) say(t *testing.T, line string) {
	t.Helper()
	if _, err := io.WriteString(c.in, line+"\n"); err != nil {
		t.Fatal(err)
	}
}

func (c *chatRun) waitFor(t *testing.T, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(c.out.String(), want) {
		if time.Now().After(deadline) {
			t.Fatalf("no %q after 5s; output: %q", want, c.out.String())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// end closes stdin and returns what converse printed.
func (c *chatRun) end(t *testing.T) string {
	t.Helper()
	_ = c.in.Close()
	if err := <-c.done; err != nil {
		t.Fatalf("converse = %v, want nil", err)
	}
	return c.out.String()
}

func chatFixture(t *testing.T) (*store.Store, store.Channel) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	p, err := st.CreateProject(ctx, store.Project{Name: "shop", Path: "/tmp/shop"})
	if err != nil {
		t.Fatal(err)
	}
	f, err := st.CreateFeature(ctx, store.Feature{ProjectID: p.ID, Slug: "login", Name: "Login"})
	if err != nil {
		t.Fatal(err)
	}
	ch, err := st.CreateChannel(ctx, store.Channel{ProjectID: p.ID, FeatureID: &f.ID, Name: "chat"})
	if err != nil {
		t.Fatal(err)
	}
	return st, ch
}

func TestConverse_Wrap(t *testing.T) {
	tests := []struct {
		name        string
		propose     bool
		answer      string
		wantOut     string
		wantBrief   string
		wantPending bool
	}{
		{name: "approve", propose: true, answer: "y",
			wantOut: "approved: brief, notes and 1 decisions saved", wantBrief: "new brief"},
		{name: "reject", propose: true, answer: "no", wantOut: "rejected: nothing changed"},
		{name: "end of input", propose: true,
			wantOut: "proposal 1 left pending", wantPending: true},
		{name: "no proposal", wantOut: "no proposal made"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			st, ch := chatFixture(t)
			w := &wrapAgent{propose: tt.propose}
			c := startChat(t, st, ch, w)

			c.say(t, "/wrap")
			if tt.propose {
				c.waitFor(t, "approve? [y/N] ")
			}
			if tt.answer != "" {
				c.say(t, tt.answer)
			} else if tt.propose {
				_ = c.in.Close()
			}
			c.waitFor(t, tt.wantOut)
			c.end(t)

			if len(w.prompts) != 1 || w.prompts[0] != _wrapPrompt {
				t.Errorf("agent prompts = %q, want the wrap prompt once", w.prompts)
			}
			msgs, err := st.Messages(ctx, ch.ID, 0)
			if err != nil {
				t.Fatal(err)
			}
			if len(msgs) != 2 || msgs[0].Author != "user" || msgs[0].Body != "/wrap" {
				t.Errorf("messages = %+v, want the user message /wrap first", msgs)
			}
			brief, err := st.Brief(ctx, 1)
			if tt.wantBrief == "" && !errors.Is(err, store.ErrNotFound) ||
				tt.wantBrief != "" && (err != nil || brief.Body != tt.wantBrief) {
				t.Errorf("brief = %q, %v, want %q", brief.Body, err, tt.wantBrief)
			}
			_, err = st.PendingProposal(ctx, ch.ID)
			if tt.wantPending != (err == nil) {
				t.Errorf("PendingProposal error = %v, want pending %v", err, tt.wantPending)
			}
		})
	}
}

func TestConverse_WrapCancelledAtPromptLeavesProposalPending(t *testing.T) {
	st, ch := chatFixture(t)
	w := &wrapAgent{propose: true}
	c := startChat(t, st, ch, w)

	c.say(t, "/wrap")
	c.waitFor(t, "approve? [y/N] ")
	c.cancel()
	out := c.end(t)

	want := "> ok\n\nproposal 1 from architect\n\nbrief:\nnew brief\n\nnotes:\nmy notes\n\n" +
		"decisions:\n- use sessions (simple)\n\napprove? [y/N] \nproposal 1 left pending\n\n"
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
	p, err := st.PendingProposal(context.Background(), ch.ID)
	if err != nil || p.ID != 1 || p.DecidedAt != nil {
		t.Errorf("PendingProposal = %+v, %v, want proposal 1 still pending", p, err)
	}
}

func TestConverse_WrapIgnoresTypedAheadLines(t *testing.T) {
	st, ch := chatFixture(t)
	w := &wrapAgent{propose: true, gate: make(chan struct{}), sent: make(chan struct{}, 1)}
	c := startChat(t, st, ch, w)

	c.say(t, "/wrap")
	<-w.sent
	c.say(t, "a")
	c.say(t, "")
	c.say(t, "y")
	close(w.gate)
	c.waitFor(t, "approve? [y/N] ")
	out := c.end(t)

	want := "> ok\n\nproposal 1 from architect\n\nbrief:\nnew brief\n\nnotes:\nmy notes\n\n" +
		"decisions:\n- use sessions (simple)\nignored while wrapping: a\n" +
		"ignored while wrapping: y\n\napprove? [y/N] \nproposal 1 left pending\n\n> "
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
	if p, err := st.PendingProposal(context.Background(), ch.ID); err != nil || p.ID != 1 {
		t.Errorf("PendingProposal = %+v, %v, want proposal 1 still pending", p, err)
	}
}

func TestConverse_WrapBlankLineAtPromptRejects(t *testing.T) {
	st, ch := chatFixture(t)
	w := &wrapAgent{propose: true}
	c := startChat(t, st, ch, w)

	c.say(t, "/wrap")
	c.waitFor(t, "approve? [y/N] ")
	c.say(t, "")
	c.waitFor(t, "rejected: nothing changed")
	out := c.end(t)

	if !strings.HasSuffix(out, "approve? [y/N] rejected: nothing changed\n\n> ") {
		t.Errorf("output = %q, want the blank line to reject", out)
	}
	if _, err := st.PendingProposal(context.Background(), ch.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("PendingProposal error = %v, want none pending", err)
	}
	if brief, err := st.Brief(context.Background(), 1); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("brief = %+v, %v, want no brief", brief, err)
	}
}

func TestRun_McpNeedsKnownToken(t *testing.T) {
	tests := []struct {
		name  string
		token string
		want  string
	}{
		{"not set", "", "REEF_TOKEN is not set"},
		{"unknown", "abc", "unknown REEF_TOKEN"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(_tokenEnv, tt.token)
			err := run(context.Background(), []string{"mcp", "--data", t.TempDir()},
				strings.NewReader(""), io.Discard, io.Discard)
			if err == nil || err.Error() != tt.want {
				t.Errorf("run mcp = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestRun_ChatBackendTypoLeavesNoChannel(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(t.TempDir())
	mustReef(t, dir, "project", "add")
	mustReef(t, dir, "feature", "add", "login", "Login")

	_, err := reef(t, dir, "chat", "login", "--role", "developer", "--backend", "nope")
	if err == nil || err.Error() != `unknown backend "nope"` {
		t.Errorf("chat error = %v, want unknown backend", err)
	}
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	if chs, err := st.Channels(context.Background(), store.ChannelFilter{}); err != nil || len(chs) != 0 {
		t.Errorf("channels = %+v, %v, want none", chs, err)
	}
}
