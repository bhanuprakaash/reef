package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/channel"
	"github.com/bhanuprakaash/reef/store"
)

type fakeSession struct {
	st        *store.Store
	channelID int64
	propose   bool
	gate      chan struct{}
	sent      chan struct{}
	stored    []string
	said      []string
	wraps     int
}

func (f *fakeSession) Say(_ context.Context, body string) error {
	f.stored = append(f.stored, body)
	return nil
}

func (f *fakeSession) Run(_ context.Context, text string, onEvent func(agent.Event)) error {
	f.said = append(f.said, text)
	if text == "cap" {
		return fmt.Errorf("%w: more than 25 tool calls", channel.ErrCapped)
	}
	onEvent(agent.Event{Kind: agent.KindText, Text: "ok"})
	return nil
}

func (f *fakeSession) Wrap(ctx context.Context, onEvent func(agent.Event)) (store.Proposal, error) {
	f.wraps++
	if f.sent != nil {
		f.sent <- struct{}{}
	}
	if f.gate != nil {
		<-f.gate
	}
	onEvent(agent.Event{Kind: agent.KindText, Text: "ok"})
	if !f.propose {
		return store.Proposal{}, channel.ErrNoProposal
	}
	return f.st.CreateProposal(ctx, store.Proposal{
		ChannelID: f.channelID, Role: "architect", Brief: "new brief", Notes: "my notes",
		Decisions: []store.ProposedDecision{{Body: "use sessions", Why: "simple"}},
	})
}

// chatRun is converse reading from a pipe the test writes to.
type chatRun struct {
	in     *io.PipeWriter
	lines  *lineReader
	out    *syncBuffer
	done   chan error
	cancel context.CancelFunc
}

