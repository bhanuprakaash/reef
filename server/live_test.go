package server

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/space"
	"github.com/bhanuprakaash/reef/store"
)

type fakeSession struct {
	st         *store.Store
	started    chan struct{}
	release    chan struct{}
	closed     atomic.Int32
	spaceID    atomic.Int64
	noProposal atomic.Bool

	// onRun, when set, runs inside every message turn: it stands in for the messages an
	// agent stores while it works.
	onRun func(role, text string, parent *int64)
	// tools, when set, names the tool each role calls at the start of its message turns.
	tools map[string]string

	mu     sync.Mutex
	ran    []string
	woke   []store.Scopes
	say    func()
	onNote func()

	// directStart receives a signal each time a direct turn starts, so tests can
	// synchronize on direct-turn execution without sleeping. It is buffered so a start
	// is never lost.
	directStart chan struct{}
	// directRelease, when non-nil, is waited on by RunDirectAs before it completes.
	directRelease chan struct{}
	// directFail, when non-nil, makes RunDirectAs fail with it and, for a request,
	// leaves a failed direct response so the sender can see that delivery failed.
	directFail    error
	directMu      sync.Mutex
	directRunning int32
	directMax     int32
}

func (f *fakeSession) Say(ctx context.Context, body string, parent *int64) (store.Message, error) {
	if f.say != nil {
		f.say()
	}
	return f.st.CreateMessage(ctx, store.Message{
		SpaceID: f.spaceID.Load(), ParentID: parent, Author: "user", Body: body,
	})
}

