package channel

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/store"
)

type recordingAgent struct {
	st        *store.Store
	channelID int64
	propose   bool
	kind      string
	startErr  error
	resumeErr error
	closeErr  error
	sendError string
	session   string
	// handshake, when set, is the session event the agent sends at Start, as codex does, and
	// Send then sends none.
	handshake *agent.Event
	// exitText, when set, makes Resume leave it as an error event and close Events, and Send
	// fail with it as claude's does.
	exitText string

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
	if a.handshake != nil {
		a.events <- *a.handshake
	}
	return a.startErr
}

func (a *recordingAgent) Resume(_ context.Context, cfg agent.Config, sessionID string) error {
	a.cfg, a.resumed = cfg, sessionID
	if a.exitText != "" {
		a.events <- agent.Event{Kind: agent.KindError, Text: a.exitText}
		close(a.events)
	}
	return a.resumeErr
}

func (a *recordingAgent) Events() <-chan agent.Event { return a.events }
func (a *recordingAgent) Interrupt() error           { a.closed = true; return a.closeErr }

func (a *recordingAgent) Send(ctx context.Context, prompt string) error {
	a.prompts = append(a.prompts, prompt)
	if a.exitText != "" {
		err := errors.New("write prompt: broken pipe")
		for ev := range a.events {
			if ev.Kind == agent.KindError {
				return fmt.Errorf("%w: %s", err, ev.Text)
			}
		}
		return err
	}
	if strings.Contains(prompt, "propose_wrap") && a.propose {
		_, err := a.st.CreateProposal(ctx, store.Proposal{
			ChannelID: a.channelID, Role: "architect", Kind: a.kind, Brief: "new brief",
			Notes: "my notes",
		})
		if err != nil {
			return err
		}
	}
	if a.sendError != "" {
		a.events <- agent.Event{Kind: agent.KindError, Text: a.sendError}
		return nil
	}
	id := a.session
	if id == "" {
		id = "s1"
	}
	if a.handshake == nil {
		a.events <- agent.Event{Kind: agent.KindSession, SessionID: id}
	}
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

	s, err := Open(ctx, st, dir, "/bin/reef", ch.ID, NewTrees(t.TempDir()), ag.factory)
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
		!strings.HasPrefix(ag.prompts[0], "## You\nYou are the architect on feature \"Login\" (login) in project \"shop\".") ||
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

	s, err := Open(ctx, st, t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), ag.factory)
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
	want := "## You\nYou are the architect in project \"shop\" (" + p.Path + ").\n" +
		"This is a quick session: it has no feature yet, so there is no brief, notes or decisions.\n" +
		"\n## How to work\n" + _toolsLine + "\n\n\nhi"
	if len(ag.prompts) != 1 || ag.prompts[0] != want {
		t.Errorf("prompts = %q, want [%q]", ag.prompts, want)
	}
}

