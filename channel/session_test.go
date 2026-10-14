package channel

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/store"
)

type recordingAgent struct {
	st        *store.Store
	channelID int64
	propose   bool
	startErr  error
	closeErr  error

	cfg     agent.Config
	started bool
	closed  bool
	resumed string
	prompts []string
	events  chan agent.Event
}

func newRecordingAgent() *recordingAgent {
	return &recordingAgent{events: make(chan agent.Event, 8)}
}

func (a *recordingAgent) Start(_ context.Context, cfg agent.Config) error {
	a.cfg, a.started = cfg, true
	return a.startErr
}

func (a *recordingAgent) Resume(_ context.Context, cfg agent.Config, sessionID string) error {
	a.cfg, a.resumed = cfg, sessionID
	return nil
}

func (a *recordingAgent) Events() <-chan agent.Event { return a.events }
func (a *recordingAgent) Interrupt() error           { a.closed = true; return a.closeErr }

func (a *recordingAgent) Send(ctx context.Context, prompt string) error {
	a.prompts = append(a.prompts, prompt)
	if strings.Contains(prompt, "propose_wrap") && a.propose {
		_, err := a.st.CreateProposal(ctx, store.Proposal{
			ChannelID: a.channelID, Role: "architect", Brief: "new brief", Notes: "my notes",
		})
		if err != nil {
			return err
		}
	}
	a.events <- agent.Event{Kind: agent.KindSession, SessionID: "s1"}
	a.events <- agent.Event{Kind: agent.KindText, Text: "ok"}
	a.events <- agent.Event{Kind: agent.KindTurnEnd}
	return nil
}

func (a *recordingAgent) factory(string) (agent.Agent, error) { return a, nil }

func sessionChannel(t *testing.T, st *store.Store, f store.Feature, quick bool) store.Channel {
	t.Helper()
	c := store.Channel{
		ProjectID: f.ProjectID, Name: "chat", Role: "architect", Backend: "claude",
		PermissionMode: "acceptEdits",
	}
	if !quick {
		c.FeatureID = &f.ID
	}
	ch, err := st.CreateChannel(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	return ch
}

func discard(agent.Event) {}

func TestOpen_StartsNewSession(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch := sessionChannel(t, st, f, false)
	ag := newRecordingAgent()
	dir := t.TempDir()

	s, err := open(ctx, st, dir, "/bin/reef", ch.ID, ag.factory)
	if err != nil {
		t.Fatal(err)
	}
	if ag.started {
		t.Error("started after open = true, want false until the first turn")
	}
	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if !ag.started || ag.resumed != "" {
		t.Errorf("started = %v, resumed = %q, want a new session", ag.started, ag.resumed)
	}
	tok := ag.cfg.MCPServers["reef"].Env["REEF_TOKEN"]
	want := agent.Config{
		Cwd: dir + "/worktrees/1/login", PermissionMode: "acceptEdits", Tools: []string{"Task", "mcp__reef__*"},
		MCPServers: map[string]agent.MCPServer{"reef": {
			Command: "/bin/reef", Args: []string{"mcp", "--data", dir},
			Env: map[string]string{"REEF_TOKEN": tok},
		}},
	}
	if !reflect.DeepEqual(ag.cfg, want) {
		t.Errorf("config = %+v, want %+v", ag.cfg, want)
	}
	if got, err := st.Token(ctx, tok); err != nil || got.ChannelID != ch.ID || got.Role != "architect" {
		t.Errorf("Token = %+v, %v, want a token of channel %d for architect", got, err, ch.ID)
	}
	if !InUse(dir, ch.ID) {
		t.Error("InUse while open = false, want true")
	}

	if err := sendAs(ctx, st, ch.ID, s.Run, "hi", discard); err != nil {
		t.Fatal(err)
	}
	if len(ag.prompts) != 1 ||
		!strings.HasPrefix(ag.prompts[0], `You are the architect on feature "Login" (login) in project "shop".`) ||
		!strings.HasSuffix(ag.prompts[0], "\n\nhi") {
		t.Errorf("prompts = %q, want the context before hi", ag.prompts)
	}

	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Token(ctx, tok); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Token after Close error = %v, want ErrNotFound", err)
	}
	if InUse(dir, ch.ID) {
		t.Error("InUse after Close = true, want false")
	}
}