func (f *fakeSession) RunAs(
	ctx context.Context, role, text string, parent, _ *int64, onEvent func(agent.Event),
) error {
	f.mu.Lock()
	f.ran = append(f.ran, text)
	f.mu.Unlock()
	if f.onRun != nil {
		f.onRun(role, text, parent)
	}
	if tool := f.tools[role]; tool != "" {
		onEvent(agent.Event{Kind: agent.KindToolCall, ToolID: "t", ToolName: tool})
	}
	if strings.HasSuffix(text, "fail") {
		return errors.New("boom")
	}
	if !strings.HasSuffix(text, "block") {
		onEvent(agent.Event{Kind: agent.KindText, Text: "hi"})
		return nil
	}
	onEvent(agent.Event{Kind: agent.KindText, Text: "part1"})
	f.started <- struct{}{}
	select {
	case <-f.release:
		onEvent(agent.Event{Kind: agent.KindText, Text: "part2"})
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (f *fakeSession) WakeAs(
	ctx context.Context, role, routed string, scopes store.Scopes, parent, messageID *int64,
	onEvent func(agent.Event),
) error {
	f.mu.Lock()
	f.woke = append(f.woke, scopes)
	f.mu.Unlock()
	return f.RunAs(ctx, role, routed, parent, messageID, onEvent)
}

func (f *fakeSession) RunDirectAs(
	ctx context.Context, role, text string, dm store.DirectMessage, onEvent func(agent.Event),
) error {
	f.mu.Lock()
	f.ran = append(f.ran, text)
	f.mu.Unlock()
	if f.onRun != nil {
		f.onRun(role, text, nil)
	}
	if f.directRelease != nil {
		f.directMu.Lock()
		f.directRunning++
		if f.directRunning > f.directMax {
			f.directMax = f.directRunning
		}
		f.directMu.Unlock()
		select {
		case f.directStart <- struct{}{}:
		default:
		}
		select {
		case <-f.directRelease:
		case <-ctx.Done():
			if dm.ReplyToID == nil {
				_, _ = f.st.CreateFailedDirectResponse(context.WithoutCancel(ctx), dm.ID, role, "")
			}
			return ctx.Err()
		}
		f.directMu.Lock()
		f.directRunning--
		f.directMu.Unlock()
	}
	if f.directFail != nil {
		if dm.ReplyToID == nil {
			_, _ = f.st.CreateFailedDirectResponse(ctx, dm.ID, role, "")
		}
		return f.directFail
	}
	onEvent(agent.Event{Kind: agent.KindText, Text: "hi"})
	if dm.ReplyToID == nil {
		_, err := f.st.CreateDirectResponse(ctx, dm.ID, role, "hi")
		return err
	}
	return nil
}

func (f *fakeSession) Wrap(_ context.Context, onEvent func(agent.Event)) (store.Proposal, error) {
	if f.onRun != nil {
		f.onRun("lead", space.WrapCommand, nil)
	}
	onEvent(agent.Event{Kind: agent.KindText, Text: "wrapped"})
	if f.noProposal.Load() {
		return store.Proposal{}, space.ErrNoProposal
	}
	return store.Proposal{}, nil
}

func (f *fakeSession) SetOnNote(fn func()) {
	f.mu.Lock()
	f.onNote = fn
	f.mu.Unlock()
}

func (f *fakeSession) Close() error {
	f.closed.Add(1)
	return nil
}

// hookCtx calls hook on its first Done call, which the store makes as a query starts.
type hookCtx struct {
	context.Context
	once sync.Once
	hook func()
}

func (c *hookCtx) Done() <-chan struct{} {
	c.once.Do(c.hook)
	return c.Context.Done()
}

type liveEnv struct {
	dir    string
	st     *store.Store
	url    string
	ts     *httptest.Server
	h      *Handler
	fake   *fakeSession
	opened atomic.Int32
	open   func(ctx context.Context) (session, error)
}

func newLiveEnv(t *testing.T) *liveEnv {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	env := &liveEnv{dir: dir, fake: &fakeSession{started: make(chan struct{}, 1), release: make(chan struct{}), directStart: make(chan struct{}, 2)}}
	env.st, env.fake.st = st, st
	env.serve(t)
	do(t, http.MethodPost, env.url+"/projects", `{"name":"reef","path":"`+t.TempDir()+`"}`)
	do(t, http.MethodPost, env.url+"/spaces", `{"projectId":1,"name":"scratch","backend":"claude"}`)
	return env
}

func (e *liveEnv) serve(t *testing.T) {
	t.Helper()
	e.h = newHandler(e.st, testMemory(t, e.st), e.dir, space.NewTrees(e.dir), func(ctx context.Context, id int64) (session, error) {
		e.opened.Add(1)
		e.fake.spaceID.Store(id)
		if e.open != nil {
			return e.open(ctx)
		}
		return e.fake, nil
	})
	e.ts = httptest.NewServer(e.h)
	e.url = e.ts.URL
	t.Cleanup(e.h.Close)
	t.Cleanup(e.ts.Close)
}

// restart stops the server and starts a new one on the same data folder.
func (e *liveEnv) restart(t *testing.T) {
	t.Helper()
	e.h.Close()
	e.ts.Close()
	if err := e.st.Close(); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(e.dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	e.st, e.fake.st = st, st
	e.serve(t)
}

// exec runs stmt on a second connection to the database file, to break and mend the store.
func (e *liveEnv) exec(t *testing.T, stmt string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(e.dir, "reef.db")+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Error(err)
		return
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(stmt); err != nil {
		t.Error(err)
	}
}

func (e *liveEnv) waitIdle(t *testing.T, id int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for l := e.h.s.lookup(int64(id)); l != nil && l.running(); l = e.h.s.lookup(int64(id)) {
		if time.Now().After(deadline) {
			t.Fatal("chain did not end")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// overBudget uses up the token budget of Space 1 and sends a message, which pauses the
// Space. It returns the pause reason.
func (e *liveEnv) overBudget(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	turn, err := e.st.CreateTurn(ctx, store.Turn{SpaceID: 1, Role: "lead", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	turn.Status = "done"
	turn.Usage = agent.Usage{InputTokens: store.TokenBudget}
	if _, err := e.st.EndTurn(ctx, turn); err != nil {
		t.Fatal(err)
	}
	if status, body := e.send(t, "hello"); status != 409 {
		t.Fatalf("send over budget = %d %s, want 409", status, body)
	}
	ch, err := e.st.Space(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	return ch.Paused
}

func (e *liveEnv) send(t *testing.T, text string) (int, string) {
	t.Helper()
	return e.sendTo(t, 1, text)
}

func (e *liveEnv) sendTo(t *testing.T, id int, text string) (int, string) {
	t.Helper()
	return do(t, http.MethodPost, fmt.Sprintf("%s/spaces/%d/send", e.url, id), `{"text":"`+text+`"}`)
}

// workItemSpace adds a work item and a Space on it, and returns the Space's id.
func (e *liveEnv) workItemSpace(t *testing.T) int {
	t.Helper()
	do(t, http.MethodPost, e.url+"/work-items", `{"projectId":1,"title":"Login"}`)
	do(t, http.MethodPost, e.url+"/spaces", `{"projectId":1,"workItemId":1,"name":"c","backend":"claude"}`)
	return 2
}

type stream struct {
	frames chan string
}

const _idleFrame = `turn {"status":"idle"}`

func subscribe(t *testing.T, url string) *stream {
	t.Helper()
	return subscribeTo(t, url, 1)
}

func subscribeTo(t *testing.T, url string, id int) *stream {
	t.Helper()
	return subscribePath(t, fmt.Sprintf("%s/spaces/%d/events", url, id))
}

func subscribePath(t *testing.T, url string) *stream {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}
	s := &stream{frames: make(chan string, 64)}
	go func() {
		defer func() { _ = res.Body.Close() }()
		var event string
		sc := bufio.NewScanner(res.Body)
		for sc.Scan() {
			line := sc.Text()
			if v, ok := strings.CutPrefix(line, "event: "); ok {
				event = v
			}
			if v, ok := strings.CutPrefix(line, "data: "); ok {
				s.frames <- event + " " + v
			}
		}
		close(s.frames)
	}()
	return s
}

func (s *stream) next(t *testing.T) string {
	t.Helper()
	select {
	case f, ok := <-s.frames:
		if !ok {
			t.Fatal("stream ended")
		}
		return f
	case <-time.After(5 * time.Second):
		t.Fatal("no frame after 5s")
	}
	return ""
}

func (s *stream) want(t *testing.T, want ...string) {
	t.Helper()
	for _, w := range want {
		if got := s.next(t); got != w {
			t.Fatalf("frame = %s, want %s", got, w)
		}
	}
}

func TestSend_Streams(t *testing.T) {
	env := newLiveEnv(t)
	sub := subscribe(t, env.url)

	status, body := env.send(t, "hello")
	if status != 202 || body != "" {
		t.Fatalf("send = %d %q, want 202", status, body)
	}
	sub.want(t,
		_idleFrame,
		`turn {"role":"lead","status":"running"}`,
		`agent {"kind":"text","text":"hi"}`,
		`turn {"role":"lead","status":"done"}`,
	)

	late := subscribe(t, env.url)
	if status, _ := env.send(t, "hello"); status != 202 {
		t.Fatalf("second send = %d, want 202", status)
	}
	late.want(t,
		_idleFrame,
		`turn {"role":"lead","status":"running"}`,
		`agent {"kind":"text","text":"hi"}`,
		`turn {"role":"lead","status":"done"}`,
	)
	if got := env.opened.Load(); got != 1 {
		t.Errorf("sessions opened = %d, want 1: the agent stays alive between turns", got)
	}
}

func TestSend_IdlePostNotifiesTheStream(t *testing.T) {
	env := newLiveEnv(t)
	sub := subscribe(t, env.url)
	env.send(t, "hello")
	sub.want(t,
		_idleFrame,
		`turn {"role":"lead","status":"running"}`,
		`agent {"kind":"text","text":"hi"}`,
		`turn {"role":"lead","status":"done"}`,
	)

	env.fake.mu.Lock()
	note := env.fake.onNote
	env.fake.mu.Unlock()
	note()
	sub.want(t, `note {}`)
}

func TestSend_Parent(t *testing.T) {
	env := newLiveEnv(t)
	sub := subscribe(t, env.url)
	sub.want(t, _idleFrame)
	send := func(body string) int {
		status, _ := do(t, http.MethodPost, env.url+"/spaces/1/send", body)
		if status == 202 {
			sub.want(t, `turn {"role":"lead","status":"running"}`, `agent {"kind":"text","text":"hi"}`,
				`turn {"role":"lead","status":"done"}`)
		}
		return status
	}
	if status := send(`{"text":"root"}`); status != 202 {
		t.Fatalf("send = %d, want 202", status)
	}
	if status := send(`{"text":"reply","parent":1}`); status != 202 {
		t.Fatalf("send with parent = %d, want 202", status)
	}
	if status := send(`{"text":"deeper","parent":2}`); status != 400 {
		t.Errorf("send to a reply = %d, want 400", status)
	}
	_, body := do(t, http.MethodGet, env.url+"/messages?space=1", "")
	if !strings.Contains(body, `"parentId":null`) || !strings.Contains(body, `"parentId":1`) ||
		strings.Contains(body, "deeper") {
		t.Errorf("messages = %s, want root with null parentId and one reply with 1", body)
	}
}

// teamSpace adds the team core, a lead and an architect or the roles given, and a Space of it.
// It returns the Space's id and a function that stores a message as author.
func (e *liveEnv) teamSpace(t *testing.T, roles ...store.Role) (int, func(author, body string, parent *int64)) {
	t.Helper()
	if len(roles) == 0 {
		roles = []store.Role{
			{Name: "lead", Lead: true, Backend: "claude"},
			{Name: "architect", Backend: "claude"},
		}
	}
	_, err := e.st.ApplyTeam(context.Background(), store.Team{Name: "core", Roles: roles})
	if err != nil {
		t.Fatal(err)
	}
	do(t, http.MethodPost, e.url+"/spaces", `{"projectId":1,"name":"t","team":"core"}`)
	return 2, func(author, body string, parent *int64) {
		t.Helper()
		_, err := e.st.CreateMessage(context.Background(),
			store.Message{SpaceID: 2, ParentID: parent, Author: author, Body: body})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func (e *liveEnv) ranTexts() []string {
	e.fake.mu.Lock()
	defer e.fake.mu.Unlock()
	return slices.Clone(e.fake.ran)
}

func TestSend_RoutesAndWakes(t *testing.T) {
	env := newLiveEnv(t)
	id, post := env.teamSpace(t)
	env.fake.onRun = func(role, text string, parent *int64) {
		switch {
		case role == "lead":
			post("lead", "@architect plan it", parent)
		case role == "architect" && parent == nil:
			post("architect", "the plan", nil)
		case role == "architect":
			post("architect", "because", parent)
		}
	}
	sub := subscribeTo(t, env.url, id)

	if status, _ := env.sendTo(t, id, "login please"); status != 202 {
		t.Fatalf("send = %d, want 202", status)
	}
	sub.want(t, _idleFrame,
		`turn {"role":"lead","status":"running"}`, `agent {"kind":"text","text":"hi"}`,
		`turn {"role":"lead","status":"done"}`,
		`turn {"role":"architect","status":"running"}`, `agent {"kind":"text","text":"hi"}`,
		`turn {"role":"architect","status":"done"}`)

	if status, _ := do(t, http.MethodPost, fmt.Sprintf("%s/spaces/%d/send", env.url, id),
		`{"text":"why?","parent":3}`); status != 202 {
		t.Fatalf("reply in the thread = %d, want 202", status)
	}
	sub.want(t, `turn {"role":"architect","status":"running"}`, `agent {"kind":"text","text":"hi"}`,
		`turn {"role":"architect","status":"done"}`)

	want := []string{
		"[Space 2, message 1]\nlogin please",
		"[Space 2, message 2]\nlead wrote: @architect plan it",
		"[Space 2, message 4, thread 3]\nwhy?",
	}
	if got := env.ranTexts(); !slices.Equal(got, want) {
		t.Errorf("ran = %q, want %q", got, want)
	}
	msgs, err := env.st.Messages(context.Background(), int64(id), 0)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, m := range msgs {
		parent := "-"
		if m.ParentID != nil {
			parent = fmt.Sprint(*m.ParentID)
		}
		got = append(got, fmt.Sprintf("%d %s %s %q", m.ID, m.Author, parent, m.Body))
	}
	wantMsgs := []string{
		`1 user - "login please"`, `2 lead - "@architect plan it"`, `3 architect - "the plan"`,
		`4 user 3 "why?"`, `5 architect 3 "because"`,
	}
	if !slices.Equal(got, wantMsgs) {
		t.Errorf("messages = %q, want %q", got, wantMsgs)
	}
}

func TestSend_SpaceRoleStandsInForLead(t *testing.T) {
	env := newLiveEnv(t)
	_, err := env.st.CreateSpace(context.Background(), store.Space{
		ProjectID: 1, Name: "dev", Role: "developer", Backend: "claude",
	})
	if err != nil {
		t.Fatal(err)
	}
	sub := subscribeTo(t, env.url, 2)

	if status, _ := env.sendTo(t, 2, "@lead hi"); status != 202 {
		t.Fatalf("send = %d, want 202", status)
	}
	sub.want(t, _idleFrame,
		`turn {"role":"developer","status":"running"}`, `agent {"kind":"text","text":"hi"}`,
		`turn {"role":"developer","status":"done"}`)
	if got, want := env.ranTexts(), []string{"[Space 2, message 1]\n@lead hi"}; !slices.Equal(got, want) {
		t.Errorf("ran = %q, want %q", got, want)
	}
}

func TestSend_WakesByPost(t *testing.T) {
	env := newLiveEnv(t)
	id, post := env.teamSpace(t)
	env.fake.onRun = func(role, _ string, parent *int64) {
		if role == "lead" {
			post("lead", "@architect look at this", parent)
			post("lead", "done, nothing to add", parent)
		}
	}
	sub := subscribeTo(t, env.url, id)
	env.sendTo(t, id, "go")
	sub.want(t, _idleFrame,
		`turn {"role":"lead","status":"running"}`, `agent {"kind":"text","text":"hi"}`,
		`turn {"role":"lead","status":"done"}`,
		`turn {"role":"architect","status":"running"}`, `agent {"kind":"text","text":"hi"}`,
		`turn {"role":"architect","status":"done"}`)
	want := []string{
		"[Space 2, message 1]\ngo",
		"[Space 2, message 2]\nlead wrote: @architect look at this",
	}
	if got := env.ranTexts(); !slices.Equal(got, want) {
		t.Errorf("ran = %q, want %q: the post that mentions nobody wakes no one", got, want)
	}
}

func TestSend_DirectRequestGetsOnePrivateResponse(t *testing.T) {
	env := newLiveEnv(t)
	id, _ := env.teamSpace(t)
	var sent atomic.Bool
	env.fake.onRun = func(role, _ string, _ *int64) {
		if role == "lead" && sent.CompareAndSwap(false, true) {
			if _, err := env.st.CreateDirectMessage(context.Background(), store.DirectMessage{
				SpaceID: int64(id), From: "lead", To: "architect", Body: "design it",
			}); err != nil {
				t.Error(err)
			}
		}
	}
	sub := subscribeTo(t, env.url, id)
	env.sendTo(t, id, "go")
	sub.want(t, _idleFrame)
	sub.wantTurns(t, "lead")
	sub.want(t,
		`turn {"role":"architect","status":"running","directMessageId":1}`,
		`agent {"kind":"text","text":"hi"}`,
		`turn {"role":"architect","status":"done","directMessageId":1}`,
		`turn {"role":"lead","status":"running","directMessageId":2}`,
		`agent {"kind":"text","text":"hi"}`,
		`turn {"role":"lead","status":"done","directMessageId":2}`)

	direct, err := env.st.DirectMessages(context.Background(), int64(id), 0)
	if err != nil || len(direct) != 2 || direct[0].From != "lead" ||
		direct[1].From != "architect" || direct[1].To != "lead" {
		t.Errorf("direct messages = %+v, %v, want one request and one response", direct, err)
	}
	public, err := env.st.Messages(context.Background(), int64(id), 0)
	if err != nil || len(public) != 1 || public[0].Author != "user" {
		t.Errorf("public messages = %+v, %v, want only the user's message", public, err)
	}
	status, body := do(t, http.MethodGet,
		env.url+fmt.Sprintf("/spaces/%d/direct-messages", id), "")
	if status != 200 || !strings.Contains(body, `"from":"lead","to":"architect","body":"design it"`) ||
		!strings.Contains(body, `"from":"architect","to":"lead","body":"hi"`) {
		t.Errorf("GET direct messages = %d %s, want both sides of the exchange", status, body)
	}
	want := []string{
		"[Space 2, message 1]\ngo",
		"[Space 2, direct message 1, chain 1, source: direct request]\nlead wrote privately: design it",
		"[Space 2, direct message 2, chain 1, source: direct response]\narchitect wrote privately: hi",
	}
	if got := env.ranTexts(); !slices.Equal(got, want) {
		t.Errorf("ran = %q, want %q", got, want)
	}
}

func TestSend_OneWakePerRole(t *testing.T) {
	env := newLiveEnv(t)
	id, post := env.teamSpace(t)
	env.fake.onRun = func(role, _ string, parent *int64) {
		if role == "lead" {
			post("lead", "@architect look at this", parent)
			post("lead", "@architect and this too", parent)
		}
	}
	sub := subscribeTo(t, env.url, id)
	env.sendTo(t, id, "go")
	sub.want(t, _idleFrame,
		`turn {"role":"lead","status":"running"}`, `agent {"kind":"text","text":"hi"}`,
		`turn {"role":"lead","status":"done"}`,
		`turn {"role":"architect","status":"running"}`, `agent {"kind":"text","text":"hi"}`,
		`turn {"role":"architect","status":"done"}`)
	want := []string{
		"[Space 2, message 1]\ngo",
		"[Space 2, message 2]\nlead wrote: @architect look at this\n\n" +
			"[Space 2, message 3]\nlead wrote: @architect and this too",
	}
	if got := env.ranTexts(); !slices.Equal(got, want) {
		t.Errorf("ran = %q, want %q: one architect turn for both posts", got, want)
	}
}

func TestSend_TopLevelWakeHasNoThread(t *testing.T) {
	env := newLiveEnv(t)
	id, _ := env.teamSpace(t)
	var parent atomic.Pointer[int64]
	parent.Store(new(int64))
	env.fake.onRun = func(_, _ string, p *int64) { parent.Store(p) }
	sub := subscribeTo(t, env.url, id)
	env.sendTo(t, id, "@architect plan it")
	sub.want(t, _idleFrame)
	sub.wantTurns(t, "architect")
	if got := env.ranTexts(); !slices.Equal(got, []string{"[Space 2, message 1]\n@architect plan it"}) {
		t.Errorf("ran = %q, want a header with no thread", got)
	}
	if got := parent.Load(); got != nil {
		t.Errorf("architect reply parent = %d, want nil: the reply is top-level", *got)
	}
}

func TestSend_RoutesOnlyItsOwnMessage(t *testing.T) {
	env := newLiveEnv(t)
	id, post := env.teamSpace(t)
	env.fake.say = func() { post("user", "@architect plan it", nil) }
	sub := subscribeTo(t, env.url, id)
	env.sendTo(t, id, "go")
	sub.want(t, _idleFrame)
	sub.wantTurns(t, "lead")
	if got := env.ranTexts(); !slices.Equal(got, []string{"[Space 2, message 2]\ngo"}) {
		t.Errorf("ran = %q, want only the send: a message stored beside it is not routed", got)
	}
}

func TestSend_OneWakePerRoleAcrossThreads(t *testing.T) {
	env := newLiveEnv(t)
	id, post := env.teamSpace(t)
	env.fake.onRun = func(role, _ string, _ *int64) {
		if role == "lead" {
			one := int64(1)
			post("lead", "@architect one", nil)
			post("lead", "@architect two", &one)
		}
	}
	sub := subscribeTo(t, env.url, id)
	env.sendTo(t, id, "go")
	sub.want(t, _idleFrame)
	sub.wantTurns(t, "lead", "architect")
	want := []string{
		"[Space 2, message 1]\ngo",
		"[Space 2, message 2]\nlead wrote: @architect one\n\n" +
			"[Space 2, message 3, thread 1]\nlead wrote: @architect two",
	}
	if got := env.ranTexts(); !slices.Equal(got, want) {
		t.Errorf("ran = %q, want %q: a top-level post and a thread reply wake the architect once", got, want)
	}
}

func TestSend_MentionsWakeEachRole(t *testing.T) {
	env := newLiveEnv(t)
	id, _ := env.teamSpace(t)
	sub := subscribeTo(t, env.url, id)
	env.sendTo(t, id, "@architect @lead hi")
	sub.want(t, _idleFrame,
		`turn {"role":"architect","status":"running"}`, `agent {"kind":"text","text":"hi"}`,
		`turn {"role":"architect","status":"done"}`,
		`turn {"role":"lead","status":"running"}`, `agent {"kind":"text","text":"hi"}`,
		`turn {"role":"lead","status":"done"}`)
}

func TestSend_SeveralMentionsShareOneMessage(t *testing.T) {
	env := newLiveEnv(t)
	id, post := env.teamSpace(t)
	env.fake.onRun = func(role, _ string, parent *int64) {
		post(role, role+" says "+map[string]string{"architect": "SQLite", "lead": "Postgres"}[role], parent)
	}
	sub := subscribeTo(t, env.url, id)
	env.sendTo(t, id, "@architect @lead @architect @ghost which database")
	sub.want(t, _idleFrame)
	sub.wantTurns(t, "architect", "lead")
	body := "@architect @lead @architect @ghost which database"
	header := "[Space 2, message 1]\nAsked together: @architect, @lead\n"
	want := []string{header + body, header + body}
	if got := env.ranTexts(); !slices.Equal(got, want) {
		t.Errorf("ran = %q, want %q: one wake each, both naming the full set", got, want)
	}
	msgs, err := env.st.Messages(context.Background(), int64(id), 0)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, m := range msgs {
		got = append(got, fmt.Sprintf("%d %s %v %q", m.ID, m.Author, m.ParentID, m.Body))
	}
	wantMsgs := []string{
		`1 user <nil> "` + body + `"`,
		`2 architect <nil> "architect says SQLite"`,
		`3 lead <nil> "lead says Postgres"`,
	}
	if !slices.Equal(got, wantMsgs) {
		t.Errorf("messages = %q, want %q: one user message, then both disagreeing replies public", got, wantMsgs)
	}
	direct, err := env.st.DirectMessages(context.Background(), int64(id), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(direct) != 0 {
		t.Errorf("direct messages = %d, want 0: @ghost and repeats create none", len(direct))
	}
}

func TestSend_ThreadReplyKeepsOwnerOverMentions(t *testing.T) {
	env := newLiveEnv(t)
	id, post := env.teamSpace(t)
	post("architect", "the plan", nil)
	sub := subscribeTo(t, env.url, id)
	if status, body := do(t, http.MethodPost, fmt.Sprintf("%s/spaces/%d/send", env.url, id),
		`{"text":"@lead @architect why?","parent":1}`); status != 202 {
		t.Fatalf("send = %d %s, want 202", status, body)
	}
	sub.want(t, _idleFrame)
	sub.wantTurns(t, "architect")
	want := []string{"[Space 2, message 2, thread 1]\n@lead @architect why?"}
	if got := env.ranTexts(); !slices.Equal(got, want) {
		t.Errorf("ran = %q, want %q: only the thread owner wakes, no Asked together line", got, want)
	}
}

func TestSend_SeveralMentionsInThreadKeepThread(t *testing.T) {
	env := newLiveEnv(t)
	_, err := env.st.ApplyTeam(context.Background(), store.Team{Name: "trio", Roles: []store.Role{
		{Name: "lead", Lead: true, Backend: "claude"},
		{Name: "architect", Backend: "claude"},
		{Name: "tester", Backend: "claude"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	do(t, http.MethodPost, env.url+"/spaces", `{"projectId":1,"name":"t","team":"trio"}`)
	env.fake.onRun = func(role, _ string, parent *int64) {
		if role == "lead" {
			one := int64(1)
			_, err := env.st.CreateMessage(context.Background(), store.Message{
				SpaceID: 2, ParentID: &one, Author: "lead", Body: "@architect @tester @lead split it",
			})
			if err != nil {
				t.Error(err)
			}
		}
	}
	sub := subscribeTo(t, env.url, 2)
	env.sendTo(t, 2, "go")
	sub.want(t, _idleFrame)
	sub.wantTurns(t, "lead", "architect", "tester")
	post := "[Space 2, message 2, thread 1]\nAsked together: @architect, @tester\n" +
		"lead wrote: @architect @tester @lead split it"
	want := []string{"[Space 2, message 1]\ngo", post, post}
	if got := env.ranTexts(); !slices.Equal(got, want) {
		t.Errorf("ran = %q, want %q: both recipients are woken with the full set, not the sender", got, want)
	}
}

func TestSend_SelfMentionDoesNotWakeSender(t *testing.T) {
	env := newLiveEnv(t)
	id, post := env.teamSpace(t)
	env.fake.onRun = func(role, _ string, parent *int64) {
		if role == "lead" {
			post("lead", "@lead @architect over to you", parent)
		}
	}
	sub := subscribeTo(t, env.url, id)
	env.sendTo(t, id, "go")
	sub.want(t, _idleFrame)
	sub.wantTurns(t, "lead", "architect")
	want := []string{
		"[Space 2, message 1]\ngo",
		"[Space 2, message 2]\nlead wrote: @lead @architect over to you",
	}
	if got := env.ranTexts(); !slices.Equal(got, want) {
		t.Errorf("ran = %q, want %q: lead's own mention wakes nobody", got, want)
	}
}

func TestSend_WhileWakeChainRuns(t *testing.T) {
	env := newLiveEnv(t)
	id, post := env.teamSpace(t)
	env.fake.onRun = func(role, text string, _ *int64) {
		if role == "lead" && strings.HasSuffix(text, "\ngo") {
			post("lead", "@architect block", nil)
		}
	}
	sub := subscribeTo(t, env.url, id)
	env.sendTo(t, id, "go")
	<-env.fake.started
	sub.want(t, _idleFrame,
		`turn {"role":"lead","status":"running"}`, `agent {"kind":"text","text":"hi"}`,
		`turn {"role":"lead","status":"done"}`,
		`turn {"role":"architect","status":"running"}`, `agent {"kind":"text","text":"part1"}`)

	if status, body := env.sendTo(t, id, "again"); status != 202 || body != "" {
		t.Fatalf("send during the wake = %d %q, want 202", status, body)
	}
	env.fake.release <- struct{}{}
	sub.want(t, `agent {"kind":"text","text":"part2"}`, `turn {"role":"architect","status":"done"}`)
	sub.wantTurns(t, "lead")
	want := []string{
		"[Space 2, message 1]\ngo",
		"[Space 2, message 2]\nlead wrote: @architect block",
		"[Space 2, message 3]\nagain",
	}
	if got := env.ranTexts(); !slices.Equal(got, want) {
		t.Errorf("ran = %q, want %q", got, want)
	}
}

// wantMessages checks the author and body of every message of Space id, in order.
func (e *liveEnv) wantMessages(t *testing.T, id int, want ...string) {
	t.Helper()
	msgs, err := e.st.Messages(context.Background(), int64(id), 0)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(msgs))
	for i, m := range msgs {
		got[i] = m.Author + ": " + m.Body
	}
	if !slices.Equal(got, want) {
		t.Errorf("messages = %q, want %q", got, want)
	}
}

const _droppedNote = "reef: Wakes for architect did not run: the turn before them was stopped or failed. " +
	"Send a message to ask again."

func TestSend_AfterStopNotesTheWakeStopDropped(t *testing.T) {
	env := newLiveEnv(t)
	id, _ := env.teamSpace(t)
	env.sendTo(t, id, "block")
	<-env.fake.started
	if status, _ := env.sendTo(t, id, "@architect design it"); status != 202 {
		t.Fatalf("send while busy = %d, want 202", status)
	}
	do(t, http.MethodPost, fmt.Sprintf("%s/spaces/%d/stop", env.url, id), "")
	if status, _ := env.sendTo(t, id, "again"); status != 202 {
		t.Fatalf("send after stop = %d, want 202", status)
	}
	env.waitIdle(t, id)
	env.wantMessages(t, id, "user: block", "user: @architect design it", "user: again", _droppedNote)
}

func TestSend_AfterStopNamesARoleOnceForTwoWakes(t *testing.T) {
	env := newLiveEnv(t)
	id, _ := env.teamSpace(t)
	env.sendTo(t, id, "block")
	<-env.fake.started
	env.sendTo(t, id, "@architect design it")
	env.sendTo(t, id, "@architect and test it")
	do(t, http.MethodPost, fmt.Sprintf("%s/spaces/%d/stop", env.url, id), "")
	env.sendTo(t, id, "again")
	env.waitIdle(t, id)
	env.wantMessages(t, id, "user: block", "user: @architect design it", "user: @architect and test it",
		"user: again", _droppedNote)
}

func TestSend_AfterFailedTurnNotesTheLeftoverWakes(t *testing.T) {
	env := newLiveEnv(t)
	id, _ := env.teamSpace(t)
	env.sendTo(t, id, "@lead @architect fail")
	env.waitIdle(t, id)
	if status, _ := env.sendTo(t, id, "again"); status != 202 {
		t.Fatalf("send after the failed turn = %d, want 202", status)
	}
	env.waitIdle(t, id)
	env.wantMessages(t, id, "user: @lead @architect fail", "user: again", _droppedNote)
	if got := len(env.ranTexts()); got != 2 {
		t.Errorf("turns run = %d, want 2: the failed lead turn and again", got)
	}
}

func TestSend_Wrap(t *testing.T) {
	env := newLiveEnv(t)
	id := env.workItemSpace(t)
	sub := subscribeTo(t, env.url, id)
	if status, _ := env.sendTo(t, id, "/wrap"); status != 202 {
		t.Fatalf("send = %d, want 202", status)
	}
	sub.want(t,
		_idleFrame,
		`turn {"role":"lead","status":"running"}`,
		`agent {"kind":"text","text":"wrapped"}`,
		`turn {"role":"lead","status":"done"}`,
	)
}

func TestSubscribe_ReplaysRunningTurn(t *testing.T) {
	env := newLiveEnv(t)
	first := subscribe(t, env.url)
	env.send(t, "block")
	<-env.fake.started
	first.want(t, _idleFrame, `turn {"role":"lead","status":"running"}`, `agent {"kind":"text","text":"part1"}`)

	second := subscribe(t, env.url)
	second.want(t, `turn {"role":"lead","status":"running"}`, `agent {"kind":"text","text":"part1"}`)

	env.fake.release <- struct{}{}
	for _, s := range []*stream{first, second} {
		s.want(t, `agent {"kind":"text","text":"part2"}`, `turn {"role":"lead","status":"done"}`)
	}
}

func TestSend_WhileRunning(t *testing.T) {
	env := newLiveEnv(t)
	sub := subscribe(t, env.url)
	env.send(t, "block")
	<-env.fake.started
	sub.want(t, _idleFrame, `turn {"role":"lead","status":"running"}`, `agent {"kind":"text","text":"part1"}`)

	status, body := env.send(t, "again")
	if status != 202 || body != "" {
		t.Fatalf("send = %d %q, want 202", status, body)
	}
	if _, body := do(t, http.MethodGet, env.url+"/messages?space=1", ""); !strings.Contains(body, `"body":"again"`) {
		t.Errorf("messages = %s, want again stored before the turn ends", body)
	}
	if got := env.ranTexts(); len(got) != 1 {
		t.Errorf("ran = %q, want only block: again waits for the turn", got)
	}
	env.fake.release <- struct{}{}
	sub.want(t, `agent {"kind":"text","text":"part2"}`, `turn {"role":"lead","status":"done"}`)
	sub.wantTurns(t, "lead")
	want := []string{"[Space 1, message 1]\nblock", "[Space 1, message 2]\nagain"}
	if got := env.ranTexts(); !slices.Equal(got, want) {
		t.Errorf("ran = %q, want %q", got, want)
	}
}

// sendWhileOpening sends first, holds its agent open, sends second, and returns the statuses
// of both once open returns what it is given. The second send must wait for the first chain.
func sendWhileOpening(
	t *testing.T, env *liveEnv, first, second string, open func() (session, error),
) (int, int) {
	t.Helper()
	opening, proceed := make(chan struct{}), make(chan struct{})
	var opens atomic.Int32
	env.open = func(context.Context) (session, error) {
		if opens.Add(1) > 1 {
			return env.fake, nil
		}
		opening <- struct{}{}
		<-proceed
		return open()
	}
	one, two := make(chan int, 1), make(chan int, 1)
	go func() {
		status, _ := env.send(t, first)
		one <- status
	}()
	<-opening
	go func() {
		status, _ := env.send(t, second)
		two <- status
	}()
	select {
	case status := <-two:
		close(proceed)
		t.Fatalf("send while starting = %d before the chain started, want it to wait", status)
	case <-time.After(100 * time.Millisecond):
	}
	close(proceed)
	return <-one, <-two
}

func TestSend_WhileStarting(t *testing.T) {
	env := newLiveEnv(t)
	sub := subscribe(t, env.url)
	one, two := sendWhileOpening(t, env, "hello", "again", func() (session, error) { return env.fake, nil })
	if one != 202 || two != 202 {
		t.Fatalf("sends = %d and %d, want 202 and 202", one, two)
	}
	sub.want(t, _idleFrame)
	sub.wantTurns(t, "lead", "lead")
	msgs, err := env.st.Messages(context.Background(), 1, 0)
	if err != nil || len(msgs) != 2 || msgs[0].ID != 1 || msgs[0].Body != "hello" ||
		msgs[1].ID != 2 || msgs[1].Body != "again" {
		t.Errorf("messages = %+v, %v, want 1 hello, 2 again", msgs, err)
	}
	want := []string{"[Space 1, message 1]\nhello", "[Space 1, message 2]\nagain"}
	if got := env.ranTexts(); !slices.Equal(got, want) {
		t.Errorf("ran = %q, want %q", got, want)
	}
}

func TestSend_WhileStartFails(t *testing.T) {
	env := newLiveEnv(t)
	sub := subscribe(t, env.url)
	one, two := sendWhileOpening(t, env, "hello", "again", func() (session, error) {
		return nil, errors.New("boom")
	})
	if one != 500 || two != 202 {
		t.Fatalf("sends = %d and %d, want 500 and 202: the second starts a chain of its own", one, two)
	}
	sub.want(t, _idleFrame)
	sub.wantTurns(t, "lead")
	if got := env.ranTexts(); !slices.Equal(got, []string{"[Space 1, message 1]\nagain"}) {
		t.Errorf("ran = %q, want only again", got)
	}
}

func TestSend_WhileWrapStarts(t *testing.T) {
	env := newLiveEnv(t)
	id := env.workItemSpace(t)
	var status int
	var body string
	env.open = func(context.Context) (session, error) {
		status, body = env.sendTo(t, id, "again")
		return env.fake, nil
	}
	sub := subscribeTo(t, env.url, id)
	if s, _ := env.sendTo(t, id, "/wrap"); s != 202 {
		t.Fatalf("wrap = %d, want 202", s)
	}
	if status != 409 || body != `{"error":"space 2 has a turn running: conflict"}` {
		t.Errorf("send while the wrap starts = %d %s, want 409 turn running", status, body)
	}
	sub.want(t, _idleFrame,
		`turn {"role":"lead","status":"running"}`, `agent {"kind":"text","text":"wrapped"}`,
		`turn {"role":"lead","status":"done"}`)
	if msgs, err := env.st.Messages(context.Background(), int64(id), 0); err != nil || len(msgs) != 1 {
		t.Errorf("messages = %+v, %v, want only the wrap command", msgs, err)
	}
}

func TestSend_WrapWakesNobody(t *testing.T) {
	env := newLiveEnv(t)
	env.teamSpace(t)
	do(t, http.MethodPost, env.url+"/work-items", `{"projectId":1,"title":"Login"}`)
	do(t, http.MethodPost, env.url+"/spaces", `{"projectId":1,"workItemId":1,"name":"w","team":"core"}`)
	env.fake.onRun = func(string, string, *int64) {
		_, err := env.st.CreateMessage(context.Background(),
			store.Message{SpaceID: 3, Author: "lead", Body: "@architect look at this"})
		if err != nil {
			t.Error(err)
		}
	}
	sub := subscribeTo(t, env.url, 3)
	env.sendTo(t, 3, "/wrap")
	sub.want(t, _idleFrame,
		`turn {"role":"lead","status":"running"}`, `agent {"kind":"text","text":"wrapped"}`,
		`turn {"role":"lead","status":"done"}`)

	env.fake.onRun = nil
	env.sendTo(t, 3, "ping")
	sub.want(t, `turn {"role":"lead","status":"running"}`, `agent {"kind":"text","text":"hi"}`,
		`turn {"role":"lead","status":"done"}`)
	if got := env.ranTexts(); !slices.Equal(got, []string{"[Space 3, message 3]\nping"}) {
		t.Errorf("ran = %q, want only ping: the wrap reply wakes no architect", got)
	}
}

func (s *stream) wantTurns(t *testing.T, roles ...string) {
	t.Helper()
	for _, role := range roles {
		s.want(t, `turn {"role":"`+role+`","status":"running"}`, `agent {"kind":"text","text":"hi"}`,
			`turn {"role":"`+role+`","status":"done"}`)
	}
}

func TestSend_UserPostMidChainIsNotRouted(t *testing.T) {
	env := newLiveEnv(t)
	id, post := env.teamSpace(t)
	var runs atomic.Int32
	env.fake.onRun = func(role, _ string, _ *int64) {
		if runs.Add(1) == 3 {
			status, body := do(t, http.MethodPost, env.url+"/messages",
				`{"spaceId":2,"author":"user","body":"@architect hey"}`)
			if status != 201 {
				t.Errorf("post = %d %s, want 201", status, body)
			}
		}
		if role == "lead" {
			post("lead", "@architect again", nil)
		} else {
			post("architect", "@lead again", nil)
		}
	}
	sub := subscribeTo(t, env.url, id)
	env.sendTo(t, id, "go")
	sub.want(t, _idleFrame)
	sub.wantTurns(t, "lead", "architect", "lead", "architect", "lead", "architect", "lead")
	sub.want(t, `pause {"reason":"`+_hopPause+`"}`)
	if got := len(env.ranTexts()); got != 7 {
		t.Errorf("turns run = %d, want 7: the user's post starts no turn and does not reset the hops", got)
	}
}

func TestSend_TokenBudgetPausesSpace(t *testing.T) {
	ctx := context.Background()
	env := newLiveEnv(t)
	turn, err := env.st.CreateTurn(ctx, store.Turn{SpaceID: 1, Role: "lead", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	turn.Status = "done"
	turn.Usage = agent.Usage{InputTokens: 1000000, OutputTokens: 312, CacheReadTokens: 5000000}
	if _, err := env.st.EndTurn(ctx, turn); err != nil {
		t.Fatal(err)
	}
	sub := subscribe(t, env.url)

	const reason = "token budget of 1000000 used: 1000312 tokens"
	status, body := env.send(t, "hello")
	if status != 409 || body != `{"error":"space 1 is paused: `+reason+`: conflict"}` {
		t.Fatalf("send over budget = %d %s, want 409 with the reason", status, body)
	}
	sub.want(t, _idleFrame, `pause {"reason":"`+reason+`"}`)
	if ch, err := env.st.Space(ctx, 1); err != nil || ch.Paused != reason {
		t.Errorf("space = %+v, %v, want paused with the reason", ch, err)
	}

	if status, _ := do(t, http.MethodPost, env.url+"/spaces/1/continue", ""); status != 204 {
		t.Fatalf("continue = %d, want 204", status)
	}
	if ch, err := env.st.Space(ctx, 1); err != nil || ch.Paused != "" || ch.TokenBudget != 2000312 {
		t.Errorf("space after continue = %+v, %v, want not paused with budget 2000312", ch, err)
	}
	if status, _ := env.send(t, "hello"); status != 202 {
		t.Errorf("send after continue = %d, want 202", status)
	}
}

type meetSession struct {
	*fakeSession
	closing *sync.WaitGroup
	met     *atomic.Int32
}

// Close returns only once every session is closing, so it never returns when the
// sessions are closed one after another.
func (m *meetSession) Close() error {
	m.closing.Done()
	m.closing.Wait()
	m.met.Add(1)
	return nil
}

func TestSend_OpenErrors(t *testing.T) {
	tests := []struct {
		name       string
		giveErr    error
		wantStatus int
		wantBody   string
	}{
		{"unknown space", store.ErrNotFound, 404, `{"error":"not found"}`},
		{"bad backend", store.ErrInvalid, 400, `{"error":"invalid"}`},
		{"open elsewhere", store.ErrConflict, 409, `{"error":"conflict"}`},
		{"other", errors.New("boom"), 500, `{"error":"internal error"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st, err := store.Open(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = st.Close() })
			if _, err := st.CreateProject(context.Background(), store.Project{Name: "reef", Path: "/tmp/reef"}); err != nil {
				t.Fatal(err)
			}
			if _, err := st.CreateSpace(context.Background(), store.Space{ProjectID: 1, Name: "chat"}); err != nil {
				t.Fatal(err)
			}
			h := newHandler(st, nil, "", space.NewTrees(t.TempDir()), func(context.Context, int64) (session, error) { return nil, tt.giveErr })
			t.Cleanup(h.Close)
			ts := httptest.NewServer(h)
			t.Cleanup(ts.Close)
			status, body := do(t, http.MethodPost, ts.URL+"/spaces/1/send", `{"text":"hi"}`)
			if status != tt.wantStatus || body != tt.wantBody {
				t.Errorf("send = %d %s, want %d %s", status, body, tt.wantStatus, tt.wantBody)
			}
			if status, _ = do(t, http.MethodPost, ts.URL+"/spaces/1/stop", ""); status != 204 {
				t.Errorf("stop after failed open = %d, want 204", status)
			}
		})
	}
}

func TestLiveRoutes_Errors(t *testing.T) {
	env := newLiveEnv(t)
	tests := []struct {
		name       string
		method     string
		path       string
		give       string
		wantStatus int
		wantBody   string
	}{
		{"no text", http.MethodPost, "/spaces/1/send", `{"text":""}`, 400, `{"error":"invalid: text is required"}`},
		{"unknown field", http.MethodPost, "/spaces/1/send", `{"body":"x"}`, 400,
			`{"error":"invalid: bad JSON: json: unknown field \"body\""}`},
		{"events of unknown space", http.MethodGet, "/spaces/9/events", "", 404,
			`{"error":"space 9: not found"}`},
		{"send by GET", http.MethodGet, "/spaces/1/send", "", 405, `{"error":"method not allowed"}`},
		{"events by POST", http.MethodPost, "/spaces/1/events", "", 405, `{"error":"method not allowed"}`},
		{"stop by GET", http.MethodGet, "/spaces/1/stop", "", 405, `{"error":"method not allowed"}`},
		{"unknown backend", http.MethodPost, "/spaces", `{"projectId":1,"name":"c","backend":"gpt"}`, 400,
			`{"error":"invalid: backend must be claude or codex"}`},
		{"bad permission mode", http.MethodPost, "/spaces",
			`{"projectId":1,"name":"c","backend":"codex","permissionMode":"acceptEdits"}`, 400,
			`{"error":"invalid: permissionMode for codex must be empty, read-only or workspace-write"}`},
		{"claude permission mode", http.MethodPost, "/spaces",
			`{"projectId":1,"name":"c","backend":"claude","permissionMode":"workspace-write"}`, 400,
			`{"error":"invalid: permissionMode for claude must be empty, default or acceptEdits"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := do(t, tt.method, env.url+tt.path, tt.give)
			if status != tt.wantStatus || body != tt.wantBody {
				t.Errorf("%s %s = %d %s, want %d %s",
					tt.method, tt.path, status, body, tt.wantStatus, tt.wantBody)
			}
		})
	}
}

func TestCreateSpace_RoleDefaultsToLead(t *testing.T) {
	env := newLiveEnv(t)
	status, body := do(t, http.MethodPost, env.url+"/spaces",
		`{"projectId":1,"name":"c","backend":"codex","permissionMode":"workspace-write"}`)
	if status != 201 || !strings.Contains(body, `"role":"lead","backend":"codex","permissionMode":"workspace-write"`) {
		t.Errorf("create = %d %s, want role lead", status, body)
	}
}

func TestSend_StoresUserMessageBefore202(t *testing.T) {
	env := newLiveEnv(t)
	if status, _ := env.send(t, "block"); status != 202 {
		t.Fatalf("send = %d, want 202", status)
	}
	_, body := do(t, http.MethodGet, env.url+"/messages?space=1", "")
	if !strings.Contains(body, `"author":"user","body":"block"`) {
		t.Errorf("messages right after 202 = %s, want the user message block", body)
	}
	env.fake.release <- struct{}{}
}

func TestSubscribe_StateOnConnect(t *testing.T) {
	env := newLiveEnv(t)
	sub := subscribe(t, env.url)
	reason := env.overBudget(t)
	sub.want(t, _idleFrame, `pause {"reason":"`+reason+`"}`)

	late := subscribe(t, env.url)
	late.want(t, _idleFrame, `pause {"reason":"`+reason+`"}`)

	do(t, http.MethodPost, env.url+"/spaces/1/continue", "")
	again := subscribe(t, env.url)
	again.want(t, _idleFrame)
	if status, _ := env.send(t, "hello"); status != 202 {
		t.Fatalf("send after continue = %d, want 202", status)
	}
	again.want(t, `turn {"role":"lead","status":"running"}`)
}

func TestWrap_QuickSession(t *testing.T) {
	env := newLiveEnv(t)
	const want = `{"error":"invalid: Space 1 has no work item, attach it to one before wrapping up"}`
	if status, body := do(t, http.MethodPost, env.url+"/spaces/1/wrap", ""); status != 400 || body != want {
		t.Errorf("wrap = %d %s, want 400 %s", status, body, want)
	}
	if status, body := env.send(t, "/wrap"); status != 400 || body != want {
		t.Errorf("send /wrap = %d %s, want 400 %s", status, body, want)
	}
	if got := env.opened.Load(); got != 0 {
		t.Errorf("sessions opened = %d, want 0: no agent turn for a refused wrap", got)
	}
}

func TestWrap_NoProposalNote(t *testing.T) {
	env := newLiveEnv(t)
	env.fake.noProposal.Store(true)
	id := env.workItemSpace(t)
	sub := subscribeTo(t, env.url, id)
	env.sendTo(t, id, "/wrap")
	sub.want(t, _idleFrame, `turn {"role":"lead","status":"running"}`, `agent {"kind":"text","text":"wrapped"}`,
		`turn {"role":"lead","status":"done","note":"no proposal made"}`)
}

func TestSend_RefusedWhileSpaceIsBeingDeleted(t *testing.T) {
	env := newLiveEnv(t)
	lift := env.h.s.stopForDelete(1)

	const want = `{"error":"space 1 is being deleted: not found"}`
	if status, body := env.send(t, "hello"); status != 404 || body != want {
		t.Errorf("send = %d %s, want 404 %s", status, body, want)
	}
	if got := env.opened.Load(); got != 0 {
		t.Errorf("sessions opened = %d, want 0", got)
	}
	env.h.s.mu.Lock()
	entries := len(env.h.s.lives)
	env.h.s.mu.Unlock()
	if entries != 0 {
		t.Errorf("entries = %d, want 0", entries)
	}

	lift()
	if status, _ := env.send(t, "hello"); status != 202 {
		t.Errorf("send after the marks are lifted = %d, want 202", status)
	}
}

func TestLive_EntryOnlyForATurn(t *testing.T) {
	env := newLiveEnv(t)
	entries := func() int {
		env.h.s.mu.Lock()
		defer env.h.s.mu.Unlock()
		return len(env.h.s.lives)
	}
	if status, _ := do(t, http.MethodPost, env.url+"/spaces/1/stop", ""); status != 204 {
		t.Fatalf("stop = %d, want 204", status)
	}
	if err := env.st.PauseSpace(context.Background(), 1, "capped"); err != nil {
		t.Fatal(err)
	}
	if status, _ := env.send(t, "hello"); status != 409 {
		t.Fatalf("send to a paused Space = %d, want 409", status)
	}
	if status, _ := do(t, http.MethodPost, env.url+"/spaces/1/continue", ""); status != 204 {
		t.Fatalf("continue = %d, want 204", status)
	}
	if status, _ := env.send(t, "/wrap"); status != 400 {
		t.Fatalf("wrap on a quick session = %d, want 400", status)
	}
	if got := entries(); got != 0 {
		t.Fatalf("entries = %d, want 0 before any turn", got)
	}

	if status, _ := env.send(t, "hello"); status != 202 {
		t.Fatalf("send = %d, want 202", status)
	}
	if got := entries(); got != 1 {
		t.Errorf("entries = %d, want 1 after a turn", got)
	}
	if status, _ := do(t, http.MethodDelete, env.url+"/spaces/1", ""); status != 204 {
		t.Fatalf("delete = %d, want 204", status)
	}
	if got := entries(); got != 0 {
		t.Errorf("entries = %d, want 0 after delete", got)
	}
}

func TestSend_DirectReadFailurePausesAndContinueRunsPublicWake(t *testing.T) {
	ctx := context.Background()
	env := newLiveEnv(t)
	id, post := env.teamSpace(t)
	env.fake.onRun = func(role, _ string, _ *int64) {
		if role == "lead" {
			post("lead", "@architect plan it", nil)
			env.exec(t, "ALTER TABLE direct_message RENAME TO direct_message_gone")
		}
	}
	sub := subscribeTo(t, env.url, id)

	env.sendTo(t, id, "go")
	const reason = "route direct messages: list direct messages: SQL logic error: no such table: direct_message (1)"
	sub.want(t, _idleFrame, `turn {"role":"lead","status":"running"}`, `agent {"kind":"text","text":"hi"}`,
		`turn {"role":"lead","status":"done"}`, `pause {"reason":"`+reason+`"}`)
	if ch, err := env.st.Space(ctx, int64(id)); err != nil || ch.Paused != reason {
		t.Errorf("space = %+v, %v, want paused with %q", ch, err, reason)
	}

	env.exec(t, "ALTER TABLE direct_message_gone RENAME TO direct_message")
	if status, _ := do(t, http.MethodPost, fmt.Sprintf("%s/spaces/%d/continue", env.url, id), ""); status != 204 {
		t.Fatalf("continue = %d, want 204", status)
	}
	sub.want(t, `pause {"reason":""}`)
	sub.wantTurns(t, "architect")
	if got := env.ranTexts(); !slices.Equal(got, []string{
		"[Space 2, message 1]\ngo",
		"[Space 2, message 2]\nlead wrote: @architect plan it",
	}) {
		t.Errorf("ran = %q, want the architect to run the wake the failed direct read left", got)
	}
}

func TestSend_StoreErrorBeforeNextWakePausesAndContinueRunsHeldWakes(t *testing.T) {
	ctx := context.Background()
	env := newLiveEnv(t)
	id, post := env.teamSpace(t)
	env.fake.onRun = func(role, _ string, _ *int64) {
		if role == "lead" {
			post("lead", "@architect plan it", nil)
			env.exec(t, "ALTER TABLE turn RENAME TO turn_gone")
		}
	}
	sub := subscribeTo(t, env.url, id)

	env.sendTo(t, id, "go")
	const reason = "sum Space tokens: SQL logic error: no such table: turn (1)"
	sub.want(t, _idleFrame, `turn {"role":"lead","status":"running"}`, `agent {"kind":"text","text":"hi"}`,
		`turn {"role":"lead","status":"done"}`, `pause {"reason":"`+reason+`"}`)
	if ch, err := env.st.Space(ctx, int64(id)); err != nil || ch.Paused != reason {
		t.Errorf("space = %+v, %v, want paused with %q", ch, err, reason)
	}

	env.exec(t, "ALTER TABLE turn_gone RENAME TO turn")
	if status, _ := do(t, http.MethodPost, fmt.Sprintf("%s/spaces/%d/continue", env.url, id), ""); status != 204 {
		t.Fatalf("continue = %d, want 204", status)
	}
	sub.want(t, `pause {"reason":""}`)
	sub.wantTurns(t, "architect")
	if got := env.ranTexts(); !slices.Equal(got, []string{
		"[Space 2, message 1]\ngo",
		"[Space 2, message 2]\nlead wrote: @architect plan it",
	}) {
		t.Errorf("ran = %q, want the architect to run the wake held by the store error", got)
	}
	env.waitIdle(t, id)
	env.wantMessages(t, id, "user: go", "lead: @architect plan it")
}

func TestSend_DirectReadFailureKeepsDirectRequestForContinue(t *testing.T) {
	ctx := context.Background()
	env := newLiveEnv(t)
	id, _ := env.teamSpace(t)
	var asked atomic.Bool
	env.fake.onRun = func(role, _ string, _ *int64) {
		if role == "lead" && asked.CompareAndSwap(false, true) {
			_, err := env.st.CreateDirectMessage(ctx, store.DirectMessage{
				SpaceID: int64(id), From: "lead", To: "architect", Body: "check the schema",
			})
			if err != nil {
				t.Error(err)
			}
			env.exec(t, "ALTER TABLE direct_message RENAME TO direct_message_gone")
		}
	}
	sub := subscribeTo(t, env.url, id)

	env.sendTo(t, id, "go")
	const reason = "route direct messages: list direct messages: SQL logic error: no such table: direct_message (1)"
	sub.want(t, _idleFrame, `turn {"role":"lead","status":"running"}`, `agent {"kind":"text","text":"hi"}`,
		`turn {"role":"lead","status":"done"}`, `pause {"reason":"`+reason+`"}`)

	env.exec(t, "ALTER TABLE direct_message_gone RENAME TO direct_message")
	if status, _ := do(t, http.MethodPost, fmt.Sprintf("%s/spaces/%d/continue", env.url, id), ""); status != 204 {
		t.Fatalf("continue = %d, want 204", status)
	}
	sub.want(t, `pause {"reason":""}`,
		`turn {"role":"architect","status":"running","directMessageId":1}`, `agent {"kind":"text","text":"hi"}`,
		`turn {"role":"architect","status":"done","directMessageId":1}`,
		`turn {"role":"lead","status":"running","directMessageId":2}`, `agent {"kind":"text","text":"hi"}`,
		`turn {"role":"lead","status":"done","directMessageId":2}`)
	want := []string{
		"[Space 2, message 1]\ngo",
		"[Space 2, direct message 1, chain 1, source: direct request]\nlead wrote privately: check the schema",
		"[Space 2, direct message 2, chain 1, source: direct response]\narchitect wrote privately: hi",
	}
	if got := env.ranTexts(); !slices.Equal(got, want) {
		t.Errorf("ran = %q, want the architect to run the direct request the failed read left", got)
	}
}