func TestOpen_QuickSession_ProjectMemory(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	if _, err := st.PutProjectMemory(ctx,
		store.ProjectMemory{ProjectID: f.ProjectID, Body: "Use Go, not Python."}); err != nil {
		t.Fatal(err)
	}
	ch := sessionChannel(t, st, f, true)
	ag := newRecordingAgent()

	s, err := Open(ctx, st, t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), ag.factory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if err := sendAs(ctx, st, ch.ID, s.Run, "hi", discard); err != nil {
		t.Fatal(err)
	}

	if len(ag.prompts) != 1 || !strings.Contains(ag.prompts[0], "\n## What we know\n### Project memory\nUse Go, not Python.\n") {
		t.Errorf("prompts = %q, want it to contain the project memory section", ag.prompts)
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

	s, err := Open(ctx, st, t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), ag.factory)
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
	t.Setenv("HOME", t.TempDir())
	st, f := openStore(t)
	if _, err := st.ApplyTeam(ctx, store.Team{Name: "core", Roles: []store.Role{
		{Name: "lead", Lead: true, Backend: "claude"},
		{
			Name: "architect", Backend: "claude", Persona: "You design.", Skills: []string{"sketch"},
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
	s, err := Open(ctx, st, t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), mk)
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
	want := "## You\nYou design.\n\nYour skills:\nsketch\n\nYou are the architect on feature"
	if len(ags) != 1 || !strings.HasPrefix(ags[0].prompts[0], want) {
		t.Errorf("prompts = %q, want one starting with %q", ags[0].prompts, want)
	}
	if !reflect.DeepEqual(ags[0].cfg.Tools, []string{"Read", "Grep", "Skill(sketch)", "mcp__reef__*"}) ||
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
	s, err := Open(ctx, st, t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), mk)
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
	if len(ags) != 2 || !strings.HasPrefix(ags[1].prompts[0], "## You\nYou test.") {
		t.Errorf("agents = %d, tester prompts = %q, want a tester turn with its persona", len(ags), ags[1].prompts)
	}
}

func TestSession_PersonaEditReachesResumedSession(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	lead := store.Role{Name: "lead", Lead: true, Backend: "claude"}
	developer := store.Role{Name: "developer", Write: true, Backend: "claude", Persona: "You code."}
	if _, err := st.ApplyTeam(ctx, store.Team{Name: "core", Roles: []store.Role{lead, developer}}); err != nil {
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
	s, err := Open(ctx, st, t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), mk)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if err := s.RunAs(ctx, "developer", "build it", nil, discard); err != nil {
		t.Fatal(err)
	}

	developer.Persona = "You code in Go only."
	if _, err := st.ApplyTeam(ctx, store.Team{Name: "core", Roles: []store.Role{lead, developer}}); err != nil {
		t.Fatal(err)
	}
	if err := s.RunAs(ctx, "developer", "build more", nil, discard); err != nil {
		t.Fatal(err)
	}
	if len(ags) != 2 || ags[1].resumed != "s1" || !strings.Contains(ags[1].prompts[0], "You code in Go only.") {
		t.Errorf("second developer: resumed = %q, prompts = %q, want resumed with the new persona",
			ags[1].resumed, ags[1].prompts)
	}

	turns, err := st.Turns(ctx, store.TurnFilter{ChannelID: &ch.ID})
	if err != nil {
		t.Fatal(err)
	}
	var last store.Turn
	for _, turn := range turns {
		if turn.Role == "developer" {
			last = turn
		}
	}
	if !strings.Contains(last.Prompt, "You code in Go only.") {
		t.Errorf("stored turn.prompt = %q, want it to contain the new persona", last.Prompt)
	}
}

func TestSession_MissingSessionStartsNewOne(t *testing.T) {
	const intro = "You lead.\n\n"
	tests := []struct {
		name string
		gone func(*recordingAgent)
	}{
		{"claude reports it on the first turn", func(ag *recordingAgent) {
			ag.sendError = "No conversation found with session ID: s1"
		}},
		{"claude exits on the resume and says why before the first turn", func(ag *recordingAgent) {
			ag.exitText = "No conversation found with session ID: s1"
		}},
		{"codex refuses the resume", func(ag *recordingAgent) {
			ag.resumeErr = errors.New("thread/resume: no rollout found for thread id s1")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			st, f := openStore(t)
			lead := store.Role{Name: "lead", Lead: true, Backend: "claude", Persona: "You lead."}
			if _, err := st.ApplyTeam(ctx, store.Team{Name: "core", Roles: []store.Role{lead}}); err != nil {
				t.Fatal(err)
			}
			ch, err := st.CreateChannel(ctx, store.Channel{
				ProjectID: f.ProjectID, FeatureID: &f.ID, Name: "chat", Role: "lead",
				Backend: "claude", Team: "core",
			})
			if err != nil {
				t.Fatal(err)
			}
			var ags []*recordingAgent
			mk := func(string) (agent.Agent, error) {
				ag := newRecordingAgent()
				switch len(ags) {
				case 1:
					tt.gone(ag)
				case 2, 3:
					ag.session = "s2"
				}
				ags = append(ags, ag)
				return ag, nil
			}
			dir := t.TempDir()
			trees := NewTrees(t.TempDir())
			s, err := Open(ctx, st, dir, "/bin/reef", ch.ID, trees, mk)
			if err != nil {
				t.Fatal(err)
			}
			if err := s.Run(ctx, "hi", nil, discard); err != nil {
				t.Fatal(err)
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}

			s, err = Open(ctx, st, dir, "/bin/reef", ch.ID, trees, mk)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = s.Close() }()
			if err := s.Run(ctx, "again", nil, discard); err == nil {
				t.Fatal("run on a missing session = nil, want an error")
			}
			if err := s.Run(ctx, "once more", nil, discard); err != nil {
				t.Fatal(err)
			}
			if len(ags) != 3 || ags[1].resumed != "s1" || !ags[2].started || ags[2].resumed != "" {
				t.Fatalf("agents = %d, second resumed %q, third started = %v, resumed %q; want 3, s1, true, empty",
					len(ags), ags[1].resumed, ags[2].started, ags[2].resumed)
			}
			if len(ags[2].prompts) != 1 || !strings.HasPrefix(ags[2].prompts[0], "## You\n"+intro) ||
				!strings.HasSuffix(ags[2].prompts[0], "\n\nonce more") {
				t.Errorf("new session prompts = %q, want the role intro before once more", ags[2].prompts)
			}

			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			s, err = Open(ctx, st, dir, "/bin/reef", ch.ID, trees, mk)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = s.Close() }()
			if err := s.Run(ctx, "next day", nil, discard); err != nil {
				t.Fatal(err)
			}
			if len(ags) != 4 || ags[3].resumed != "s2" || ags[3].started {
				t.Errorf("agents = %d, last resumed %q; want 4 and the new session s2", len(ags), ags[3].resumed)
			}
		})
	}
}