func startChat(t *testing.T, st *store.Store, ch store.Channel, w *fakeSession) *chatRun {
	t.Helper()
	w.st, w.channelID = st, ch.ID
	pr, pw := io.Pipe()
	t.Cleanup(func() { _ = pw.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	lines := readLines(pr)
	c := &chatRun{
		in: pw, lines: lines, out: &syncBuffer{}, done: make(chan error, 1), cancel: cancel,
	}
	go func() { c.done <- converse(ctx, st, w, lines, c.out) }()
	return c
}

func (c *chatRun) say(t *testing.T, line string) {
	t.Helper()
	if _, err := io.WriteString(c.in, line+"\n"); err != nil {
		t.Fatal(err)
	}
}

func (c *chatRun) waitQueued(t *testing.T, count int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		c.lines.mu.Lock()
		queued := len(c.lines.lines)
		c.lines.mu.Unlock()
		if queued >= count {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("line reader queued %d lines, want %d", queued, count)
		}
		time.Sleep(time.Millisecond)
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
		name         string
		propose      bool
		answer       string
		wantOut      string
		wantExactOut string
		wantBrief    string
		wantPending  bool
	}{
		{name: "approve", propose: true, answer: "y",
			wantOut: "approved: brief, notes and 1 decisions saved", wantBrief: "new brief"},
		{name: "reject", propose: true, answer: "no", wantOut: "rejected: nothing changed"},
		{name: "end of input", propose: true,
			wantOut: "proposal 1 left pending",
			wantExactOut: "> ok\n\nproposal 1 from architect\n\nbrief:\nnew brief\n\nnotes:\nmy notes\n\n" +
				"decisions:\n- use sessions (simple)\n\napprove? [y/N] \nproposal 1 left pending\n\n",
			wantPending: true},
		{name: "no proposal", wantOut: "no proposal made"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			st, ch := chatFixture(t)
			w := &fakeSession{propose: tt.propose}
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
			out := c.end(t)
			if tt.wantExactOut != "" && out != tt.wantExactOut {
				t.Errorf("output = %q, want %q", out, tt.wantExactOut)
			}

			if w.wraps != 1 || len(w.said) != 0 || !slices.Equal(w.stored, []string{"/wrap"}) {
				t.Errorf("wraps = %d, said = %q, stored = %q, want one wrap, stored as /wrap",
					w.wraps, w.said, w.stored)
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

func TestConverse_CappedTurnPrintsReasonAndEnds(t *testing.T) {
	st, ch := chatFixture(t)
	w := &fakeSession{}
	c := startChat(t, st, ch, w)

	c.say(t, "cap")
	if err := <-c.done; err != nil {
		t.Fatalf("converse = %v, want nil", err)
	}
	if want := "> \n\ncapped: more than 25 tool calls\n"; c.out.String() != want {
		t.Errorf("output = %q, want %q", c.out.String(), want)
	}
}

func TestConverse_WrapCancelledAtPromptLeavesProposalPending(t *testing.T) {
	st, ch := chatFixture(t)
	w := &fakeSession{propose: true}
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
	w := &fakeSession{propose: true, gate: make(chan struct{}), sent: make(chan struct{}, 1)}
	c := startChat(t, st, ch, w)

	c.say(t, "/wrap")
	<-w.sent
	c.say(t, "a")
	c.say(t, "")
	c.say(t, "y")
	c.waitQueued(t, 3)
	close(w.gate)
	c.waitFor(t, "approve? [y/N] ")
	out := c.end(t)

	want := "> ok\n\nproposal 1 from architect\n\nbrief:\nnew brief\n\nnotes:\nmy notes\n\n" +
		"decisions:\n- use sessions (simple)\n\napprove? [y/N] \n" +
		"ignored while wrapping: a\nignored while wrapping: y\napprove? [y/N] \n" +
		"proposal 1 left pending\n\n"
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
	if p, err := st.PendingProposal(context.Background(), ch.ID); err != nil || p.ID != 1 {
		t.Errorf("PendingProposal = %+v, %v, want proposal 1 still pending", p, err)
	}
}

func TestConverse_WrapBlankLineAtPromptRejects(t *testing.T) {
	st, ch := chatFixture(t)
	w := &fakeSession{propose: true}
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
	proposal, err := st.Proposal(context.Background(), 1)
	if err != nil || proposal.Status != store.ProposalRejected {
		t.Errorf("Proposal status = %q, %v, want %q", proposal.Status, err, store.ProposalRejected)
	}
	if brief, err := st.Brief(context.Background(), 1); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("brief = %+v, %v, want no brief", brief, err)
	}
}

func TestConverse_BlankLineThenMessage(t *testing.T) {
	st, ch := chatFixture(t)
	w := &fakeSession{}
	c := startChat(t, st, ch, w)

	c.say(t, "")
	c.say(t, "hi")
	c.waitFor(t, "ok")
	out := c.end(t)

	if !slices.Equal(w.said, []string{"hi"}) || !slices.Equal(w.stored, []string{"hi"}) {
		t.Errorf("said = %q, stored = %q, want [hi] for both; output %q", w.said, w.stored, out)
	}
}

func TestConverse_WrapDecisionElsewhereContinues(t *testing.T) {
	for _, tt := range []struct {
		name   string
		decide func(*store.Store) error
		answer string
		status string
	}{
		{
			name: "approved", answer: "y", status: store.ProposalApproved,
			decide: func(st *store.Store) error {
				_, _, err := st.ApproveProposal(context.Background(), 1)
				return err
			},
		},
		{
			name: "rejected", answer: "no", status: store.ProposalRejected,
			decide: func(st *store.Store) error {
				_, err := st.RejectProposal(context.Background(), 1)
				return err
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			st, ch := chatFixture(t)
			w := &fakeSession{propose: true}
			c := startChat(t, st, ch, w)
			c.say(t, "/wrap")
			c.waitFor(t, "approve? [y/N] ")
			if err := tt.decide(st); err != nil {
				t.Fatal(err)
			}
			c.say(t, tt.answer)
			wantLine := "reef: proposal 1 is already " + tt.status
			c.waitFor(t, wantLine)
			c.say(t, "hi")
			c.waitFor(t, "ok\n\n> ")
			out := c.end(t)

			if !strings.Contains(out, wantLine) || w.wraps != 1 || len(w.said) != 1 || w.said[0] != "hi" {
				t.Errorf("output = %q, said = %q; want warning and continued chat", out, w.said)
			}
		})
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
			t.Setenv(channel.TokenEnv, tt.token)
			err := run(context.Background(), []string{"mcp", "--data", t.TempDir()},
				strings.NewReader(""), io.Discard, io.Discard)
			if err == nil || err.Error() != tt.want {
				t.Errorf("run mcp = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestRun_ChatResumeRejectsNewSettings(t *testing.T) {
	tests := []struct {
		name string
		give []string
	}{
		{"role", []string{"--role", "tester"}},
		{"backend", []string{"--backend", "codex"}},
		{"name", []string{"--name", "x"}},
		{"permission mode", []string{"--permission-mode", "acceptEdits"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := reef(t, t.TempDir(), append([]string{"chat", "--channel", "1"}, tt.give...)...)
			if err == nil || err.Error() != _chatUsage {
				t.Errorf("chat error = %v, want the usage", err)
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
	if err == nil || err.Error() != `invalid: unknown backend "nope"` {
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