func TestOpen_QuickSession(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch := sessionChannel(t, st, f, true)
	ag := newRecordingAgent()

	s, err := open(ctx, st, t.TempDir(), "/bin/reef", ch.ID, ag.factory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if err := sendAs(ctx, st, ch.ID, s.Run, "hi", discard); err != nil {
		t.Fatal(err)
	}

	p, err := st.Project(ctx, f.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if ag.cfg.Cwd != p.Path {
		t.Errorf("Cwd = %q, want the project path %q", ag.cfg.Cwd, p.Path)
	}
	want := "You are the architect in project \"shop\" (" + p.Path + ").\n" +
		"This is a quick session: it has no feature yet, so there is no brief, notes or decisions.\n" +
		"\n\nhi"
	if len(ag.prompts) != 1 || ag.prompts[0] != want {
		t.Errorf("prompts = %q, want [%q]", ag.prompts, want)
	}
}

func TestOpen_ResumesNewestSession(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch := sessionChannel(t, st, f, false)
	for _, session := range []string{"sess-old", "sess-new", ""} {
		turn, err := st.CreateTurn(ctx, store.Turn{
			ChannelID: ch.ID, Role: "architect", Backend: "claude", SessionID: session,
		})
		if err != nil {
			t.Fatal(err)
		}
		if session == "" {
			continue
		}
		turn.Status = "done"
		if _, err := st.EndTurn(ctx, turn); err != nil {
			t.Fatal(err)
		}
	}
	ag := newRecordingAgent()

	s, err := open(ctx, st, t.TempDir(), "/bin/reef", ch.ID, ag.factory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if ag.started || ag.resumed != "sess-new" {
		t.Errorf("started = %v, resumed = %q, want sess-new resumed", ag.started, ag.resumed)
	}
	if ag.cfg.PermissionMode != "acceptEdits" {
		t.Errorf("permission mode = %q, want acceptEdits", ag.cfg.PermissionMode)
	}
	stale, err := st.Turn(ctx, 3)
	if err != nil {
		t.Fatal(err)
	}
	if stale.Status != "failed" || stale.Error != "chat exited" {
		t.Errorf("stale turn = %q %q, want failed %q", stale.Status, stale.Error, "chat exited")
	}

	if err := sendAs(ctx, st, ch.ID, s.Run, "hi", discard); err != nil {
		t.Fatal(err)
	}
	if len(ag.prompts) != 1 || ag.prompts[0] != "hi" {
		t.Errorf("prompts = %q, want [hi] without the context", ag.prompts)
	}
}

func TestSession_RunAsSpecialist(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	if _, err := st.ApplyTeam(ctx, store.Team{Name: "core", Roles: []store.Role{
		{Name: "lead", Lead: true, Backend: "claude"},
		{
			Name: "architect", Backend: "claude", Persona: "You design.", Skills: "Sketch first.",
			Tools: []string{"Read", "Grep"}, MCPServers: []agent.MCPServer{{URL: "http://x"}},
		},
	}}); err != nil {
		t.Fatal(err)
	}
	ch, err := st.CreateChannel(ctx, store.Channel{
		ProjectID: f.ProjectID, FeatureID: &f.ID, Name: "chat", Role: "lead", Backend: "claude",
		Team: "core",
	})
	if err != nil {
		t.Fatal(err)
	}
	root, err := st.CreateMessage(ctx, store.Message{ChannelID: ch.ID, Author: "user", Body: "root"})
	if err != nil {
		t.Fatal(err)
	}
	var ags []*recordingAgent
	var closeErr error
	mk := func(string) (agent.Agent, error) {
		ag := newRecordingAgent()
		ag.closeErr = closeErr
		ags = append(ags, ag)
		return ag, nil
	}
	s, err := open(ctx, st, t.TempDir(), "/bin/reef", ch.ID, mk)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if len(ags) != 0 {
		t.Fatalf("agents after open = %d, want none", len(ags))
	}

	var tokenRole string
	whoAmI := func(agent.Event) {
		tok := ags[0].cfg.MCPServers["reef"].Env["REEF_TOKEN"]
		if got, err := st.Token(ctx, tok); err == nil {
			tokenRole = got.Role
		}
	}
	if err := s.RunAs(ctx, "architect", "plan it", &root.ID, whoAmI); err != nil {
		t.Fatal(err)
	}
	want := "You design.\n\nYour skills:\nSketch first.\n\nYou are the architect on feature"
	if len(ags) != 1 || !strings.HasPrefix(ags[0].prompts[0], want) {
		t.Errorf("prompts = %q, want one starting with %q", ags[0].prompts, want)
	}
	if !reflect.DeepEqual(ags[0].cfg.Tools, []string{"Read", "Grep", "mcp__reef__*"}) ||
		ags[0].cfg.MCPServers["mcp1"].URL != "http://x" {
		t.Errorf("config = %+v, want the role's tools and mcp server", ags[0].cfg)
	}
	if tokenRole != "architect" {
		t.Errorf("token role = %q, want architect", tokenRole)
	}
	if thread, err := st.Thread(ctx, ch.ID, root.ID); err != nil || len(thread) != 2 ||
		thread[1].Author != "architect" {
		t.Errorf("thread = %+v, %v, want the root and the architect's reply", thread, err)
	}
	if !ags[0].closed {
		t.Error("architect closed after its turn = false, want true")
	}

	if err := s.RunAs(ctx, "architect", "again", nil, discard); err != nil {
		t.Fatal(err)
	}
	if ags[1].resumed != "s1" || ags[1].prompts[0] != "again" {
		t.Errorf("resumed = %q, prompts = %q, want s1 and [again]", ags[1].resumed, ags[1].prompts)
	}
	for range 2 {
		if err := s.Run(ctx, "lead turn", nil, discard); err != nil {
			t.Fatal(err)
		}
	}
	if len(ags) != 3 || ags[2].closed || len(ags[2].prompts) != 2 {
		t.Errorf("agents = %d, want the lead started once at its first turn and still open", len(ags))
	}
	if err := s.RunAs(ctx, "tester", "hi", nil, discard); !errors.Is(err, store.ErrInvalid) {
		t.Errorf("RunAs unknown role error = %v, want ErrInvalid", err)
	}

	closeErr = errors.New("close failed")
	if err := s.RunAs(ctx, "architect", "last", nil, discard); err != nil {
		t.Errorf("RunAs error = %v, want nil: a failed close does not fail a good turn", err)
	}
}

func TestSession_RunAsRoleAppliedAfterOpen(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	lead := store.Role{Name: "lead", Lead: true, Backend: "claude"}
	if _, err := st.ApplyTeam(ctx, store.Team{Name: "core", Roles: []store.Role{lead}}); err != nil {
		t.Fatal(err)
	}
	ch, err := st.CreateChannel(ctx, store.Channel{
		ProjectID: f.ProjectID, FeatureID: &f.ID, Name: "chat", Role: "lead", Backend: "claude",
		Team: "core",
	})
	if err != nil {
		t.Fatal(err)
	}
	var ags []*recordingAgent
	mk := func(string) (agent.Agent, error) {
		ag := newRecordingAgent()
		ags = append(ags, ag)
		return ag, nil
	}
	s, err := open(ctx, st, t.TempDir(), "/bin/reef", ch.ID, mk)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}

	tester := store.Role{Name: "tester", Backend: "claude", Persona: "You test."}
	if _, err := st.ApplyTeam(ctx, store.Team{Name: "core", Roles: []store.Role{lead, tester}}); err != nil {
		t.Fatal(err)
	}
	if err := s.RunAs(ctx, "tester", "check it", nil, discard); err != nil {
		t.Fatalf("RunAs tester error = %v, want nil", err)
	}
	if len(ags) != 2 || !strings.HasPrefix(ags[1].prompts[0], "You test.") {
		t.Errorf("agents = %d, tester prompts = %q, want a tester turn with its persona", len(ags), ags[1].prompts)
	}
}

func TestSession_RunAsRoleChangedBackend(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	lead := store.Role{Name: "lead", Lead: true, Backend: "claude"}
	architect := store.Role{Name: "architect", Backend: "claude", Persona: "You design."}
	if _, err := st.ApplyTeam(ctx, store.Team{Name: "core", Roles: []store.Role{lead, architect}}); err != nil {
		t.Fatal(err)
	}
	ch, err := st.CreateChannel(ctx, store.Channel{
		ProjectID: f.ProjectID, FeatureID: &f.ID, Name: "chat", Role: "lead", Backend: "claude",
		Team: "core",
	})
	if err != nil {
		t.Fatal(err)
	}
	var ags []*recordingAgent
	var backends []string
	mk := func(backend string) (agent.Agent, error) {
		ag := newRecordingAgent()
		ags = append(ags, ag)
		backends = append(backends, backend)
		return ag, nil
	}
	s, err := open(ctx, st, t.TempDir(), "/bin/reef", ch.ID, mk)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if err := s.RunAs(ctx, "architect", "plan it", nil, discard); err != nil {
		t.Fatal(err)
	}

	architect.Backend = "codex"
	if _, err := st.ApplyTeam(ctx, store.Team{Name: "core", Roles: []store.Role{lead, architect}}); err != nil {
		t.Fatal(err)
	}
	if err := s.RunAs(ctx, "architect", "again", nil, discard); err != nil {
		t.Fatal(err)
	}
	if len(ags) != 2 || backends[1] != "codex" || !ags[1].started || ags[1].resumed != "" ||
		!strings.HasPrefix(ags[1].prompts[0], "You design.") {
		t.Errorf("second architect: backend %q, started = %v, resumed = %q, prompts = %q, "+
			"want codex, started fresh with the start prompt", backends[1], ags[1].started,
			ags[1].resumed, ags[1].prompts)
	}
}

func TestOpen_Errors(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch := sessionChannel(t, st, f, false)
	bad, err := st.CreateChannel(ctx, store.Channel{
		ProjectID: f.ProjectID, Name: "typo", Role: "architect", Backend: "nope",
	})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		give int64
		want string
	}{
		{"unknown channel", 99, "channel 99: not found"},
		{"unknown backend", bad.ID, `unknown backend "nope"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := Open(ctx, st, t.TempDir(), "/bin/reef", tt.give, NewTrees())
			if err == nil {
				err = s.Start(ctx)
				_ = s.Close()
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Open error = %v, want %q", err, tt.want)
			}
		})
	}

	t.Run("open elsewhere", func(t *testing.T) {
		dir := t.TempDir()
		s, err := open(ctx, st, dir, "/bin/reef", ch.ID, newRecordingAgent().factory)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = s.Close() }()
		_, err = open(ctx, st, dir, "/bin/reef", ch.ID, newRecordingAgent().factory)
		if err == nil || err.Error() != "channel 1 is open elsewhere: conflict" {
			t.Errorf("second open error = %v, want the open-elsewhere error", err)
		}
	})

	t.Run("agent does not start", func(t *testing.T) {
		dir := t.TempDir()
		ag := newRecordingAgent()
		ag.startErr = errors.New("no claude")
		s, err := open(ctx, st, dir, "/bin/reef", ch.ID, ag.factory)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Run(ctx, "hi", nil, discard); err == nil || err.Error() != "no claude" {
			t.Errorf("Run error = %v, want no claude", err)
		}
		turns, err := st.Turns(ctx, store.TurnFilter{ChannelID: &ch.ID})
		if err != nil {
			t.Fatal(err)
		}
		if len(turns) != 1 || turns[0].Role != "architect" || turns[0].Status != "failed" ||
			turns[0].Error != "no claude" || turns[0].WakeText != "hi" {
			t.Errorf("turns = %+v, want one failed architect turn with error no claude, waking on hi", turns)
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
		if InUse(dir, ch.ID) {
			t.Error("InUse after Close = true, want false")
		}
		tok := ag.cfg.MCPServers["reef"].Env["REEF_TOKEN"]
		if _, err := st.Token(ctx, tok); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("Token after failed open error = %v, want ErrNotFound", err)
		}
	})
}

func TestSession_Wrap(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch := sessionChannel(t, st, f, false)
	ag := newRecordingAgent()
	ag.st, ag.channelID, ag.propose = st, ch.ID, true
	s, err := open(ctx, st, t.TempDir(), "/bin/reef", ch.ID, ag.factory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	p, err := s.Wrap(ctx, discard)
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != 1 || p.Brief != "new brief" {
		t.Errorf("proposal = %+v, want proposal 1 with the new brief", p)
	}
	msgs, err := st.Messages(ctx, ch.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 || msgs[0].Author != "architect" {
		t.Errorf("messages = %+v, want only the agent's reply, no user message", msgs)
	}
	if !strings.HasSuffix(ag.prompts[0], _wrapPrompt) {
		t.Errorf("prompt = %q, want it to end with the wrap prompt", ag.prompts[0])
	}

	if turns, err := st.Turns(ctx, store.TurnFilter{ChannelID: &ch.ID}); err != nil || len(turns) != 1 || turns[0].WakeText != "" {
		t.Errorf("turns = %+v, %v, want one turn that kept no wake", turns, err)
	}

	ag.propose = false
	if _, err := s.Wrap(ctx, discard); !errors.Is(err, ErrNoProposal) {
		t.Errorf("Wrap with an older proposal pending error = %v, want ErrNoProposal", err)
	}
}

func TestSession_Promote(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch := sessionChannel(t, st, f, true)
	ag := newRecordingAgent()
	ag.st, ag.channelID, ag.propose = st, ch.ID, true
	s, err := open(ctx, st, t.TempDir(), "/bin/reef", ch.ID, ag.factory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	if _, err := s.Promote(ctx, discard); !errors.Is(err, store.ErrInvalid) {
		t.Errorf("Promote before the channel has a feature error = %v, want ErrInvalid", err)
	}
	if _, err := st.PromoteChannel(ctx, ch.ID, store.Feature{Slug: "idea", Name: "Big idea"}); err != nil {
		t.Fatal(err)
	}
	p, err := s.Promote(ctx, discard)
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != 1 || p.Brief != "new brief" {
		t.Errorf("proposal = %+v, want proposal 1 with the new brief", p)
	}
	msgs, err := st.Messages(ctx, ch.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 || msgs[0].Author != "architect" {
		t.Errorf("messages = %+v, want only the agent's reply, no user message", msgs)
	}
	want := `This quick session is now feature "Big idea" (idea). Call the propose_wrap tool once`
	if len(ag.prompts) != 1 || !strings.Contains(ag.prompts[0], want) {
		t.Errorf("prompts = %q, want one with %q", ag.prompts, want)
	}
}

func TestSession_PromoteMovesNextWakeToWorktree(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	p, err := st.Project(ctx, f.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	ch := sessionChannel(t, st, f, true)
	var ags []*recordingAgent
	mk := func(string) (agent.Agent, error) {
		ag := newRecordingAgent()
		ag.st, ag.channelID, ag.propose = st, ch.ID, true
		ags = append(ags, ag)
		return ag, nil
	}
	dir := t.TempDir()
	s, err := open(ctx, st, dir, "/bin/reef", ch.ID, mk)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if err := s.Run(ctx, "hi", nil, discard); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.Path, "wip.txt"), []byte("wip"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PromoteChannel(ctx, ch.ID, store.Feature{Slug: "idea", Name: "Big idea"}); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Promote(ctx, discard); err != nil {
		t.Fatal(err)
	}
	if err := s.Run(ctx, "go on", nil, discard); err != nil {
		t.Fatal(err)
	}

	worktree := dir + "/worktrees/1/idea"
	if len(ags) != 2 || ags[0].cfg.Cwd != p.Path || !ags[0].closed ||
		ags[1].cfg.Cwd != worktree || ags[1].resumed != "s1" {
		t.Errorf("agents = %+v, want the first in %s and closed by the promote, the second resumed in %s",
			ags, p.Path, worktree)
	}
	msgs, err := st.Messages(ctx, ch.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := "Uncommitted changes stay on the main checkout. This feature works in " + worktree +
		" on feature/idea."
	if len(msgs) < 2 || msgs[1].Author != "reef" || msgs[1].Body != want {
		t.Errorf("messages = %+v, want the second from reef saying %q", msgs, want)
	}
}

func TestSession_NoWorktreeFailsTurn(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	p, err := st.CreateProject(ctx, store.Project{Name: "notes", Path: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	f, err = st.CreateFeature(ctx, store.Feature{ProjectID: p.ID, Slug: "docs", Name: "Docs"})
	if err != nil {
		t.Fatal(err)
	}
	ch := sessionChannel(t, st, f, false)
	ag := newRecordingAgent()
	s, err := open(ctx, st, t.TempDir(), "/bin/reef", ch.ID, ag.factory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	err = s.Run(ctx, "hi", nil, discard)
	if err == nil || !strings.Contains(err.Error(), "not a git repository") {
		t.Fatalf("Run error = %v, want one saying not a git repository", err)
	}
	turns, err := st.Turns(ctx, store.TurnFilter{ChannelID: &ch.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 1 || turns[0].Status != "failed" ||
		!strings.Contains(turns[0].Error, "not a git repository") || ag.started {
		t.Errorf("turns = %+v, started = %v, want one failed turn and no agent started", turns, ag.started)
	}
}

func TestSession_RecreatesRemovedWorktree(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch := sessionChannel(t, st, f, false)
	var ags []*recordingAgent
	mk := func(string) (agent.Agent, error) {
		ag := newRecordingAgent()
		ags = append(ags, ag)
		return ag, nil
	}
	dir := t.TempDir()
	s, err := open(ctx, st, dir, "/bin/reef", ch.ID, mk)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if err := s.Run(ctx, "hi", nil, discard); err != nil {
		t.Fatal(err)
	}
	tree := dir + "/worktrees/1/login"
	if err := os.RemoveAll(tree); err != nil {
		t.Fatal(err)
	}

	if err := s.Run(ctx, "again", nil, discard); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(tree + "/.git"); err != nil {
		t.Errorf("worktree after the second wake: %v, want it made again", err)
	}
	if len(ags) != 2 || !ags[0].closed || ags[1].cfg.Cwd != tree || len(ags[1].prompts) != 1 {
		t.Errorf("agents = %+v, want the first closed and a second in %s that got the wake", ags, tree)
	}
	turns, err := st.Turns(ctx, store.TurnFilter{ChannelID: &ch.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 2 || turns[0].Status != "done" || turns[1].Status != "done" {
		t.Errorf("turns = %+v, want two done", turns)
	}
}

func TestSession_WrapOrPromoteCancelledStoresTurn(t *testing.T) {
	tests := []struct {
		name string
		give func(s *Session, ctx context.Context) error
	}{
		{"wrap", func(s *Session, ctx context.Context) error {
			_, err := s.Wrap(ctx, discard)
			return err
		}},
		{"promote", func(s *Session, ctx context.Context) error {
			_, err := s.Promote(ctx, discard)
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bg := context.Background()
			st, f := openStore(t)
			ch := sessionChannel(t, st, f, false)
			ag := newRecordingAgent()
			s, err := open(bg, st, t.TempDir(), "/bin/reef", ch.ID, ag.factory)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = s.Close() }()
			ctx, cancel := context.WithCancel(bg)
			cancel()

			if err := tt.give(s, ctx); !errors.Is(err, context.Canceled) {
				t.Fatalf("error = %v, want context canceled", err)
			}
			if len(ag.prompts) != 0 {
				t.Errorf("prompts = %q, want none", ag.prompts)
			}
			turns, err := st.Turns(bg, store.TurnFilter{ChannelID: &ch.ID})
			if err != nil {
				t.Fatal(err)
			}
			if len(turns) != 1 || turns[0].Status != "failed" || turns[0].Error != "interrupted" || turns[0].WakeText != "" {
				t.Errorf("turns = %+v, want one failed interrupted turn that kept no wake", turns)
			}
		})
	}
}

func TestSession_PromoteIntoFeatureWithBrief(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	if _, err := st.PutBrief(ctx, store.Brief{FeatureID: f.ID, Body: "old brief", Author: "user"}); err != nil {
		t.Fatal(err)
	}
	ch := sessionChannel(t, st, f, true)
	ag := newRecordingAgent()
	ag.st, ag.channelID, ag.propose = st, ch.ID, true
	s, err := open(ctx, st, t.TempDir(), "/bin/reef", ch.ID, ag.factory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := st.PromoteChannel(ctx, ch.ID, store.Feature{ID: f.ID}); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Promote(ctx, discard); err != nil {
		t.Fatal(err)
	}
	want := `This quick session is now part of feature "Login" (login), which already has a brief. ` +
		`Call the get_brief tool first.`
	if len(ag.prompts) != 1 || !strings.Contains(ag.prompts[0], want) {
		t.Errorf("prompts = %q, want one with %q", ag.prompts, want)
	}
}