func TestSession_LeadSessionFromStartIsStored(t *testing.T) {
	tests := []struct {
		name  string
		start bool
	}{
		{"start then run", true},
		{"run alone", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			st, f := openStore(t)
			ch := sessionChannel(t, st, f, false)
			ag := newRecordingAgent()
			ag.handshake = &agent.Event{Kind: agent.KindSession, SessionID: "c1", Model: "gpt-5"}
			s, err := Open(ctx, st, t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), ag.factory)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = s.Close() }()
			if tt.start {
				if err := s.Start(ctx); err != nil {
					t.Fatal(err)
				}
			}
			if err := s.Run(ctx, "go", nil, discard); err != nil {
				t.Fatal(err)
			}
			turns, err := st.Turns(ctx, store.TurnFilter{ChannelID: &ch.ID})
			if err != nil {
				t.Fatal(err)
			}
			if len(turns) != 1 || turns[0].SessionID != "c1" || turns[0].Model != "gpt-5" {
				t.Errorf("turns = %+v, want one with session c1 and model gpt-5", turns)
			}
		})
	}
}

func TestSession_PersonaEditReachesLiveLead(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	lead := store.Role{Name: "lead", Lead: true, Backend: "claude", Persona: "You lead."}
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
	s, err := Open(ctx, st, t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), mk)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.Run(ctx, "hi", nil, discard); err != nil {
		t.Fatal(err)
	}

	lead.Persona = "You lead with care."
	if _, err := st.ApplyTeam(ctx, store.Team{Name: "core", Roles: []store.Role{lead}}); err != nil {
		t.Fatal(err)
	}
	if err := s.Run(ctx, "again", nil, discard); err != nil {
		t.Fatal(err)
	}
	if len(ags) != 1 || len(ags[0].prompts) != 2 || !strings.Contains(ags[0].prompts[1], "You lead with care.") {
		t.Errorf("second lead prompt = %q, want the lead kept live with the new persona",
			ags[0].prompts)
	}
}

func TestSession_WriteEditReachesLiveLead(t *testing.T) {
	for _, tc := range []struct {
		name          string
		before, after bool
	}{
		{"granted", false, true}, {"taken away", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			st, f := openStore(t)
			lead := store.Role{Name: "lead", Lead: true, Backend: "claude", Write: tc.before}
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
			s, err := Open(ctx, st, t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), mk)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = s.Close() }()
			if err := s.Run(ctx, "hi", nil, discard); err != nil {
				t.Fatal(err)
			}

			lead.Write = tc.after
			if _, err := st.ApplyTeam(ctx, store.Team{Name: "core", Roles: []store.Role{lead}}); err != nil {
				t.Fatal(err)
			}
			if err := s.Run(ctx, "again", nil, discard); err != nil {
				t.Fatal(err)
			}
			if len(ags) != 2 || !ags[0].closed || ags[1].resumed != "s1" {
				t.Fatalf("agents = %d, first closed = %v, second resumed %q; want 2, true, s1",
					len(ags), ags[0].closed, ags[1].resumed)
			}
			cfg := ags[1].cfg
			if got := !slices.Contains(cfg.DisallowedTools, "Edit"); got != tc.after {
				t.Errorf("next turn can edit = %v, want %v (disallowed %v)", got, tc.after, cfg.DisallowedTools)
			}
			if got := s.members["lead"].run.CheckpointDir != ""; got != tc.after {
				t.Errorf("next turn has a checkpoint = %v, want %v", got, tc.after)
			}
		})
	}
}

func TestSession_PersonaUnchangedNoRoleIntroOnResume(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	lead := store.Role{Name: "lead", Lead: true, Backend: "claude"}
	developer := store.Role{Name: "developer", Write: true, Backend: "claude", Persona: "You code."}
	if _, err := st.ApplyTeam(ctx, store.Team{Name: "core", Roles: []store.Role{lead, developer}}); err != nil {
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
	s, err := Open(ctx, st, t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), mk)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if err := s.RunAs(ctx, "developer", "build it", nil, discard); err != nil {
		t.Fatal(err)
	}
	if err := s.RunAs(ctx, "developer", "build more", nil, discard); err != nil {
		t.Fatal(err)
	}
	if len(ags) != 2 || ags[1].resumed != "s1" || ags[1].prompts[0] != "build more" {
		t.Errorf("second developer: resumed = %q, prompts = %q, want [build more] without a role intro",
			ags[1].resumed, ags[1].prompts)
	}
}

