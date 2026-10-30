package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/memory"
	"github.com/bhanuprakaash/reef/space"
	"github.com/bhanuprakaash/reef/store"
)

func testCLIBot(t *testing.T, st *store.Store, name string) *int64 {
	t.Helper()
	bots, err := st.Teammates(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range bots {
		if b.Name == name {
			return &b.ID
		}
	}
	b, err := st.CreateTeammate(context.Background(), store.Teammate{Name: name, Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	return &b.ID
}

type fakeSession struct {
	st       *store.Store
	spaceID  int64
	propose  bool
	replaces []int64
	barrier  chan struct{}
	sent     chan struct{}
	stored   []string
	said     []string
	wraps    int
}

func (f *fakeSession) Say(_ context.Context, body string, _ *int64) (store.Message, error) {
	f.stored = append(f.stored, body)
	return store.Message{}, nil
}

func (f *fakeSession) Run(_ context.Context, text string, _ *int64, onEvent func(agent.Event)) error {
	f.said = append(f.said, text)
	onEvent(agent.Event{Kind: agent.KindText, Text: "ok"})
	return nil
}

func (f *fakeSession) Wrap(ctx context.Context, onEvent func(agent.Event)) (store.Proposal, error) {
	f.wraps++
	if f.sent != nil {
		f.sent <- struct{}{}
	}
	if f.barrier != nil {
		<-f.barrier
	}
	onEvent(agent.Event{Kind: agent.KindText, Text: "ok"})
	if !f.propose {
		return store.Proposal{}, space.ErrNoProposal
	}
	id, err := f.st.SpaceMemberTeammateID(ctx, f.spaceID, "architect")
	if err != nil {
		return store.Proposal{}, err
	}
	return f.st.CreateProposal(ctx, store.Proposal{
		SpaceID: f.spaceID, BotID: &id, Role: "architect", Brief: "new brief",
		Items: []store.ProposedItem{
			{Kind: "decision", Body: "use sessions", Why: "simple", Scope: "work_item", Replaces: f.replaces},
			{Kind: "lesson", Body: "reply short", Scope: "teammate"},
		},
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

func startChat(t *testing.T, st *store.Store, ch store.Space, w *fakeSession) *chatRun {
	t.Helper()
	id := testCLIBot(t, st, "architect")
	if _, err := st.AddSpaceMember(context.Background(), ch.ID, *id); err != nil {
		t.Fatal(err)
	}
	w.st, w.spaceID = st, ch.ID
	pr, pw := io.Pipe()
	t.Cleanup(func() { _ = pw.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	lines := readLines(pr)
	c := &chatRun{
		in: pw, lines: lines, out: &syncBuffer{}, done: make(chan error, 1), cancel: cancel,
	}
	ms, err := memory.Open(st.DB(), nil)
	if err != nil {
		t.Fatal(err)
	}
	go func() { c.done <- converse(ctx, st, ms, ch.ID, w, lines, c.out) }()
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

func chatFixture(t *testing.T) (*store.Store, store.Space) {
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
	f, err := st.CreateWorkItem(ctx, store.WorkItem{ProjectID: p.ID, Title: "Login"})
	if err != nil {
		t.Fatal(err)
	}
	ch, err := st.CreateSpace(ctx, store.Space{ProjectID: p.ID, WorkItemID: &f.ID, Name: "chat"})
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
		wantRows     int
	}{
		{name: "approve", propose: true, answer: "y",
			wantOut: "approved: brief and 2 items saved", wantBrief: "new brief", wantRows: 2},
		{name: "reject", propose: true, answer: "no", wantOut: "rejected: nothing changed"},
		{name: "end of input", propose: true,
			wantOut: "proposal 1 left pending",
			wantExactOut: "> ok\n\nproposal 1 from architect\n\nbrief:\nnew brief\n\nitems:\n" +
				"- decision (work_item): use sessions (simple)\n- lesson (teammate): reply short\n\n" +
				"approve? [y/N] \nproposal 1 left pending\n\n",
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
			if _, err := memory.Open(st.DB(), nil); err != nil {
				t.Fatal(err)
			}
			var rows int
			if err := st.DB().QueryRow("SELECT COUNT(*) FROM memory").Scan(&rows); err != nil || rows != tt.wantRows {
				t.Errorf("memory rows = %d, %v, want %d", rows, err, tt.wantRows)
			}
		})
	}
}

func TestConverse_WrapApproveRefusedByMemoryIsReportedAndLeftPending(t *testing.T) {
	ctx := context.Background()
	st, ch := chatFixture(t)
	ms, err := memory.Open(st.DB(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ms.Remember(ctx,
		memory.Writer{Author: "user", Role: "user", Scope: memory.Ref{Kind: memory.ScopeProject, ID: "1"}},
		memory.Note{Kind: memory.KindFact, Body: "on the project shelf"}); err != nil {
		t.Fatal(err)
	}
	w := &fakeSession{propose: true, replaces: []int64{1}}
	c := startChat(t, st, ch, w)

	c.say(t, "/wrap")
	c.waitFor(t, "approve? [y/N] ")
	c.say(t, "y")
	c.waitFor(t, "reef: ")
	out := c.end(t)

	want := "approve? [y/N] reef: save item \"use sessions\": replace 1: " +
		"invalid: memory 1 is in scope project:1, not work_item:1\n\n> "
	if !strings.HasSuffix(out, want) {
		t.Errorf("output = %q, want it to end with %q", out, want)
	}
	if p, err := st.PendingProposal(ctx, ch.ID); err != nil || p.ID != 1 {
		t.Errorf("PendingProposal = %+v, %v, want proposal 1 still pending", p, err)
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

	want := "> ok\n\nproposal 1 from architect\n\nbrief:\nnew brief\n\nitems:\n" +
		"- decision (work_item): use sessions (simple)\n- lesson (teammate): reply short\n\n" +
		"approve? [y/N] \nproposal 1 left pending\n\n"
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
	w := &fakeSession{propose: true, barrier: make(chan struct{}), sent: make(chan struct{}, 1)}
	c := startChat(t, st, ch, w)

	c.say(t, "/wrap")
	<-w.sent
	c.say(t, "a")
	c.say(t, "")
	c.say(t, "y")
	c.waitQueued(t, 3)
	close(w.barrier)
	c.waitFor(t, "approve? [y/N] ")
	out := c.end(t)

	want := "> ok\n\nproposal 1 from architect\n\nbrief:\nnew brief\n\nitems:\n" +
		"- decision (work_item): use sessions (simple)\n- lesson (teammate): reply short\n\n" +
		"approve? [y/N] \n" +
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
				_, err := st.ApproveProposal(context.Background(), 1)
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
			t.Setenv(space.TokenEnv, tt.token)
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
			_, err := reef(t, t.TempDir(), append([]string{"chat", "--space", "1"}, tt.give...)...)
			if err == nil || err.Error() != _chatUsage {
				t.Errorf("chat error = %v, want the usage", err)
			}
		})
	}
}

func TestRun_ChatBackendTypoLeavesNoSpace(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(t.TempDir())
	mustReef(t, dir, "project", "add")
	mustReef(t, dir, "work-item", "add", "Login")

	_, err := reef(t, dir, "chat", "1", "--backend", "nope")
	if err == nil || err.Error() != `invalid: unknown backend "nope"` {
		t.Errorf("chat error = %v, want unknown backend", err)
	}
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	if chs, err := st.Spaces(context.Background(), store.SpaceFilter{}); err != nil || len(chs) != 0 {
		t.Errorf("spaces = %+v, %v, want none", chs, err)
	}
}

func TestRun_ChatRefusesWhatCannotLeadItsSpace(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	t.Chdir(t.TempDir())
	mustReef(t, dir, "project", "add")
	mustReef(t, dir, "work-item", "add", "Login")
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	parent := int64(1)
	if _, err := st.CreateWorkItem(ctx, store.WorkItem{ProjectID: 1, ParentID: &parent, Title: "Form"}); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		give []string
		want string
	}{
		{"another role", []string{"chat", "1", "--role", "developer", "--backend", "claude"},
			"invalid: reef chat runs the lead of team solo, lead: --role developer is not it"},
		{"sub-item", []string{"chat", "2", "--backend", "claude"},
			"invalid: work item 2 is a sub-item of work item 1: open its parent"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := reef(t, dir, tt.give...)
			if err == nil || err.Error() != tt.want {
				t.Errorf("chat error = %v, want %q", err, tt.want)
			}
		})
	}
	if chs, err := st.Spaces(ctx, store.SpaceFilter{}); err != nil || len(chs) != 0 {
		t.Errorf("spaces = %+v, %v, want none", chs, err)
	}
}

func TestRun_ChatResumesPausedSpace(t *testing.T) {
	dir := t.TempDir()
	projectPath := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	p, err := st.CreateProject(context.Background(), store.Project{Name: "shop", Path: projectPath})
	if err != nil {
		t.Fatal(err)
	}
	ch, err := st.CreateSpace(context.Background(), store.Space{
		ProjectID: p.ID, Name: "chat", Role: "lead", Backend: "claude",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PauseSpace(context.Background(), ch.ID, "token budget of 1000000 used: 1000312 tokens"); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	fakeClaude, err := filepath.Abs("../../agent/claude/testdata/claude")
	if err != nil {
		t.Fatal(err)
	}
	binDir := t.TempDir()
	if err := os.Symlink(fakeClaude, filepath.Join(binDir, "claude")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_CLAUDE", "hang")

	out, err := reef(t, dir, "chat", "--space", "1")
	if err != nil {
		t.Fatalf("reef chat --space 1 = %v, output %q", err, out)
	}
	want := "> \nspace 1: resume with reef chat --space 1\n"
	if out != want {
		t.Errorf("reef chat output = %q, want %q", out, want)
	}
}

func TestRun_ChatRefusesMovedSpace(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	p, err := st.CreateProject(ctx, store.Project{Name: "shop", Path: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"talk", "moved"} {
		if _, err := st.CreateSpace(ctx, store.Space{
			ProjectID: p.ID, Name: name, Role: "lead", Backend: "claude",
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.DB().Exec("UPDATE space SET moved_to = 2 WHERE id = 1"); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	out, err := reef(t, dir, "chat", "--space", "1")
	if err == nil || err.Error() != "conversation 1 moved to Space 2: conflict" {
		t.Errorf("reef chat --space 1 = %v, output %q, want the moved error", err, out)
	}
	st, err = store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	if msgs, err := st.Messages(ctx, 1, 0); err != nil || len(msgs) != 0 {
		t.Errorf("messages = %+v, %v, want none", msgs, err)
	}
}

func TestChat_ResumeMultiRoleTeamUsesWindow(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	p, err := st.CreateProject(ctx, store.Project{Name: "shop", Path: "/tmp/shop"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.ApplyTeam(ctx, store.Team{Name: "duo", Roles: []store.Role{
		{Name: "lead", Lead: true, Backend: "claude"},
		{Name: "architect", Backend: "claude"},
	}}); err != nil {
		t.Fatal(err)
	}
	ch, err := st.CreateSpace(ctx, store.Space{
		ProjectID: p.ID, Name: "chat", Team: "duo", Role: "lead", Backend: "claude",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	out, err := reef(t, dir, "chat", "--space", fmt.Sprint(ch.ID))
	if err == nil || !strings.Contains(err.Error(), "use the Reef window") {
		t.Fatalf("reef chat --space %d error = %v, output %q", ch.ID, err, out)
	}
	if !strings.Contains(err.Error(), `team "duo"`) {
		t.Fatalf("reef chat error = %v, want team name", err)
	}

	st, err = store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	turns, err := st.Turns(ctx, store.TurnFilter{SpaceID: &ch.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 0 {
		t.Fatalf("turns = %v, want none", turns)
	}
}

func useTokens(t *testing.T, st *store.Store, spaceID, tokens int64) {
	t.Helper()
	ctx := context.Background()
	turn, err := st.CreateTurn(ctx, store.Turn{SpaceID: spaceID, Role: "developer", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	turn.Status, turn.Usage = "done", agent.Usage{InputTokens: tokens}
	if _, err := st.EndTurn(ctx, turn); err != nil {
		t.Fatal(err)
	}
}

func TestConverse_PausedSpaceRefusesMessageUntilContinue(t *testing.T) {
	ctx := context.Background()
	st, ch := chatFixture(t)
	if err := st.PauseSpace(ctx, ch.ID, "token budget of 1000000 used: 1000312 tokens"); err != nil {
		t.Fatal(err)
	}
	w := &fakeSession{}
	c := startChat(t, st, ch, w)

	c.say(t, "hi")
	c.waitFor(t, "/continue to go on")
	c.say(t, "/continue")
	c.waitFor(t, "continued")
	c.say(t, "hello")
	c.waitFor(t, "ok")
	out := c.end(t)

	want := "> space 1 is paused: token budget of 1000000 used: 1000312 tokens\n/continue to go on\n\n> " +
		"continued\n\n> ok\n\n> "
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
	if !slices.Equal(w.said, []string{"hello"}) || !slices.Equal(w.stored, []string{"hello"}) {
		t.Errorf("said = %q, stored = %q, want only [hello]", w.said, w.stored)
	}
	if got, err := st.Space(ctx, ch.ID); err != nil || got.Paused != "" || got.TokenBudget != store.TokenBudget {
		t.Errorf("space = paused %q budget %d, %v, want not paused, budget %d",
			got.Paused, got.TokenBudget, err, store.TokenBudget)
	}
}

func TestConverse_TokenBudgetReachedMidChatPausesNextMessage(t *testing.T) {
	ctx := context.Background()
	st, ch := chatFixture(t)
	w := &fakeSession{}
	c := startChat(t, st, ch, w)

	c.say(t, "one")
	c.waitFor(t, "ok")
	useTokens(t, st, ch.ID, store.TokenBudget)
	c.say(t, "two")
	c.waitFor(t, "/continue to go on")
	c.say(t, "/continue")
	c.waitFor(t, "continued")
	c.say(t, "three")
	c.waitFor(t, "continued\n\n> ok\n\n> ")
	out := c.end(t)

	if want := "space 1 is paused: token budget of 1000000 used: 1000000 tokens\n"; !strings.Contains(out, want) {
		t.Errorf("output = %q, want it to contain %q", out, want)
	}
	if !slices.Equal(w.said, []string{"one", "three"}) {
		t.Errorf("said = %q, want [one three]", w.said)
	}
	if got, err := st.Space(ctx, ch.ID); err != nil || got.Paused != "" || got.TokenBudget != 2*store.TokenBudget {
		t.Errorf("space = paused %q budget %d, %v, want not paused, budget %d",
			got.Paused, got.TokenBudget, err, 2*store.TokenBudget)
	}
}

func TestConverse_ContinueWhenNotPaused(t *testing.T) {
	st, ch := chatFixture(t)
	c := startChat(t, st, ch, &fakeSession{})

	c.say(t, "/continue")
	c.waitFor(t, "the Space is not paused")
	if out := c.end(t); out != "> the Space is not paused\n\n> " {
		t.Errorf("output = %q, want the Space is not paused", out)
	}
}
