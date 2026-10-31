package space

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/memory"
	"github.com/bhanuprakaash/reef/store"
	"github.com/bhanuprakaash/reef/worktree"
)

type recordingAgent struct {
	st        *store.Store
	spaceID   int64
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
			SpaceID: a.spaceID, Role: "architect", Kind: a.kind, Brief: "new brief",
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

func editBot(t *testing.T, st *store.Store, name string, edit func(*store.Bot)) {
	t.Helper()
	all, err := st.Bots(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range all {
		if m.Name == name {
			edit(&m)
			if _, err := st.UpdateBot(context.Background(), m); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	t.Fatalf("no bot %s", name)
}

func sessionSpace(t *testing.T, st *store.Store, f store.WorkItem, quick bool) store.Space {
	t.Helper()
	c := store.Space{
		ProjectID: f.ProjectID, Name: "chat", Role: "architect",
	}
	if !quick {
		c.WorkItemID = &f.ID
	}
	id := testBotID(t, st, "architect")
	bot, err := st.Bot(context.Background(), *id)
	if err != nil {
		t.Fatal(err)
	}
	bot.Write, bot.PermissionMode = true, "acceptEdits"
	if _, err = st.UpdateBot(context.Background(), bot); err != nil {
		t.Fatal(err)
	}
	ch, err := st.CreateBotSpace(context.Background(), c, *id)
	if err != nil {
		t.Fatal(err)
	}
	return ch
}

func discard(agent.Event) {}

func TestOpen_StartsNewSession(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch := sessionSpace(t, st, f, false)
	ag := newRecordingAgent()
	dir := t.TempDir()

	s, err := Open(ctx, st, testMemory(t, st), dir, "/bin/reef", ch.ID, NewTrees(t.TempDir()), ag.factory)
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
	if !strings.HasPrefix(ag.cfg.Instructions, "You are a Bot working in Reef. ") {
		t.Errorf("instructions = %q, want the shared prompt first", ag.cfg.Instructions)
	}
	want := agent.Config{
		Cwd: dir + "/worktrees/1/1", PermissionMode: "acceptEdits", Tools: []string{"Task", "mcp__reef__*"},
		Instructions: ag.cfg.Instructions,
		MCPServers: map[string]agent.MCPServer{"reef": {
			Command: "/bin/reef", Args: []string{"mcp", "--data", dir},
			Env: map[string]string{"REEF_TOKEN": tok},
		}},
	}
	if !reflect.DeepEqual(ag.cfg, want) {
		t.Errorf("config = %+v, want %+v", ag.cfg, want)
	}
	if got, err := st.Token(ctx, tok); err != nil || got.SpaceID != ch.ID || got.Role != "architect" {
		t.Errorf("Token = %+v, %v, want a token of Space %d for architect", got, err, ch.ID)
	}
	if !InUse(dir, ch.ID) {
		t.Error("InUse while open = false, want true")
	}

	if err := sendAs(ctx, st, ch.ID, s.Run, "hi", discard); err != nil {
		t.Fatal(err)
	}
	if len(ag.prompts) != 1 ||
		!strings.HasPrefix(ag.prompts[0], "## You\nYou are architect on work item \"Login\" (1) in project \"shop\".") ||
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
	ch := sessionSpace(t, st, f, true)
	ag := newRecordingAgent()
	dir := t.TempDir()

	s, err := Open(ctx, st, testMemory(t, st), dir, "/bin/reef", ch.ID, NewTrees(t.TempDir()), ag.factory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if err := sendAs(ctx, st, ch.ID, s.Run, "hi", discard); err != nil {
		t.Fatal(err)
	}

	if want := filepath.Join(dir, "chats", "1"); ag.cfg.Cwd != want {
		t.Errorf("Cwd = %q, want %q: an associated project is not the folder", ag.cfg.Cwd, want)
	}
	want := "## You\nYou are architect.\n" +
		"This is a free Space: it has no work item, so there is no brief or work item items.\n" +
		"Associated projects, for context only; you do not work in them: shop (1).\n" +
		"\n## How to work\n" +
		"Details are on demand through the reef MCP tools recall and read_memory. " +
		"remember saves to this Space by default; use scope project or bot for what holds " +
		"across the project or for you.\n" +
		"\n\nhi"
	if len(ag.prompts) != 1 || ag.prompts[0] != want {
		t.Errorf("prompts = %q, want [%q]", ag.prompts, want)
	}
}

func TestWorkDir_NoWorkItem(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	dir := t.TempDir()
	for _, quick := range []bool{true, false} {
		ch := sessionSpace(t, st, f, quick)
		got, key, err := WorkDir(ctx, st, dir, ch.ID)
		if err != nil {
			t.Fatal(err)
		}
		if quick {
			if want := filepath.Join(dir, "chats", "1"); got != want || key != "" {
				t.Errorf("WorkDir of a Space with no work item = %q, %q, want %q and no key", got, key, want)
			}
		} else if want := filepath.Join(dir, "worktrees", "1", "1"); got != want || key != "1" {
			t.Errorf("WorkDir of a work item Space = %q, %q, want %q and key 1", got, key, want)
		}
	}
}

func TestOpen_ChatWithoutProject(t *testing.T) {
	ctx := context.Background()
	st, _ := openStore(t)
	if _, err := st.CreateProject(ctx, store.Project{
		Name: "infra", Path: t.TempDir(), Purpose: "Deploys shop.\nSecond line.",
	}); err != nil {
		t.Fatal(err)
	}
	id := testBotID(t, st, "architect")
	ch, err := st.CreateBotSpace(ctx, store.Space{Name: "chat", Role: "architect"}, *id)
	if err != nil {
		t.Fatal(err)
	}
	ag := newRecordingAgent()
	dir := t.TempDir()

	s, err := Open(ctx, st, testMemory(t, st), dir, "/bin/reef", ch.ID, NewTrees(t.TempDir()), ag.factory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if err := sendAs(ctx, st, ch.ID, s.Run, "hi", discard); err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(dir, "chats", "1")
	if ag.cfg.Cwd != want {
		t.Errorf("Cwd = %q, want %q", ag.cfg.Cwd, want)
	}
	if _, err := os.Stat(filepath.Join(want, ".git")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("stat .git in %s = %v, want not exist", want, err)
	}
	if worktree.IsRepo(ctx, want) {
		t.Errorf("%s is a git repo, want none", want)
	}
	wantPrompt := "## You\nYou are architect.\n" +
		"This is a free Space: it has no work item, so there is no brief or work item items.\n" +
		"Other projects (list_projects finds them, associate_project links one to this Space): shop (1); infra (2): Deploys shop.\n" +
		"\n## How to work\n" +
		"Details are on demand through the reef MCP tools recall and read_memory. " +
		"remember saves to this Space by default; use scope bot for what holds for you.\n" +
		"\n\nhi"
	if len(ag.prompts) != 1 || ag.prompts[0] != wantPrompt {
		t.Errorf("prompts = %q, want [%q]", ag.prompts, wantPrompt)
	}
}

func TestOpen_QuickSession_ProjectMemory(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ms := testMemory(t, st)
	day := remember(t, ms, projectScope(f), memory.KindFact, "user", "Use Go, not Python.").
		CreatedAt.Format(_dateLayout)
	ch := sessionSpace(t, st, f, true)
	ag := newRecordingAgent()

	s, err := Open(ctx, st, ms, t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), ag.factory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if err := sendAs(ctx, st, ch.ID, s.Run, "hi", discard); err != nil {
		t.Fatal(err)
	}

	want := "\n## What we know\n### Project\nfact 1: Use Go, not Python. (user, " + day + ")\n" +
		"1 of 1; recall(..., scope=project)\n"
	if len(ag.prompts) != 1 || !strings.Contains(ag.prompts[0], want) {
		t.Errorf("prompts = %q, want it to contain %q", ag.prompts, want)
	}
}

func TestOpen_QuickSession_SecondTurnListsProjectItemAddedSinceTheFirst(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ms := testMemory(t, st)
	ch := sessionSpace(t, st, f, true)
	ag := newRecordingAgent()

	s, err := Open(ctx, st, ms, t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), ag.factory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if err := sendAs(ctx, st, ch.ID, s.Run, "one", discard); err != nil {
		t.Fatal(err)
	}
	day := remember(t, ms, projectScope(f), memory.KindFact, "user", "Use Go, not Python.").
		CreatedAt.Format(_dateLayout)
	if err := sendAs(ctx, st, ch.ID, s.Run, "two", discard); err != nil {
		t.Fatal(err)
	}

	want := "### Changed since your last turn\nfact 1: Use Go, not Python. (user, " + day + ")\n\ntwo"
	if len(ag.prompts) != 2 || ag.prompts[1] != want {
		t.Errorf("prompts = %q, want the second to be %q", ag.prompts, want)
	}
}

func TestOpen_ResumesNewestSession(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch := sessionSpace(t, st, f, false)
	for _, session := range []string{"sess-old", "sess-new", ""} {
		turn, err := st.CreateTurn(ctx, store.Turn{
			SpaceID: ch.ID, Role: "architect", BotID: testBotID(t, st, "architect"), BotSessionID: testBotSessionID(t, st, ch.ID, testBotID(t, st, "architect")), Backend: "claude", SessionID: session,
			WakeText: "go",
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

	s, err := Open(ctx, st, testMemory(t, st), t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), ag.factory)
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
	web, err := st.CreateConnection(ctx, store.Connection{Name: "web", URL: "http://x"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testApplyTeam(t, st, ctx, store.Team{Name: "core", Roles: []store.Role{
		{Name: "lead", Lead: true, Backend: "claude"},
		{
			Name: "architect", Backend: "claude", Description: "You design.", Skills: []string{"claude:sketch"},
			Tools: []string{"Read", "Grep"}, Connections: []store.Connection{web},
		},
	}}); err != nil {
		t.Fatal(err)
	}
	ch, err := st.CreateSpace(ctx, store.Space{
		ProjectID: f.ProjectID, WorkItemID: &f.ID, Name: "chat", Role: "lead",
		Team: "core",
	})
	if err != nil {
		t.Fatal(err)
	}
	root, err := st.CreateMessage(ctx, store.Message{SpaceID: ch.ID, Author: "user", Body: "root"})
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
	s, err := Open(ctx, st, testMemory(t, st), t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), mk)
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
	if err := s.RunAs(ctx, "architect", "plan it", &root.ID, nil, whoAmI); err != nil {
		t.Fatal(err)
	}
	want := "## You\nYou are architect on work item"
	if len(ags) != 1 || !strings.HasPrefix(ags[0].prompts[0], want) {
		t.Errorf("prompts = %q, want one starting with %q", ags[0].prompts, want)
	}
	if !reflect.DeepEqual(ags[0].cfg.Tools, []string{"Read", "Grep", "Skill(sketch)", "mcp__reef__*"}) ||
		ags[0].cfg.MCPServers["web"].URL != "http://x" {
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

	if err := s.RunAs(ctx, "architect", "again", nil, nil, discard); err != nil {
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
	if err := s.RunAs(ctx, "tester", "hi", nil, nil, discard); !errors.Is(err, store.ErrInvalid) {
		t.Errorf("RunAs unknown role error = %v, want ErrInvalid", err)
	}

	closeErr = errors.New("close failed")
	if err := s.RunAs(ctx, "architect", "last", nil, nil, discard); err != nil {
		t.Errorf("RunAs error = %v, want nil: a failed close does not fail a good turn", err)
	}
}

func TestSession_RunAsMemberAddedAfterOpen(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	lead := store.Role{Name: "lead", Lead: true, Backend: "claude"}
	if _, err := testApplyTeam(t, st, ctx, store.Team{Name: "core", Roles: []store.Role{lead}}); err != nil {
		t.Fatal(err)
	}
	ch, err := st.CreateSpace(ctx, store.Space{
		ProjectID: f.ProjectID, WorkItemID: &f.ID, Name: "chat", Role: "lead",
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
	s, err := Open(ctx, st, testMemory(t, st), t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), mk)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}

	tester, err := st.CreateBot(ctx, store.Bot{Name: "tester", Backend: "claude", Description: "You test."})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddSpaceMember(ctx, ch.ID, tester.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.RunAs(ctx, "tester", "check it", nil, nil, discard); err != nil {
		t.Fatalf("RunAs tester error = %v, want nil", err)
	}
	if len(ags) != 2 || !strings.HasSuffix(ags[1].cfg.Instructions, "\n\nYou test.") {
		t.Errorf("agents = %d, tester instructions = %q, want the description after the shared prompt",
			len(ags), ags[len(ags)-1].cfg.Instructions)
	}
}

func TestSession_RoleSetupSpaceWithoutMembers(t *testing.T) {
	s := &Session{ch: store.Space{ID: 7, Role: "lead"}}
	_, err := s.roleSetup(store.Team{Name: "full"}, "lead")
	if !errors.Is(err, store.ErrConflict) || err.Error() != "conflict: Space 7 has no members" {
		t.Errorf("roleSetup of a Space with no members error = %v, want conflict: Space 7 has no members", err)
	}
}

func TestSession_AdoptedSettingsSurviveBotEdits(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	lead := store.Role{Name: "lead", Lead: true, Backend: "claude"}
	developer := store.Role{
		Name: "developer", Backend: "claude", Write: true, Description: "You code.", Model: "haiku",
		Tools: []string{"Grep"}, PermissionMode: "acceptEdits",
	}
	if _, err := testApplyTeam(t, st, ctx, store.Team{Name: "core", Roles: []store.Role{lead, developer}}); err != nil {
		t.Fatal(err)
	}
	newSpace := func() int64 {
		ch, err := st.CreateSpace(ctx, store.Space{
			ProjectID: f.ProjectID, Name: "chat", Role: "lead", Team: "core",
		})
		if err != nil {
			t.Fatal(err)
		}
		return ch.ID
	}
	var ags []*recordingAgent
	mk := func(string) (agent.Agent, error) {
		ag := newRecordingAgent()
		ags = append(ags, ag)
		return ag, nil
	}
	dir, trees := t.TempDir(), NewTrees(t.TempDir())
	turn := func(spaceID int64, text string) *recordingAgent {
		s, err := Open(ctx, st, testMemory(t, st), dir, "/bin/reef", spaceID, trees, mk)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = s.Close() }()
		_ = s.RunAs(ctx, "developer", text, nil, nil, discard)
		return ags[len(ags)-1]
	}
	adopted := func(ag *recordingAgent, resumed, model string) {
		t.Helper()
		if ag.resumed != resumed || ag.cfg.Model != model || ag.cfg.PermissionMode != "acceptEdits" ||
			!reflect.DeepEqual(ag.cfg.Tools, []string{"Grep", "Task", "mcp__reef__*"}) {
			t.Errorf("resumed %q, cfg %+v; want resumed %q, model %s, acceptEdits and Grep", ag.resumed, ag.cfg, resumed, model)
		}
		if strings.Contains(strings.Join(ag.prompts, "\n"), "Go only") {
			t.Errorf("prompts = %q, want the adopted description", ag.prompts)
		}
	}

	first := newSpace()
	turn(first, "build it")
	editBot(t, st, "developer", func(m *store.Bot) {
		m.Description, m.Model, m.Tools, m.PermissionMode = "You code in Go only.", "opus", []string{"Read"}, "default"
	})
	adopted(turn(first, "build more"), "s1", "haiku")

	ags = nil
	ag := newRecordingAgent()
	ag.resumeErr = errors.New("thread/resume: no rollout found for thread id s1")
	mkGone := mk
	mk = func(string) (agent.Agent, error) { ags = append(ags, ag); return ag, nil }
	turn(first, "gone")
	mk = mkGone
	adopted(turn(first, "fresh context"), "", "haiku")

	second := newSpace()
	got := turn(second, "new chat")
	if got.cfg.Model != "opus" || got.cfg.PermissionMode != "default" ||
		!reflect.DeepEqual(got.cfg.Tools, []string{"Read", "Task", "mcp__reef__*"}) ||
		!strings.HasSuffix(got.cfg.Instructions, "\n\nYou code in Go only.") {
		t.Errorf("new Space: cfg %+v; want the edited opus, Read and description", got.cfg)
	}
}

func TestSession_MissingSessionStartsNewOne(t *testing.T) {
	const intro = "You are lead on work item"
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
			lead := store.Role{Name: "lead", Lead: true, Backend: "claude", Description: "You lead."}
			if _, err := testApplyTeam(t, st, ctx, store.Team{Name: "core", Roles: []store.Role{lead}}); err != nil {
				t.Fatal(err)
			}
			ch, err := st.CreateSpace(ctx, store.Space{
				ProjectID: f.ProjectID, WorkItemID: &f.ID, Name: "chat", Role: "lead",
				Team: "core",
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
			s, err := Open(ctx, st, testMemory(t, st), dir, "/bin/reef", ch.ID, trees, mk)
			if err != nil {
				t.Fatal(err)
			}
			if err := s.Run(ctx, "hi", nil, discard); err != nil {
				t.Fatal(err)
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}

			s, err = Open(ctx, st, testMemory(t, st), dir, "/bin/reef", ch.ID, trees, mk)
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
			s, err = Open(ctx, st, testMemory(t, st), dir, "/bin/reef", ch.ID, trees, mk)
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
			ch := sessionSpace(t, st, f, false)
			ag := newRecordingAgent()
			ag.handshake = &agent.Event{Kind: agent.KindSession, SessionID: "c1", Model: "gpt-5"}
			s, err := Open(ctx, st, testMemory(t, st), t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), ag.factory)
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
			turns, err := st.Turns(ctx, store.TurnFilter{SpaceID: &ch.ID})
			if err != nil {
				t.Fatal(err)
			}
			if len(turns) != 1 || turns[0].SessionID != "c1" || turns[0].Model != "gpt-5" {
				t.Errorf("turns = %+v, want one with session c1 and model gpt-5", turns)
			}
		})
	}
}

func TestSession_TeamWriteEditLeavesRunningSpaceAlone(t *testing.T) {
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
			if _, err := testApplyTeam(t, st, ctx, store.Team{Name: "core", Roles: []store.Role{lead}}); err != nil {
				t.Fatal(err)
			}
			ch, err := st.CreateSpace(ctx, store.Space{
				ProjectID: f.ProjectID, WorkItemID: &f.ID, Name: "chat", Role: "lead",
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
			s, err := Open(ctx, st, testMemory(t, st), t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), mk)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = s.Close() }()
			if err := s.Run(ctx, "hi", nil, discard); err != nil {
				t.Fatal(err)
			}

			lead.Write = tc.after
			if _, err := testApplyTeam(t, st, ctx, store.Team{Name: "core", Roles: []store.Role{lead}}); err != nil {
				t.Fatal(err)
			}
			if err := s.Run(ctx, "again", nil, discard); err != nil {
				t.Fatal(err)
			}
			if len(ags) != 1 || ags[0].closed {
				t.Fatalf("agents = %d, first closed = %v; want the one agent kept", len(ags), ags[0].closed)
			}
			cfg := ags[0].cfg
			if got := !slices.Contains(cfg.DisallowedTools, "Edit"); got != tc.before {
				t.Errorf("next turn can edit = %v, want %v (disallowed %v)", got, tc.before, cfg.DisallowedTools)
			}
			if got := s.members[s.leadID].run.CheckpointDir != ""; got != tc.before {
				t.Errorf("next turn has a checkpoint = %v, want %v", got, tc.before)
			}
		})
	}
}

func TestSession_PersonaUnchangedNoRoleIntroOnResume(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	lead := store.Role{Name: "lead", Lead: true, Backend: "claude"}
	developer := store.Role{Name: "developer", Write: true, Backend: "claude", Description: "You code."}
	if _, err := testApplyTeam(t, st, ctx, store.Team{Name: "core", Roles: []store.Role{lead, developer}}); err != nil {
		t.Fatal(err)
	}
	ch, err := st.CreateSpace(ctx, store.Space{
		ProjectID: f.ProjectID, WorkItemID: &f.ID, Name: "chat", Role: "lead",
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
	s, err := Open(ctx, st, testMemory(t, st), t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), mk)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if err := s.RunAs(ctx, "developer", "build it", nil, nil, discard); err != nil {
		t.Fatal(err)
	}
	if err := s.RunAs(ctx, "developer", "build more", nil, nil, discard); err != nil {
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
	developer := store.Role{Name: "developer", Write: true, Backend: "claude", Description: "You code."}
	if _, err := testApplyTeam(t, st, ctx, store.Team{Name: "core", Roles: []store.Role{lead, developer}}); err != nil {
		t.Fatal(err)
	}
	ch, err := st.CreateSpace(ctx, store.Space{
		ProjectID: f.ProjectID, WorkItemID: &f.ID, Name: "chat", Role: "lead",
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
	s, err := Open(ctx, st, testMemory(t, st), t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), mk)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	for i, wake := range []string{"wake 1", "wake 2", "wake 3", "wake 4"} {
		if err := s.RunAs(ctx, "developer", wake, nil, nil, discard); err != nil {
			t.Fatalf("wake %d: %v", i+1, err)
		}
	}
	if len(ags) != 4 {
		t.Fatalf("agents = %d, want 4", len(ags))
	}
	if len(ags[0].prompts) != 1 || !strings.HasSuffix(ags[0].cfg.Instructions, "\n\nYou code.") ||
		!strings.HasSuffix(ags[0].prompts[0], "wake 1") {
		t.Errorf("wake 1: prompts = %q, instructions = %q, want the description and wake 1",
			ags[0].prompts, ags[0].cfg.Instructions)
	}
	want := []string{"wake 2", "wake 3", "wake 4"}
	for i, ag := range ags[1:] {
		if len(ag.prompts) != 1 || ag.prompts[0] != want[i] {
			t.Errorf("wake %d: prompts = %q, want [%q] without a role intro", i+2, ag.prompts, want[i])
		}
	}
}

func TestSession_NoEditFourLiveWakesOnlyFirstHasIntro(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	lead := store.Role{Name: "lead", Lead: true, Backend: "claude", Description: "You lead."}
	if _, err := testApplyTeam(t, st, ctx, store.Team{Name: "core", Roles: []store.Role{lead}}); err != nil {
		t.Fatal(err)
	}
	ch, err := st.CreateSpace(ctx, store.Space{
		ProjectID: f.ProjectID, WorkItemID: &f.ID, Name: "chat", Role: "lead",
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
	s, err := Open(ctx, st, testMemory(t, st), t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), mk)
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
	if !strings.HasSuffix(ags[0].cfg.Instructions, "\n\nYou lead.") || !strings.HasSuffix(ags[0].prompts[0], "wake 1") {
		t.Errorf("wake 1: prompt = %q, instructions = %q, want the description and wake 1",
			ags[0].prompts[0], ags[0].cfg.Instructions)
	}
	want := []string{"wake 2", "wake 3", "wake 4"}
	for i, p := range ags[0].prompts[1:] {
		if p != want[i] {
			t.Errorf("wake %d: prompt = %q, want %q without a role intro", i+2, p, want[i])
		}
	}
}

func TestOpen_Errors(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch := sessionSpace(t, st, f, false)
	bad, err := st.CreateSpace(ctx, store.Space{
		ProjectID: f.ProjectID, Name: "typo", Role: "architect",
	})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		give int64
		want string
	}{
		{"unknown space", 99, "space 99: not found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := Open(ctx, st, testMemory(t, st), t.TempDir(), "/bin/reef", tt.give, NewTrees(t.TempDir()), NewAgent)
			if err == nil {
				err = s.Start(ctx)
				_ = s.Close()
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Open error = %v, want %q", err, tt.want)
			}
		})
	}

	t.Run("profile backend overrides Space backend", func(t *testing.T) {
		ag := newRecordingAgent()
		s, err := Open(ctx, st, testMemory(t, st), t.TempDir(), "/bin/reef", bad.ID, NewTrees(t.TempDir()), ag.factory)
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		if err = s.Start(ctx); err != nil || !ag.started {
			t.Fatalf("start = %v, started = %v", err, ag.started)
		}
	})

	t.Run("open elsewhere", func(t *testing.T) {
		dir := t.TempDir()
		s, err := Open(ctx, st, testMemory(t, st), dir, "/bin/reef", ch.ID, NewTrees(t.TempDir()), newRecordingAgent().factory)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = s.Close() }()
		_, err = Open(ctx, st, testMemory(t, st), dir, "/bin/reef", ch.ID, NewTrees(t.TempDir()), newRecordingAgent().factory)
		if err == nil || err.Error() != "space 1 is open elsewhere: conflict" {
			t.Errorf("second open error = %v, want the open-elsewhere error", err)
		}
	})

	t.Run("agent does not start", func(t *testing.T) {
		dir := t.TempDir()
		ag := newRecordingAgent()
		ag.startErr = errors.New("no claude")
		s, err := Open(ctx, st, testMemory(t, st), dir, "/bin/reef", ch.ID, NewTrees(t.TempDir()), ag.factory)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Run(ctx, "hi", nil, discard); err == nil || err.Error() != "no claude" {
			t.Errorf("Run error = %v, want no claude", err)
		}
		turns, err := st.Turns(ctx, store.TurnFilter{SpaceID: &ch.ID})
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
	ch := sessionSpace(t, st, f, false)
	ag := newRecordingAgent()
	ag.st, ag.spaceID, ag.propose = st, ch.ID, true
	s, err := Open(ctx, st, testMemory(t, st), t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), ag.factory)
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

	if turns, err := st.Turns(ctx, store.TurnFilter{SpaceID: &ch.ID}); err != nil || len(turns) != 1 || turns[0].WakeText != "" {
		t.Errorf("turns = %+v, %v, want one turn that kept no wake", turns, err)
	}

	ag.propose = false
	if _, err := s.Wrap(ctx, discard); !errors.Is(err, ErrNoProposal) {
		t.Errorf("Wrap with an older proposal pending error = %v, want ErrNoProposal", err)
	}
}

func TestMaintain_SharesNothingWithTheSession(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch := sessionSpace(t, st, f, false)
	dir := t.TempDir()
	var mu sync.Mutex
	var agents []*recordingAgent
	mk := func(string) (agent.Agent, error) {
		ag := newRecordingAgent()
		mu.Lock()
		agents = append(agents, ag)
		mu.Unlock()
		return ag, nil
	}
	s, err := Open(ctx, st, testMemory(t, st), dir, "/bin/reef", ch.ID, NewTrees(t.TempDir()), mk)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	lead := s.leadID

	done := make(chan error, 1)
	go func() {
		done <- Maintain(ctx, st, dir, "/bin/reef", ch.ID, lead, mk, discard)
	}()
	for range 10 {
		if err := s.RunBot(ctx, lead, "hello", nil, nil, discard); err != nil {
			t.Fatal(err)
		}
		if err := s.Refresh(ctx, lead); err != nil {
			t.Fatal(err)
		}
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	var review *recordingAgent
	for _, ag := range agents {
		if len(ag.prompts) == 1 && ag.prompts[0] == _maintainPrompt {
			review = ag
		}
	}
	if review == nil {
		t.Fatal("no agent got the review prompt alone")
	}
	cfg := review.cfg
	if review.resumed != "" || cfg.Fork || cfg.PermissionMode != "default" || !review.closed ||
		!slices.Equal(cfg.DisallowedTools, []string{"Edit", "Write", "MultiEdit", "NotebookEdit", "Bash"}) ||
		len(cfg.MCPServers) != 1 || cfg.Cwd != filepath.Join(dir, "reviews", strconv.FormatInt(lead, 10)) {
		t.Errorf("config = %+v, resumed = %q, closed = %v, want a fresh read-only context in the review folder with only reef",
			cfg, review.resumed, review.closed)
	}
	if _, err := st.Token(ctx, cfg.MCPServers["reef"].Env[TokenEnv]); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("review token after the review: error = %v, want ErrNotFound", err)
	}
	msgs, err := st.Messages(ctx, ch.ID, 0)
	if replies := slices.DeleteFunc(msgs, func(m store.Message) bool { return m.Author != "architect" }); err != nil || len(replies) != 10 {
		t.Errorf("replies = %d, %v, want the 10 of the session turns and none of the review", len(replies), err)
	}
	turns, err := st.Turns(ctx, store.TurnFilter{SpaceID: &ch.ID})
	if err != nil {
		t.Fatal(err)
	}
	var reviews int
	for _, turn := range turns {
		if turn.WakeText == "" && turn.Reply == "ok" && turn.Status == "done" {
			reviews++
		}
	}
	if reviews != 1 {
		t.Errorf("done turns with no wake that kept their reply = %d, want 1", reviews)
	}
}

func TestSession_WrapIncludesDirectExchanges(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch := sessionSpace(t, st, f, false)
	req, err := st.CreateDirectMessage(ctx, store.DirectMessage{
		SpaceID: ch.ID, From: "developer", To: "tester", FromBotID: testBotID(t, st, "developer"), ToBotID: testBotID(t, st, "tester"), Body: "use the sqlite fixture"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateFailedDirectResponse(ctx, req.ID, testBotID(t, st, "tester"), "tester", "half an answer"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateDirectResponse(ctx, req.ID, testBotID(t, st, "tester"), "tester", "fixture works"); err != nil {
		t.Fatal(err)
	}
	ag := newRecordingAgent()
	ag.st, ag.spaceID, ag.propose = st, ch.ID, true
	s, err := Open(ctx, st, testMemory(t, st), t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), ag.factory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	if _, err := s.Wrap(ctx, discard); err != nil {
		t.Fatal(err)
	}
	want := "Direct exchanges in this Space, private between two bots:\n" +
		"[direct 1] developer -> tester: use the sqlite fixture\n" +
		"[direct 1] tester -> developer (failed): half an answer\n" +
		"[direct 1] tester -> developer: fixture works\n" +
		"Propose as items what these exchanges decided or found that is not saved yet, and a " +
		"brief that reflects the direct work that is complete.\n\n" + _wrapPrompt
	if !strings.HasSuffix(ag.prompts[0], want) {
		t.Errorf("prompt = %q, want it to end with %q", ag.prompts[0], want)
	}
}

func TestSession_WrapSaysHowManyExchangesItLeftOut(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch := sessionSpace(t, st, f, false)
	for i := 1; i <= 120; i++ {
		if _, err := st.CreateDirectMessage(ctx, store.DirectMessage{
			SpaceID: ch.ID, From: "developer", To: "tester", FromBotID: testBotID(t, st, "developer"), ToBotID: testBotID(t, st, "tester"), Body: fmt.Sprintf("message %d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	ag := newRecordingAgent()
	ag.st, ag.spaceID, ag.propose = st, ch.ID, true
	s, err := Open(ctx, st, testMemory(t, st), t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), ag.factory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	if _, err := s.Wrap(ctx, discard); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ag.prompts[0], "bots:\n20 older exchanges left out (read_direct_messages)\n"+
		"[direct 21] developer -> tester: message 21\n") || strings.Contains(ag.prompts[0], "message 20\n") {
		t.Errorf("prompt = %q, want 20 older exchanges left out and message 21 first", ag.prompts[0])
	}
}

func TestSession_WrapRunsInACopyOfTheLeadsSession(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch := sessionSpace(t, st, f, false)
	turn, err := st.CreateTurn(ctx, store.Turn{
		SpaceID: ch.ID, Role: "architect", BotID: testBotID(t, st, "architect"), BotSessionID: testBotSessionID(t, st, ch.ID, testBotID(t, st, "architect")), Backend: "claude", SessionID: "sess-lead", WakeText: "go",
	})
	if err != nil {
		t.Fatal(err)
	}
	turn.Status = "done"
	if _, err := st.EndTurn(ctx, turn); err != nil {
		t.Fatal(err)
	}
	ag := newRecordingAgent()
	ag.st, ag.spaceID, ag.propose, ag.session = st, ch.ID, true, "sess-copy"
	s, err := Open(ctx, st, testMemory(t, st), t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), ag.factory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	if _, err := s.Wrap(ctx, discard); err != nil {
		t.Fatal(err)
	}
	if ag.resumed != "sess-lead" || !ag.cfg.Fork || !ag.closed || len(s.members) != 0 {
		t.Errorf("resumed = %q, fork = %v, closed = %v, members = %d, want a closed copy of sess-lead",
			ag.resumed, ag.cfg.Fork, ag.closed, len(s.members))
	}
	last, err := newestSessionTurn(ctx, st, ch.ID, s.leadID, *testBotSessionID(t, st, ch.ID, &s.leadID))
	if err != nil || last.SessionID != "sess-lead" {
		t.Errorf("session to resume = %q, %v, want sess-lead, not the wrap-up's sess-copy", last.SessionID, err)
	}
}

func TestSession_CodexWrapRunsInAFreshThread(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch, err := st.CreateBotSpace(ctx, store.Space{
		ProjectID: f.ProjectID, WorkItemID: &f.ID, Name: "chat", Role: "architect",
	}, *testBotID(t, st, "architect"))
	if err != nil {
		t.Fatal(err)
	}
	editBot(t, st, "architect", func(m *store.Bot) { m.Backend = "codex" })
	turn, err := st.CreateTurn(ctx, store.Turn{
		SpaceID: ch.ID, Role: "architect", BotID: testBotID(t, st, "architect"), BotSessionID: testBotSessionID(t, st, ch.ID, testBotID(t, st, "architect")), Backend: "codex", SessionID: "thread-lead", WakeText: "go",
	})
	if err != nil {
		t.Fatal(err)
	}
	turn.Status = "done"
	if _, err := st.EndTurn(ctx, turn); err != nil {
		t.Fatal(err)
	}
	ag := newRecordingAgent()
	ag.st, ag.spaceID, ag.propose, ag.session = st, ch.ID, true, "thread-wrap"
	s, err := Open(ctx, st, testMemory(t, st), t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), ag.factory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	if _, err := s.Wrap(ctx, discard); err != nil {
		t.Fatal(err)
	}
	if !ag.started || ag.resumed != "" || !ag.closed || len(s.members) != 0 {
		t.Errorf("started = %v, resumed = %q, closed = %v, members = %d, want a closed fresh thread",
			ag.started, ag.resumed, ag.closed, len(s.members))
	}
	last, err := newestSessionTurn(ctx, st, ch.ID, s.leadID, *testBotSessionID(t, st, ch.ID, &s.leadID))
	if err != nil || last.SessionID != "thread-lead" {
		t.Errorf("thread to resume = %q, %v, want thread-lead, not the wrap-up's thread-wrap", last.SessionID, err)
	}
}

func TestNewestSessionTurn_MissingSessionOnAWrapTurnResets(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch := sessionSpace(t, st, f, false)
	for _, turn := range []store.Turn{
		{SpaceID: ch.ID, Role: "architect", BotID: testBotID(t, st, "architect"), BotSessionID: testBotSessionID(t, st, ch.ID, testBotID(t, st, "architect")), Backend: "claude", SessionID: "sess-lead", WakeText: "go"},
		{SpaceID: ch.ID, Role: "architect", BotID: testBotID(t, st, "architect"), BotSessionID: testBotSessionID(t, st, ch.ID, testBotID(t, st, "architect")), Backend: "claude", Status: "failed", Error: "No conversation found with session ID: sess-lead"},
	} {
		created, err := st.CreateTurn(ctx, turn)
		if err != nil {
			t.Fatal(err)
		}
		if turn.Status == "" {
			turn.Status = "done"
		}
		created.Status, created.Error = turn.Status, turn.Error
		if _, err := st.EndTurn(ctx, created); err != nil {
			t.Fatal(err)
		}
	}

	last, err := newestSessionTurn(ctx, st, ch.ID, *testBotID(t, st, "architect"), *testBotSessionID(t, st, ch.ID, testBotID(t, st, "architect")))
	if err != nil || last.SessionID != "" {
		t.Errorf("session to resume = %q, %v, want none", last.SessionID, err)
	}
}

func TestSession_WrapIgnoresBriefProposal(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch := sessionSpace(t, st, f, false)
	ag := newRecordingAgent()
	ag.st, ag.spaceID, ag.propose, ag.kind = st, ch.ID, true, store.ProposalBrief
	s, err := Open(ctx, st, testMemory(t, st), t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), ag.factory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	if p, err := s.Wrap(ctx, discard); !errors.Is(err, ErrNoProposal) {
		t.Errorf("Wrap answered with propose_brief = %+v, %v, want ErrNoProposal", p, err)
	}
}

func TestSession_NoWorktreeFailsTurn(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	p, err := st.CreateProject(ctx, store.Project{Name: "notes", Path: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	f, err = st.CreateWorkItem(ctx, store.WorkItem{ProjectID: p.ID, Title: "Docs"})
	if err != nil {
		t.Fatal(err)
	}
	ch := sessionSpace(t, st, f, false)
	ag := newRecordingAgent()
	s, err := Open(ctx, st, testMemory(t, st), t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), ag.factory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	err = s.Run(ctx, "hi", nil, discard)
	if err == nil || !strings.Contains(err.Error(), "not a git repository") {
		t.Fatalf("Run error = %v, want one saying not a git repository", err)
	}
	turns, err := st.Turns(ctx, store.TurnFilter{SpaceID: &ch.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 1 || turns[0].Status != "failed" ||
		!strings.HasPrefix(turns[0].Error, "work items need the project folder to be a git repo with a commit: ") ||
		!strings.Contains(turns[0].Error, "not a git repository") || ag.started {
		t.Errorf("turns = %+v, started = %v, want one failed turn and no agent started", turns, ag.started)
	}
}

func TestSession_RecreatesRemovedWorktree(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch := sessionSpace(t, st, f, false)
	var ags []*recordingAgent
	mk := func(string) (agent.Agent, error) {
		ag := newRecordingAgent()
		ags = append(ags, ag)
		return ag, nil
	}
	dir := t.TempDir()
	s, err := Open(ctx, st, testMemory(t, st), dir, "/bin/reef", ch.ID, NewTrees(t.TempDir()), mk)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if err := s.Run(ctx, "hi", nil, discard); err != nil {
		t.Fatal(err)
	}
	tree := dir + "/worktrees/1/1"
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
	turns, err := st.Turns(ctx, store.TurnFilter{SpaceID: &ch.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 2 || turns[0].Status != "done" || turns[1].Status != "done" {
		t.Errorf("turns = %+v, want two done", turns)
	}
}

func TestSession_WrapCancelledStoresTurn(t *testing.T) {
	tests := []struct {
		name string
		give func(s *Session, ctx context.Context) error
	}{
		{"wrap", func(s *Session, ctx context.Context) error {
			_, err := s.Wrap(ctx, discard)
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bg := context.Background()
			st, f := openStore(t)
			ch := sessionSpace(t, st, f, false)
			ag := newRecordingAgent()
			s, err := Open(bg, st, testMemory(t, st), t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), ag.factory)
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
			turns, err := st.Turns(bg, store.TurnFilter{SpaceID: &ch.ID})
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
	ch := sessionSpace(t, st, f, false)
	dataDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dataDir, "locks"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	ag := newRecordingAgent()
	s, err := Open(ctx, st, testMemory(t, st), t.TempDir(), "/bin/reef", ch.ID, NewTrees(dataDir), ag.factory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	if err := s.Run(ctx, "hi", nil, discard); err == nil {
		t.Fatal("want a lock error")
	}
	turns, err := st.Turns(ctx, store.TurnFilter{SpaceID: &ch.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 1 || turns[0].Status != "failed" || turns[0].Error == "interrupted" {
		t.Errorf("turns = %+v, want one failed turn with the lock's own reason", turns)
	}
}

func TestSession_CoordinatorUsesProfileProvider(t *testing.T) {
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
			if _, err := testApplyTeam(t, st, ctx, store.Team{Name: "core", Roles: []store.Role{lead}}); err != nil {
				t.Fatal(err)
			}
			ch, err := st.CreateSpace(ctx, store.Space{
				ProjectID: f.ProjectID, WorkItemID: &f.ID, Name: "chat", Role: "lead",
				Team: "core",
			})
			if err != nil {
				t.Fatal(err)
			}
			ag := newRecordingAgent()
			mk := func(string) (agent.Agent, error) { return ag, nil }
			s, err := Open(ctx, st, testMemory(t, st), t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), mk)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = s.Close() }()
			if err := s.RunAs(ctx, "lead", "hi", nil, nil, discard); err != nil {
				t.Fatal(err)
			}
			want := ""
			want = filepath.Join(dir, "claude")
			if ag.cfg.Bin != want {
				t.Errorf("Bin on a %s Space = %q, want %q", tc.backend, ag.cfg.Bin, want)
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
	if _, err := testApplyTeam(t, st, ctx, store.Team{Name: "core", Roles: []store.Role{lead, architect}}); err != nil {
		t.Fatal(err)
	}
	ch, err := st.CreateSpace(ctx, store.Space{
		ProjectID: f.ProjectID, WorkItemID: &f.ID, Name: "chat", Role: "lead",
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
	s, err := Open(ctx, st, testMemory(t, st), t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), mk)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if err := s.RunAs(ctx, "lead", "hi", nil, nil, discard); err != nil {
		t.Fatal(err)
	}
	if err := s.RunAs(ctx, "architect", "plan it", nil, nil, discard); err != nil {
		t.Fatal(err)
	}
	if ags[0].cfg.Bin != "" || ags[1].cfg.Bin != filepath.Join(dir, "claude") {
		t.Errorf("Bin of lead, architect = %q, %q, want empty and %s/claude",
			ags[0].cfg.Bin, ags[1].cfg.Bin, dir)
	}

	if err := os.Remove(filepath.Join(dir, "claude")); err != nil {
		t.Fatal(err)
	}
	s2, err := Open(ctx, st, testMemory(t, st), t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), mk)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s2.Close() }()
	err = s2.RunAs(ctx, "architect", "again", nil, nil, discard)
	if !errors.Is(err, store.ErrInvalid) || !strings.Contains(err.Error(), "no claude binary in "+dir) {
		t.Fatalf("RunAs with a removed binary error = %v, want no claude binary in %s", err, dir)
	}
	turns, err := st.Turns(ctx, store.TurnFilter{SpaceID: &ch.ID, Status: "failed"})
	if err != nil || len(turns) != 1 || turns[0].Role != "architect" ||
		!strings.Contains(turns[0].Error, "no claude binary in "+dir) {
		t.Errorf("failed turns = %+v, %v, want one architect turn naming the missing binary", turns, err)
	}
}

func TestSession_RoleTheSpaceLacksStoresFailedTurn(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	lead := store.Role{Name: "lead", Lead: true, Backend: "claude"}
	architect := store.Role{Name: "architect", Backend: "claude"}
	if _, err := testApplyTeam(t, st, ctx, store.Team{Name: "core", Roles: []store.Role{lead, architect}}); err != nil {
		t.Fatal(err)
	}
	ch, err := st.CreateSpace(ctx, store.Space{
		ProjectID: f.ProjectID, WorkItemID: &f.ID, Name: "chat", Role: "lead",
		Team: "core",
	})
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(ctx, st, testMemory(t, st), t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), newRecordingAgent().factory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := testApplyTeam(t, st, ctx, store.Team{Name: "core", Roles: []store.Role{lead, architect}}); err != nil {
		t.Fatal(err)
	}

	err = s.RunAs(ctx, "tester", "plan it", nil, nil, discard)
	if !errors.Is(err, store.ErrInvalid) {
		t.Fatalf("RunAs for a role the Space lacks error = %v, want invalid", err)
	}
	turns, err := st.Turns(ctx, store.TurnFilter{SpaceID: &ch.ID})
	if err != nil || len(turns) != 1 {
		t.Fatalf("turns = %+v, %v, want one", turns, err)
	}
	got := turns[0]
	if got.Role != "tester" || got.Status != "failed" || got.WakeText != "plan it" ||
		got.Error != "invalid: team core has no role tester" {
		t.Errorf("turn = %+v, want a failed tester turn with wake \"plan it\" and the start error", got)
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
	developer := store.Role{Name: "developer", Write: true, Backend: "claude", Description: "You code."}
	if _, err := testApplyTeam(t, st, ctx, store.Team{Name: "core", Roles: []store.Role{lead, developer}}); err != nil {
		t.Fatal(err)
	}
	ch, err := st.CreateSpace(ctx, store.Space{
		ProjectID: f.ProjectID, WorkItemID: &f.ID, Name: "chat", Role: "lead",
		Team: "core",
	})
	if err != nil {
		t.Fatal(err)
	}
	dataDir, trees := t.TempDir(), NewTrees(t.TempDir())
	hung := hangingAgent{newRecordingAgent()}
	s1, err := Open(ctx, st, testMemory(t, st), dataDir, "/bin/reef", ch.ID, trees,
		func(string) (agent.Agent, error) { return hung, nil })
	if err != nil {
		t.Fatal(err)
	}
	runCtx, quit := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- s1.RunAs(runCtx, "developer", "build it", nil, nil, discard) }()
	want := agent.Usage{InputTokens: 10, OutputTokens: 7, CacheWriteTokens: 5}
	deadline := time.Now().Add(5 * time.Second)
	for {
		turns, err := st.Turns(ctx, store.TurnFilter{SpaceID: &ch.ID, Status: "running"})
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
	s2, err := Open(ctx, st, testMemory(t, st), dataDir, "/bin/reef", ch.ID, trees, func(string) (agent.Agent, error) {
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

	turns, err := st.Turns(ctx, store.TurnFilter{SpaceID: &ch.ID})
	if err != nil || len(turns) != 1 {
		t.Fatalf("turns = %+v, %v, want one", turns, err)
	}
	got := turns[0]
	if got.Status != "failed" || got.Error != "chat exited" || got.Usage != want {
		t.Errorf("turn = %q %q %+v, want failed, chat exited, %+v", got.Status, got.Error, got.Usage, want)
	}
	if !strings.HasSuffix(got.Prompt, "build it") {
		t.Errorf("prompt = %q, want the wake last", got.Prompt)
	}

	if used, err := st.SpaceTokens(ctx, ch.ID); err != nil || used != 22 {
		t.Errorf("space tokens = %d, %v, want 22", used, err)
	}
	over := store.Space{ID: ch.ID, TokenBudget: 22}
	if now, err := CheckCaps(ctx, st, &over); !now || !errors.Is(err, store.ErrConflict) {
		t.Errorf("CheckCaps at a budget of 22 = %v, %v, want paused now", now, err)
	}

	if err := s2.RunAs(ctx, "developer", "build more", nil, nil, discard); err != nil {
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
	ch := sessionSpace(t, st, f, false)
	ag := newRecordingAgent()
	s, err := Open(ctx, st, testMemory(t, st), t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), ag.factory)
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
	turns, err := st.Turns(ctx, store.TurnFilter{SpaceID: &ch.ID})
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

func TestSession_DirectBotUsesRecipientIDAfterRename(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch := sessionSpace(t, st, f, false)
	original := *testBotID(t, st, "architect")
	bot, err := st.Bot(ctx, original)
	if err != nil {
		t.Fatal(err)
	}
	bot.Name = "renamed"
	if _, err = st.UpdateBot(ctx, bot); err != nil {
		t.Fatal(err)
	}
	replacement := *testBotID(t, st, "architect")
	if _, err = st.AddSpaceMember(ctx, ch.ID, replacement); err != nil {
		t.Fatal(err)
	}
	sender := *testBotID(t, st, "lead")
	req, err := st.CreateDirectMessage(ctx, store.DirectMessage{SpaceID: ch.ID, From: "lead", To: "architect", FromBotID: &sender, ToBotID: &original, Body: "private"})
	if err != nil {
		t.Fatal(err)
	}
	ag := newRecordingAgent()
	sess, err := Open(ctx, st, testMemory(t, st), t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), ag.factory)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	if err = sess.RunDirectBot(ctx, replacement, "private", req, discard); !errors.Is(err, store.ErrInvalid) {
		t.Fatalf("wrong recipient = %v", err)
	}
	if ag.started {
		t.Fatal("wrong recipient started")
	}
	if err = sess.RunDirectBot(ctx, original, "private", req, discard); err != nil {
		t.Fatal(err)
	}
	directs, err := st.DirectMessages(ctx, ch.ID, 0)
	if err != nil || len(directs) != 2 || directs[1].FromBotID == nil || *directs[1].FromBotID != original || directs[1].From != "renamed" {
		t.Fatalf("directs = %+v, %v", directs, err)
	}
}

func TestSession_CoordinatorUsesEditedProfileOnReopen(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch := sessionSpace(t, st, f, false)
	id := *testBotID(t, st, "architect")
	bot, err := st.Bot(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	bot.Backend = "codex"
	bot.PermissionMode = "workspace-write"
	if _, err = st.UpdateBot(ctx, bot); err != nil {
		t.Fatal(err)
	}
	ag := newRecordingAgent()
	var backend string
	sess, err := Open(ctx, st, testMemory(t, st), t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), func(b string) (agent.Agent, error) { backend = b; return ag, nil })
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	if err = sess.Run(ctx, "hello", nil, discard); err != nil {
		t.Fatal(err)
	}
	if backend != "codex" || ag.cfg.PermissionMode != "workspace-write" {
		t.Fatalf("backend = %q, permission = %q", backend, ag.cfg.PermissionMode)
	}
}

func TestSession_ConnectionsReachOnlySelectedBots(t *testing.T) {
	ctx := context.Background()
	t.Setenv("HOME", t.TempDir())
	st, f := openStore(t)
	web, err := st.CreateConnection(ctx, store.Connection{Name: "web", URL: "http://x"})
	if err != nil {
		t.Fatal(err)
	}
	files, err := st.CreateConnection(ctx, store.Connection{Name: "files", Command: "files-mcp", Args: []string{"--ro"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testApplyTeam(t, st, ctx, store.Team{Name: "core", Roles: []store.Role{
		{Name: "lead", Lead: true, Backend: "claude"},
		{Name: "architect", Backend: "claude", Connections: []store.Connection{web, files}},
		{Name: "tester", Backend: "codex"},
	}}); err != nil {
		t.Fatal(err)
	}
	ch, err := st.CreateSpace(ctx, store.Space{
		ProjectID: f.ProjectID, WorkItemID: &f.ID, Name: "chat", Role: "lead", Team: "core",
	})
	if err != nil {
		t.Fatal(err)
	}
	root, err := st.CreateMessage(ctx, store.Message{SpaceID: ch.ID, Author: "user", Body: "root"})
	if err != nil {
		t.Fatal(err)
	}
	var ags []*recordingAgent
	mk := func(string) (agent.Agent, error) {
		ag := newRecordingAgent()
		ags = append(ags, ag)
		return ag, nil
	}
	s, err := Open(ctx, st, testMemory(t, st), t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), mk)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	for _, role := range []string{"architect", "tester"} {
		if err := s.RunAs(ctx, role, "go", &root.ID, nil, func(agent.Event) {}); err != nil {
			t.Fatal(err)
		}
	}
	if len(ags) != 2 {
		t.Fatalf("agents = %d, want 2", len(ags))
	}
	got := ags[0].cfg.MCPServers
	if got["web"].URL != "http://x" || got["files"].Command != "files-mcp" ||
		!reflect.DeepEqual(got["files"].Args, []string{"--ro"}) || len(got) != 3 {
		t.Errorf("architect servers = %+v, want reef, web by url and files by command", got)
	}
	if got := ags[1].cfg.MCPServers; len(got) != 1 || got["reef"].Command == "" {
		t.Errorf("tester servers = %+v, want only reef", got)
	}
}