func TestSession_NoEditFourResumedWakesOnlyFirstHasIntro(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	lead := store.Role{Name: "lead", Lead: true, Backend: "claude"}
	developer := store.Role{Name: "developer", Write: true, Backend: "claude", Persona: "You code."}
	if _, err := st.ApplyTeam(ctx, store.Team{Name: "core", Roles: []store.Role{lead, developer}}); err != nil {
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
	s, err := Open(ctx, st, t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), mk)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	for i, wake := range []string{"wake 1", "wake 2", "wake 3", "wake 4"} {
		if err := s.RunAs(ctx, "developer", wake, nil, discard); err != nil {
			t.Fatalf("wake %d: %v", i+1, err)
		}
	}
	if len(ags) != 4 {
		t.Fatalf("agents = %d, want 4", len(ags))
	}
	if len(ags[0].prompts) != 1 || !strings.Contains(ags[0].prompts[0], "You code.") ||
		!strings.HasSuffix(ags[0].prompts[0], "wake 1") {
		t.Errorf("wake 1: prompts = %q, want the persona and wake 1", ags[0].prompts)
	}
	want := []string{"wake 2", "wake 3", "wake 4"}
	for i, ag := range ags[1:] {
		if len(ag.prompts) != 1 || ag.prompts[0] != want[i] {
			t.Errorf("wake %d: prompts = %q, want [%q] without a role intro", i+2, ag.prompts, want[i])
		}
	}
}

func TestSession_EditAfterSecondResumedWakeUpdatesOnceThenNone(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	lead := store.Role{Name: "lead", Lead: true, Backend: "claude"}
	developer := store.Role{Name: "developer", Write: true, Backend: "claude", Persona: "You code."}
	if _, err := st.ApplyTeam(ctx, store.Team{Name: "core", Roles: []store.Role{lead, developer}}); err != nil {
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
	s, err := Open(ctx, st, t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), mk)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if err := s.RunAs(ctx, "developer", "wake 1", nil, discard); err != nil {
		t.Fatal(err)
	}
	if err := s.RunAs(ctx, "developer", "wake 2", nil, discard); err != nil {
		t.Fatal(err)
	}
	developer.Persona = "You code in Go only."
	if _, err := st.ApplyTeam(ctx, store.Team{Name: "core", Roles: []store.Role{lead, developer}}); err != nil {
		t.Fatal(err)
	}
	if err := s.RunAs(ctx, "developer", "wake 3", nil, discard); err != nil {
		t.Fatal(err)
	}
	if err := s.RunAs(ctx, "developer", "wake 4", nil, discard); err != nil {
		t.Fatal(err)
	}
	if len(ags) != 4 {
		t.Fatalf("agents = %d, want 4", len(ags))
	}
	want3 := "Your persona or skills changed:\n\nYou code in Go only.\n\n\n\nwake 3"
	if len(ags[2].prompts) != 1 || ags[2].prompts[0] != want3 {
		t.Errorf("wake 3: prompts = %q, want [%q]", ags[2].prompts, want3)
	}
	if len(ags[3].prompts) != 1 || ags[3].prompts[0] != "wake 4" {
		t.Errorf("wake 4: prompts = %q, want [wake 4]", ags[3].prompts)
	}
}

func TestSession_NoEditFourLiveWakesOnlyFirstHasIntro(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	lead := store.Role{Name: "lead", Lead: true, Backend: "claude", Persona: "You lead."}
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
	s, err := Open(ctx, st, t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), mk)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}
	for i, wake := range []string{"wake 1", "wake 2", "wake 3", "wake 4"} {
		if err := s.Run(ctx, wake, nil, discard); err != nil {
			t.Fatalf("wake %d: %v", i+1, err)
		}
	}
	if len(ags) != 1 {
		t.Fatalf("agents = %d, want 1", len(ags))
	}
	if len(ags[0].prompts) != 4 {
		t.Fatalf("prompts = %q, want 4", ags[0].prompts)
	}
	if !strings.Contains(ags[0].prompts[0], "You lead.") || !strings.HasSuffix(ags[0].prompts[0], "wake 1") {
		t.Errorf("wake 1: prompt = %q, want the persona and wake 1", ags[0].prompts[0])
	}
	want := []string{"wake 2", "wake 3", "wake 4"}
	for i, p := range ags[0].prompts[1:] {
		if p != want[i] {
			t.Errorf("wake %d: prompt = %q, want %q without a role intro", i+2, p, want[i])
		}
	}
}

func TestSession_EditAfterSecondLiveWakeUpdatesOnceThenNone(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	lead := store.Role{Name: "lead", Lead: true, Backend: "claude", Persona: "You lead."}
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
	s, err := Open(ctx, st, t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), mk)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.Run(ctx, "wake 1", nil, discard); err != nil {
		t.Fatal(err)
	}
	if err := s.Run(ctx, "wake 2", nil, discard); err != nil {
		t.Fatal(err)
	}
	lead.Persona = "You lead with care."
	if _, err := st.ApplyTeam(ctx, store.Team{Name: "core", Roles: []store.Role{lead}}); err != nil {
		t.Fatal(err)
	}
	if err := s.Run(ctx, "wake 3", nil, discard); err != nil {
		t.Fatal(err)
	}
	if err := s.Run(ctx, "wake 4", nil, discard); err != nil {
		t.Fatal(err)
	}
	if len(ags) != 1 || len(ags[0].prompts) != 4 {
		t.Fatalf("agents = %d, prompts = %q, want 1 agent with 4 prompts", len(ags), ags[0].prompts)
	}
	want3 := "Your persona or skills changed:\n\nYou lead with care.\n\n\n\nwake 3"
	if ags[0].prompts[2] != want3 {
		t.Errorf("wake 3: prompt = %q, want %q", ags[0].prompts[2], want3)
	}
	if ags[0].prompts[3] != "wake 4" {
		t.Errorf("wake 4: prompt = %q, want wake 4", ags[0].prompts[3])
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
	s, err := Open(ctx, st, t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), mk)
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
		!strings.HasPrefix(ags[1].prompts[0], "## You\nYou design.") {
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
			s, err := Open(ctx, st, t.TempDir(), "/bin/reef", tt.give, NewTrees(t.TempDir()), NewAgent)
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
		s, err := Open(ctx, st, dir, "/bin/reef", ch.ID, NewTrees(t.TempDir()), newRecordingAgent().factory)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = s.Close() }()
		_, err = Open(ctx, st, dir, "/bin/reef", ch.ID, NewTrees(t.TempDir()), newRecordingAgent().factory)
		if err == nil || err.Error() != "channel 1 is open elsewhere: conflict" {
			t.Errorf("second open error = %v, want the open-elsewhere error", err)
		}
	})

	t.Run("agent does not start", func(t *testing.T) {
		dir := t.TempDir()
		ag := newRecordingAgent()
		ag.startErr = errors.New("no claude")
		s, err := Open(ctx, st, dir, "/bin/reef", ch.ID, NewTrees(t.TempDir()), ag.factory)
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
	s, err := Open(ctx, st, t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), ag.factory)
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
	if !strings.HasSuffix(ag.prompts[0], _wrapPrompt) || strings.Contains(ag.prompts[0], "[direct") {
		t.Errorf("prompt = %q, want it to end with the wrap prompt and list no direct exchange", ag.prompts[0])
	}

	if turns, err := st.Turns(ctx, store.TurnFilter{ChannelID: &ch.ID}); err != nil || len(turns) != 1 || turns[0].WakeText != "" {
		t.Errorf("turns = %+v, %v, want one turn that kept no wake", turns, err)
	}

	ag.propose = false
	if _, err := s.Wrap(ctx, discard); !errors.Is(err, ErrNoProposal) {
		t.Errorf("Wrap with an older proposal pending error = %v, want ErrNoProposal", err)
	}
}

func TestSession_WrapIncludesDirectExchanges(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch := sessionChannel(t, st, f, false)
	req, err := st.CreateDirectMessage(ctx, store.DirectMessage{
		ChannelID: ch.ID, From: "developer", To: "tester", Body: "use the sqlite fixture"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateFailedDirectResponse(ctx, req.ID, "tester", "half an answer"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateDirectResponse(ctx, req.ID, "tester", "fixture works"); err != nil {
		t.Fatal(err)
	}
	ag := newRecordingAgent()
	ag.st, ag.channelID, ag.propose = st, ch.ID, true
	s, err := Open(ctx, st, t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), ag.factory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	if _, err := s.Wrap(ctx, discard); err != nil {
		t.Fatal(err)
	}
	want := "Direct exchanges in this channel, private between two teammates:\n" +
		"[direct 1] developer -> tester: use the sqlite fixture\n" +
		"[direct 1] tester -> developer (failed): half an answer\n" +
		"[direct 1] tester -> developer: fixture works\n" +
		"Propose the decisions made in these exchanges that are not recorded yet, and a brief " +
		"that reflects the direct work that is complete.\n\n" + _wrapPrompt
	if !strings.HasSuffix(ag.prompts[0], want) {
		t.Errorf("prompt = %q, want it to end with %q", ag.prompts[0], want)
	}
}

func TestSession_WrapIgnoresBriefProposal(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch := sessionChannel(t, st, f, false)
	ag := newRecordingAgent()
	ag.st, ag.channelID, ag.propose, ag.kind = st, ch.ID, true, store.ProposalBrief
	s, err := Open(ctx, st, t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), ag.factory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	if p, err := s.Wrap(ctx, discard); !errors.Is(err, ErrNoProposal) {
		t.Errorf("Wrap answered with propose_brief = %+v, %v, want ErrNoProposal", p, err)
	}
}

func TestSession_Promote(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch := sessionChannel(t, st, f, true)
	ag := newRecordingAgent()
	ag.st, ag.channelID, ag.propose = st, ch.ID, true
	dir := t.TempDir()
	s, err := Open(ctx, st, dir, "/bin/reef", ch.ID, NewTrees(t.TempDir()), ag.factory)
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
	note := "This feature works in " + dir + "/worktrees/1/idea on feature/idea."
	if len(msgs) != 2 || msgs[0].Author != "reef" || msgs[0].Body != note || msgs[1].Author != "architect" {
		t.Errorf("messages = %+v, want the note %q from a clean checkout, then the agent's reply", msgs, note)
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
	s, err := Open(ctx, st, dir, "/bin/reef", ch.ID, NewTrees(t.TempDir()), mk)
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
	want := "This feature works in " + worktree +
		" on feature/idea. Uncommitted changes stay on the main checkout."
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
	s, err := Open(ctx, st, t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), ag.factory)
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
		!strings.HasPrefix(turns[0].Error, "features need the project folder to be a git repo with a commit: ") ||
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
	s, err := Open(ctx, st, dir, "/bin/reef", ch.ID, NewTrees(t.TempDir()), mk)
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
			s, err := Open(bg, st, t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), ag.factory)
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

func TestSession_LockErrorStoresItsOwnReason(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch := sessionChannel(t, st, f, false)
	dataDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dataDir, "locks"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	ag := newRecordingAgent()
	s, err := Open(ctx, st, t.TempDir(), "/bin/reef", ch.ID, NewTrees(dataDir), ag.factory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	if err := s.Run(ctx, "hi", nil, discard); err == nil {
		t.Fatal("want a lock error")
	}
	turns, err := st.Turns(ctx, store.TurnFilter{ChannelID: &ch.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 1 || turns[0].Status != "failed" || turns[0].Error == "interrupted" {
		t.Errorf("turns = %+v, want one failed turn with the lock's own reason", turns)
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
	s, err := Open(ctx, st, t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), ag.factory)
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

func TestSession_LeadOnAnotherBackendIgnoresItsRoleProvider(t *testing.T) {
	for _, tc := range []struct {
		backend  string
		provider bool
	}{{"codex", false}, {"claude", true}} {
		t.Run(tc.backend, func(t *testing.T) {
			ctx := context.Background()
			st, f := openStore(t)
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "claude"), []byte("#!/bin/sh\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			if _, err := st.CreateProvider(ctx, store.Provider{Name: "work", Kind: "claude", Folder: dir}); err != nil {
				t.Fatal(err)
			}
			lead := store.Role{Name: "lead", Lead: true, Backend: "claude", Provider: "work"}
			if _, err := st.ApplyTeam(ctx, store.Team{Name: "core", Roles: []store.Role{lead}}); err != nil {
				t.Fatal(err)
			}
			ch, err := st.CreateChannel(ctx, store.Channel{
				ProjectID: f.ProjectID, FeatureID: &f.ID, Name: "chat", Role: "lead",
				Backend: tc.backend, Team: "core",
			})
			if err != nil {
				t.Fatal(err)
			}
			ag := newRecordingAgent()
			mk := func(string) (agent.Agent, error) { return ag, nil }
			s, err := Open(ctx, st, t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), mk)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = s.Close() }()
			if err := s.RunAs(ctx, "lead", "hi", nil, discard); err != nil {
				t.Fatal(err)
			}
			want := ""
			if tc.provider {
				want = filepath.Join(dir, "claude")
			}
			if ag.cfg.Bin != want {
				t.Errorf("Bin on a %s channel = %q, want %q", tc.backend, ag.cfg.Bin, want)
			}
		})
	}
}

func TestSession_RoleRunsItsProviderBinary(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateProvider(ctx, store.Provider{Name: "work", Kind: "claude", Folder: dir}); err != nil {
		t.Fatal(err)
	}
	lead := store.Role{Name: "lead", Lead: true, Backend: "claude"}
	architect := store.Role{Name: "architect", Backend: "claude", Provider: "work"}
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
	mk := func(string) (agent.Agent, error) {
		ag := newRecordingAgent()
		ags = append(ags, ag)
		return ag, nil
	}
	s, err := Open(ctx, st, t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), mk)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if err := s.RunAs(ctx, "lead", "hi", nil, discard); err != nil {
		t.Fatal(err)
	}
	if err := s.RunAs(ctx, "architect", "plan it", nil, discard); err != nil {
		t.Fatal(err)
	}
	if ags[0].cfg.Bin != "" || ags[1].cfg.Bin != filepath.Join(dir, "claude") {
		t.Errorf("Bin of lead, architect = %q, %q, want empty and %s/claude",
			ags[0].cfg.Bin, ags[1].cfg.Bin, dir)
	}

	if err := os.Remove(filepath.Join(dir, "claude")); err != nil {
		t.Fatal(err)
	}
	s2, err := Open(ctx, st, t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), mk)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s2.Close() }()
	err = s2.RunAs(ctx, "architect", "again", nil, discard)
	if !errors.Is(err, store.ErrInvalid) || !strings.Contains(err.Error(), "no claude binary in "+dir) {
		t.Fatalf("RunAs with a removed binary error = %v, want no claude binary in %s", err, dir)
	}
	turns, err := st.Turns(ctx, store.TurnFilter{ChannelID: &ch.ID, Status: "failed"})
	if err != nil || len(turns) != 1 || turns[0].Role != "architect" ||
		!strings.Contains(turns[0].Error, "no claude binary in "+dir) {
		t.Errorf("failed turns = %+v, %v, want one architect turn naming the missing binary", turns, err)
	}
}

func TestSession_RemovedRoleStoresFailedTurn(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	lead := store.Role{Name: "lead", Lead: true, Backend: "claude"}
	architect := store.Role{Name: "architect", Backend: "claude"}
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
	s, err := Open(ctx, st, t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), newRecordingAgent().factory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := st.ApplyTeam(ctx, store.Team{Name: "core", Roles: []store.Role{lead}}); err != nil {
		t.Fatal(err)
	}

	err = s.RunAs(ctx, "architect", "plan it", nil, discard)
	if !errors.Is(err, store.ErrInvalid) {
		t.Fatalf("RunAs for a removed role error = %v, want invalid", err)
	}
	turns, err := st.Turns(ctx, store.TurnFilter{ChannelID: &ch.ID})
	if err != nil || len(turns) != 1 {
		t.Fatalf("turns = %+v, %v, want one", turns, err)
	}
	got := turns[0]
	if got.Role != "architect" || got.Status != "failed" || got.WakeText != "plan it" ||
		got.Error != "invalid: team core has no role architect" {
		t.Errorf("turn = %+v, want a failed architect turn with wake \"plan it\" and the start error", got)
	}
}

type hangingAgent struct{ *recordingAgent }

func (a hangingAgent) Send(_ context.Context, prompt string) error {
	a.prompts = append(a.prompts, prompt)
	a.events <- agent.Event{Kind: agent.KindSession, SessionID: "s1"}
	a.events <- agent.Event{Kind: agent.KindUsage, Usage: &agent.Usage{InputTokens: 10, OutputTokens: 3}}
	a.events <- agent.Event{
		Kind: agent.KindUsage, Usage: &agent.Usage{OutputTokens: 4, CacheWriteTokens: 5},
	}
	return nil
}

func TestSession_QuitMidTurnKeepsPromptAndTokens(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	lead := store.Role{Name: "lead", Lead: true, Backend: "claude"}
	developer := store.Role{Name: "developer", Write: true, Backend: "claude", Persona: "You code."}
	if _, err := st.ApplyTeam(ctx, store.Team{Name: "core", Roles: []store.Role{lead, developer}}); err != nil {
		t.Fatal(err)
	}
	ch, err := st.CreateChannel(ctx, store.Channel{
		ProjectID: f.ProjectID, FeatureID: &f.ID, Name: "chat", Role: "lead", Backend: "claude",
		Team: "core",
	})
	if err != nil {
		t.Fatal(err)
	}
	dataDir, trees := t.TempDir(), NewTrees(t.TempDir())
	hung := hangingAgent{newRecordingAgent()}
	s1, err := Open(ctx, st, dataDir, "/bin/reef", ch.ID, trees,
		func(string) (agent.Agent, error) { return hung, nil })
	if err != nil {
		t.Fatal(err)
	}
	runCtx, quit := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- s1.RunAs(runCtx, "developer", "build it", nil, discard) }()
	want := agent.Usage{InputTokens: 10, OutputTokens: 7, CacheWriteTokens: 5}
	deadline := time.Now().Add(5 * time.Second)
	for {
		turns, err := st.Turns(ctx, store.TurnFilter{ChannelID: &ch.ID, Status: "running"})
		if err != nil {
			t.Fatal(err)
		}
		if len(turns) == 1 && turns[0].Usage == want {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("running turns = %+v, want one with usage %+v", turns, want)
		}
		time.Sleep(5 * time.Millisecond)
	}

	if err := s1.lock.Close(); err != nil {
		t.Fatal(err)
	}
	var ags []*recordingAgent
	s2, err := Open(ctx, st, dataDir, "/bin/reef", ch.ID, trees, func(string) (agent.Agent, error) {
		ag := newRecordingAgent()
		ags = append(ags, ag)
		return ag, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s2.Close() }()
	quit()
	<-done

	turns, err := st.Turns(ctx, store.TurnFilter{ChannelID: &ch.ID})
	if err != nil || len(turns) != 1 {
		t.Fatalf("turns = %+v, %v, want one", turns, err)
	}
	got := turns[0]
	if got.Status != "failed" || got.Error != "chat exited" || got.Usage != want {
		t.Errorf("turn = %q %q %+v, want failed, chat exited, %+v", got.Status, got.Error, got.Usage, want)
	}
	if !strings.Contains(got.Prompt, "You code.") || !strings.HasSuffix(got.Prompt, "build it") {
		t.Errorf("prompt = %q, want the persona and the wake", got.Prompt)
	}

	if used, err := st.ChannelTokens(ctx, ch.ID); err != nil || used != 22 {
		t.Errorf("channel tokens = %d, %v, want 22", used, err)
	}
	over := store.Channel{ID: ch.ID, TokenBudget: 22}
	if now, err := CheckCaps(ctx, st, &over); !now || !errors.Is(err, store.ErrConflict) {
		t.Errorf("CheckCaps at a budget of 22 = %v, %v, want paused now", now, err)
	}

	if err := s2.RunAs(ctx, "developer", "build more", nil, discard); err != nil {
		t.Fatal(err)
	}
	if len(ags) != 1 || ags[0].resumed != "s1" || !slices.Equal(ags[0].prompts, []string{"build more"}) {
		t.Errorf("resumed developer: resumed = %q, prompts = %q, want s1 and [build more]",
			ags[0].resumed, ags[0].prompts)
	}
}

func TestSession_IdleLeadPostsBackgroundAnswer(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch := sessionChannel(t, st, f, false)
	ag := newRecordingAgent()
	s, err := Open(ctx, st, t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), ag.factory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	noted := make(chan struct{}, 1)
	s.SetOnNote(func() { noted <- struct{}{} })
	if err := s.Run(ctx, "go", nil, discard); err != nil {
		t.Fatal(err)
	}

	idle := agent.Usage{InputTokens: 7, OutputTokens: 9}
	ag.events <- agent.Event{Kind: agent.KindUsage, Usage: &idle}
	ag.events <- agent.Event{Kind: agent.KindBackground, Text: "The background command printed bgdone."}
	var got []string
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		msgs, err := st.Messages(ctx, ch.ID, 0)
		if err != nil {
			t.Fatal(err)
		}
		got = got[:0]
		for _, m := range msgs {
			got = append(got, m.Author+": "+m.Body)
		}
		if len(got) == 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	want := []string{"architect: ok", "architect: The background command printed bgdone."}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("messages = %q, want %q", got, want)
	}
	select {
	case <-noted:
	case <-time.After(5 * time.Second):
		t.Fatal("no note hook call after the background answer was posted")
	}

	if err := s.Run(ctx, "again", nil, discard); err != nil {
		t.Fatal(err)
	}
	turns, err := st.Turns(ctx, store.TurnFilter{ChannelID: &ch.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 2 {
		t.Fatalf("turns = %d, want 2", len(turns))
	}
	for _, turn := range turns {
		if turn.Status != "done" {
			t.Errorf("turn %d status = %q, want done", turn.ID, turn.Status)
		}
	}
	if u := turns[1].Usage; u != idle {
		t.Errorf("second turn usage = %+v, want the idle usage %+v", u, idle)
	}
	msgs, err := st.Messages(ctx, ch.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(msgs); n != 3 || msgs[2].Body != "ok" {
		t.Errorf("messages after the second turn = %+v, want its reply \"ok\" as the third", msgs)
	}
}
