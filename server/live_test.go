package server

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/channel"
	"github.com/bhanuprakaash/reef/store"
)

type fakeSession struct {
	st         *store.Store
	started    chan struct{}
	release    chan struct{}
	closed     atomic.Int32
	channelID  atomic.Int64
	promoted   atomic.Int32
	noProposal atomic.Bool

	// onRun, when set, runs inside every message turn: it stands in for the messages an
	// agent stores while it works.
	onRun func(role, text string, parent *int64)

	mu  sync.Mutex
	ran []string
}

func (f *fakeSession) Say(ctx context.Context, body string, parent *int64) error {
	_, err := f.st.CreateMessage(ctx, store.Message{
		ChannelID: f.channelID.Load(), ParentID: parent, Author: "user", Body: body,
	})
	return err
}

func (f *fakeSession) RunAs(
	ctx context.Context, role, text string, parent *int64, onEvent func(agent.Event),
) error {
	f.mu.Lock()
	f.ran = append(f.ran, text)
	f.mu.Unlock()
	if f.onRun != nil {
		f.onRun(role, text, parent)
	}
	switch text {
	case "cap":
		const reason = "capped: more than 25 tool calls"
		err := fmt.Errorf("%w: more than 25 tool calls", channel.ErrCapped)
		return errors.Join(err, f.st.PauseChannel(ctx, f.channelID.Load(), reason))
	case "cap-lost":
		err := fmt.Errorf("%w: more than 25 tool calls", channel.ErrCapped)
		return errors.Join(err, f.st.DeleteChannel(ctx, f.channelID.Load()))
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

func (f *fakeSession) Wrap(_ context.Context, onEvent func(agent.Event)) (store.Proposal, error) {
	if f.onRun != nil {
		f.onRun("lead", channel.WrapCommand, nil)
	}
	onEvent(agent.Event{Kind: agent.KindText, Text: "wrapped"})
	if f.noProposal.Load() {
		return store.Proposal{}, channel.ErrNoProposal
	}
	return store.Proposal{}, nil
}

func (f *fakeSession) Promote(_ context.Context, onEvent func(agent.Event)) (store.Proposal, error) {
	f.promoted.Add(1)
	onEvent(agent.Event{Kind: agent.KindText, Text: "promoted"})
	return store.Proposal{}, nil
}

func (f *fakeSession) Close() error {
	f.closed.Add(1)
	return nil
}

type liveEnv struct {
	st     *store.Store
	url    string
	h      *Handler
	fake   *fakeSession
	opened atomic.Int32
	open   func(ctx context.Context) (session, error)
}

func newLiveEnv(t *testing.T) *liveEnv {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	env := &liveEnv{fake: &fakeSession{started: make(chan struct{}, 1), release: make(chan struct{})}}
	env.st, env.fake.st = st, st
	env.h = newHandler(st, func(ctx context.Context, id int64) (session, error) {
		env.opened.Add(1)
		env.fake.channelID.Store(id)
		if env.open != nil {
			return env.open(ctx)
		}
		return env.fake, nil
	})
	ts := httptest.NewServer(env.h)
	env.url = ts.URL
	t.Cleanup(env.h.Close)
	t.Cleanup(ts.Close)
	do(t, http.MethodPost, ts.URL+"/projects", `{"name":"reef","path":"`+t.TempDir()+`"}`)
	do(t, http.MethodPost, ts.URL+"/channels", `{"projectId":1,"name":"scratch","backend":"claude"}`)
	return env
}

func (e *liveEnv) send(t *testing.T, text string) (int, string) {
	t.Helper()
	return e.sendTo(t, 1, text)
}

func (e *liveEnv) sendTo(t *testing.T, id int, text string) (int, string) {
	t.Helper()
	return do(t, http.MethodPost, fmt.Sprintf("%s/channels/%d/send", e.url, id), `{"text":"`+text+`"}`)
}

// featureChannel adds a feature and a channel on it, and returns the channel's id.
func (e *liveEnv) featureChannel(t *testing.T) int {
	t.Helper()
	do(t, http.MethodPost, e.url+"/features", `{"projectId":1,"slug":"login","name":"Login"}`)
	do(t, http.MethodPost, e.url+"/channels", `{"projectId":1,"featureId":1,"name":"c","backend":"claude"}`)
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
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/channels/%d/events", url, id), nil)
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

func TestSend_Parent(t *testing.T) {
	env := newLiveEnv(t)
	sub := subscribe(t, env.url)
	sub.want(t, _idleFrame)
	send := func(body string) int {
		status, _ := do(t, http.MethodPost, env.url+"/channels/1/send", body)
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
	_, body := do(t, http.MethodGet, env.url+"/messages?channel=1", "")
	if !strings.Contains(body, `"parentId":null`) || !strings.Contains(body, `"parentId":1`) ||
		strings.Contains(body, "deeper") {
		t.Errorf("messages = %s, want root with null parentId and one reply with 1", body)
	}
}

// teamChannel adds the team core, a lead and an architect, and a channel of it. It returns
// the channel's id and a function that stores a message as author.
func (e *liveEnv) teamChannel(t *testing.T) (int, func(author, body string, parent *int64)) {
	t.Helper()
	_, err := e.st.ApplyTeam(context.Background(), store.Team{Name: "core", Roles: []store.Role{
		{Name: "lead", Lead: true, Backend: "claude"},
		{Name: "architect", Backend: "claude"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	do(t, http.MethodPost, e.url+"/channels", `{"projectId":1,"name":"t","team":"core"}`)
	return 2, func(author, body string, parent *int64) {
		t.Helper()
		_, err := e.st.CreateMessage(context.Background(),
			store.Message{ChannelID: 2, ParentID: parent, Author: author, Body: body})
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
	id, post := env.teamChannel(t)
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

	if status, _ := do(t, http.MethodPost, fmt.Sprintf("%s/channels/%d/send", env.url, id),
		`{"text":"why?","parent":3}`); status != 202 {
		t.Fatalf("reply in the thread = %d, want 202", status)
	}
	sub.want(t, `turn {"role":"architect","status":"running"}`, `agent {"kind":"text","text":"hi"}`,
		`turn {"role":"architect","status":"done"}`)

	want := []string{
		"login please",
		"lead wrote: @architect plan it",
		"Thread so far:\narchitect: the plan\n\nwhy?",
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

func TestSend_WakesByPost(t *testing.T) {
	env := newLiveEnv(t)
	id, post := env.teamChannel(t)
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
	want := []string{"go", "lead wrote: @architect look at this"}
	if got := env.ranTexts(); !slices.Equal(got, want) {
		t.Errorf("ran = %q, want %q: the post that mentions nobody wakes no one", got, want)
	}
}

func TestSend_WhileWakeChainRuns(t *testing.T) {
	env := newLiveEnv(t)
	id, post := env.teamChannel(t)
	env.fake.onRun = func(role, _ string, _ *int64) {
		if role == "lead" {
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

	status, body := env.sendTo(t, id, "again")
	if status != 409 || body != `{"error":"channel 2 has a turn running: conflict"}` {
		t.Errorf("send during the wake = %d %s, want 409 turn running", status, body)
	}
	env.fake.release <- struct{}{}
	sub.want(t, `agent {"kind":"text","text":"part2"}`, `turn {"role":"architect","status":"done"}`)
}

func TestSend_Wrap(t *testing.T) {
	env := newLiveEnv(t)
	id := env.featureChannel(t)
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
	env.send(t, "block")
	<-env.fake.started

	status, body := env.send(t, "again")
	if status != 409 || body != `{"error":"channel 1 has a turn running: conflict"}` {
		t.Errorf("send = %d %s, want 409 turn running", status, body)
	}
	env.fake.release <- struct{}{}
}

func TestStop_FailsTurnAndClosesSession(t *testing.T) {
	env := newLiveEnv(t)
	sub := subscribe(t, env.url)
	env.send(t, "block")
	<-env.fake.started
	sub.want(t, _idleFrame, `turn {"role":"lead","status":"running"}`, `agent {"kind":"text","text":"part1"}`)

	if status, body := do(t, http.MethodPost, env.url+"/channels/1/stop", ""); status != 204 || body != "" {
		t.Fatalf("stop = %d %q, want 204", status, body)
	}
	sub.want(t, `turn {"role":"lead","status":"failed","error":"interrupted"}`)
	if got := env.fake.closed.Load(); got != 1 {
		t.Errorf("session closes = %d, want 1", got)
	}

	if status, _ := env.send(t, "hello"); status != 202 {
		t.Fatalf("send after stop = %d, want 202", status)
	}
	sub.want(t, `turn {"role":"lead","status":"running"}`, `agent {"kind":"text","text":"hi"}`, `turn {"role":"lead","status":"done"}`)
	if got := env.opened.Load(); got != 2 {
		t.Errorf("sessions opened = %d, want 2: a send after stop opens the agent again", got)
	}
}

func TestSend_WrapWakesNobody(t *testing.T) {
	env := newLiveEnv(t)
	env.teamChannel(t)
	do(t, http.MethodPost, env.url+"/features", `{"projectId":1,"slug":"login","name":"Login"}`)
	do(t, http.MethodPost, env.url+"/channels", `{"projectId":1,"featureId":1,"name":"w","team":"core"}`)
	env.fake.onRun = func(string, string, *int64) {
		_, err := env.st.CreateMessage(context.Background(),
			store.Message{ChannelID: 3, Author: "lead", Body: "@architect look at this"})
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
	if got := env.ranTexts(); !slices.Equal(got, []string{"ping"}) {
		t.Errorf("ran = %q, want only ping: the wrap reply wakes no architect", got)
	}
}

func TestSend_ToolCapPausesChannel(t *testing.T) {
	env := newLiveEnv(t)
	sub := subscribe(t, env.url)
	env.send(t, "cap")
	sub.want(t,
		_idleFrame,
		`turn {"role":"lead","status":"running"}`,
		`turn {"role":"lead","status":"failed","error":"capped: more than 25 tool calls"}`,
		`pause {"reason":"capped: more than 25 tool calls"}`,
	)

	status, body := env.send(t, "hello")
	if status != 409 ||
		body != `{"error":"channel 1 is paused: capped: more than 25 tool calls: conflict"}` {
		t.Errorf("send to paused channel = %d %s, want 409 with the reason", status, body)
	}
	_, body = do(t, http.MethodGet, env.url+"/channels/1", "")
	if !strings.Contains(body, `"paused":"capped: more than 25 tool calls"`) {
		t.Errorf("channel = %s, want paused with the reason", body)
	}

	if status, body := do(t, http.MethodPost, env.url+"/channels/1/continue", ""); status != 204 || body != "" {
		t.Fatalf("continue = %d %q, want 204", status, body)
	}
	sub.want(t, `pause {"reason":""}`)
	if status, _ := env.send(t, "hello"); status != 202 {
		t.Errorf("send after continue = %d, want 202", status)
	}
	sub.want(t, `turn {"role":"lead","status":"running"}`, `agent {"kind":"text","text":"hi"}`, `turn {"role":"lead","status":"done"}`)

	status, body = do(t, http.MethodPost, env.url+"/channels/1/continue", "")
	if status != 409 || body != `{"error":"channel 1 is not paused: conflict"}` {
		t.Errorf("continue when not paused = %d %s, want 409", status, body)
	}
}

func TestSend_TokenBudgetPausesChannel(t *testing.T) {
	ctx := context.Background()
	env := newLiveEnv(t)
	turn, err := env.st.CreateTurn(ctx, store.Turn{ChannelID: 1, Role: "lead", Backend: "claude"})
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
	if status != 409 || body != `{"error":"channel 1 is paused: `+reason+`: conflict"}` {
		t.Fatalf("send over budget = %d %s, want 409 with the reason", status, body)
	}
	sub.want(t, _idleFrame, `pause {"reason":"`+reason+`"}`)
	if ch, err := env.st.Channel(ctx, 1); err != nil || ch.Paused != reason {
		t.Errorf("channel = %+v, %v, want paused with the reason", ch, err)
	}

	if status, _ := do(t, http.MethodPost, env.url+"/channels/1/continue", ""); status != 204 {
		t.Fatalf("continue = %d, want 204", status)
	}
	if ch, err := env.st.Channel(ctx, 1); err != nil || ch.Paused != "" || ch.TokenBudget != 2000000 {
		t.Errorf("channel after continue = %+v, %v, want not paused with budget 2000000", ch, err)
	}
	if status, _ := env.send(t, "hello"); status != 202 {
		t.Errorf("send after continue = %d, want 202", status)
	}
}

func TestStop_NotLive(t *testing.T) {
	env := newLiveEnv(t)
	if status, body := do(t, http.MethodPost, env.url+"/channels/1/stop", ""); status != 204 || body != "" {
		t.Errorf("stop = %d %q, want 204", status, body)
	}
	if got := env.fake.closed.Load(); got != 0 {
		t.Errorf("session closes = %d, want 0", got)
	}
}

func TestClose_FailsRunningTurn(t *testing.T) {
	env := newLiveEnv(t)
	sub := subscribe(t, env.url)
	env.send(t, "block")
	<-env.fake.started
	sub.want(t, _idleFrame, `turn {"role":"lead","status":"running"}`, `agent {"kind":"text","text":"part1"}`)

	env.h.Close()
	sub.want(t, `turn {"role":"lead","status":"failed","error":"interrupted"}`)
	if got := env.fake.closed.Load(); got != 1 {
		t.Errorf("session closes = %d, want 1", got)
	}
	if status, body := env.send(t, "hello"); status != 503 || body != `{"error":"server is stopping"}` {
		t.Errorf("send after close = %d %s, want 503", status, body)
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

func TestClose_ClosesSessionsInParallel(t *testing.T) {
	env := newLiveEnv(t)
	do(t, http.MethodPost, env.url+"/channels", `{"projectId":1,"name":"two","backend":"claude"}`)
	do(t, http.MethodPost, env.url+"/channels", `{"projectId":1,"name":"three","backend":"claude"}`)
	var closing sync.WaitGroup
	var met atomic.Int32
	closing.Add(3)
	env.open = func(context.Context) (session, error) {
		return &meetSession{fakeSession: env.fake, closing: &closing, met: &met}, nil
	}
	for id := 1; id <= 3; id++ {
		sub := subscribeTo(t, env.url, id)
		if status, body := env.sendTo(t, id, "hi"); status != 202 {
			t.Fatalf("send to %d = %d %s, want 202", id, status, body)
		}
		sub.want(t, _idleFrame, `turn {"role":"lead","status":"running"}`, `agent {"kind":"text","text":"hi"}`,
			`turn {"role":"lead","status":"done"}`)
	}

	done := make(chan struct{})
	go func() {
		env.h.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not return after 5s: sessions are closed one at a time")
	}
	if got := met.Load(); got != 3 {
		t.Errorf("sessions closed = %d, want 3", got)
	}
}

func TestSend_OpenErrors(t *testing.T) {
	tests := []struct {
		name       string
		giveErr    error
		wantStatus int
		wantBody   string
	}{
		{"unknown channel", store.ErrNotFound, 404, `{"error":"not found"}`},
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
			if _, err := st.CreateChannel(context.Background(), store.Channel{ProjectID: 1, Name: "chat"}); err != nil {
				t.Fatal(err)
			}
			h := newHandler(st, func(context.Context, int64) (session, error) { return nil, tt.giveErr })
			t.Cleanup(h.Close)
			ts := httptest.NewServer(h)
			t.Cleanup(ts.Close)
			status, body := do(t, http.MethodPost, ts.URL+"/channels/1/send", `{"text":"hi"}`)
			if status != tt.wantStatus || body != tt.wantBody {
				t.Errorf("send = %d %s, want %d %s", status, body, tt.wantStatus, tt.wantBody)
			}
			if status, _ = do(t, http.MethodPost, ts.URL+"/channels/1/stop", ""); status != 204 {
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
		{"no text", http.MethodPost, "/channels/1/send", `{"text":""}`, 400, `{"error":"invalid: text is required"}`},
		{"unknown field", http.MethodPost, "/channels/1/send", `{"body":"x"}`, 400,
			`{"error":"invalid: bad JSON: json: unknown field \"body\""}`},
		{"events of unknown channel", http.MethodGet, "/channels/9/events", "", 404,
			`{"error":"channel 9: not found"}`},
		{"send by GET", http.MethodGet, "/channels/1/send", "", 405, `{"error":"method not allowed"}`},
		{"events by POST", http.MethodPost, "/channels/1/events", "", 405, `{"error":"method not allowed"}`},
		{"stop by GET", http.MethodGet, "/channels/1/stop", "", 405, `{"error":"method not allowed"}`},
		{"unknown backend", http.MethodPost, "/channels", `{"projectId":1,"name":"c","backend":"gpt"}`, 400,
			`{"error":"invalid: backend must be claude or codex"}`},
		{"bad permission mode", http.MethodPost, "/channels",
			`{"projectId":1,"name":"c","backend":"codex","permissionMode":"acceptEdits"}`, 400,
			`{"error":"invalid: permissionMode for codex must be empty, read-only or workspace-write"}`},
		{"claude permission mode", http.MethodPost, "/channels",
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

func TestCreateChannel_RoleDefaultsToLead(t *testing.T) {
	env := newLiveEnv(t)
	status, body := do(t, http.MethodPost, env.url+"/channels",
		`{"projectId":1,"name":"c","backend":"codex","permissionMode":"workspace-write"}`)
	if status != 201 || !strings.Contains(body, `"role":"lead","backend":"codex","permissionMode":"workspace-write"`) {
		t.Errorf("create = %d %s, want role lead", status, body)
	}
}

func TestSend_PromoteTextIsAMessage(t *testing.T) {
	env := newLiveEnv(t)
	id := env.featureChannel(t)
	sub := subscribeTo(t, env.url, id)

	if status, _ := env.sendTo(t, id, "/promote"); status != 202 {
		t.Fatalf("send = %d, want 202", status)
	}
	sub.want(t,
		_idleFrame,
		`turn {"role":"lead","status":"running"}`,
		`agent {"kind":"text","text":"hi"}`,
		`turn {"role":"lead","status":"done"}`,
	)
	env.fake.mu.Lock()
	ran := env.fake.ran
	env.fake.mu.Unlock()
	if !slices.Equal(ran, []string{"/promote"}) || env.fake.promoted.Load() != 0 {
		t.Errorf("ran = %q, promotes = %d, want the text run as a message, no promote",
			ran, env.fake.promoted.Load())
	}
	_, body := do(t, http.MethodGet, fmt.Sprintf("%s/messages?channel=%d", env.url, id), "")
	if !strings.Contains(body, `"author":"user","body":"/promote"`) {
		t.Errorf("messages = %s, want the user message /promote", body)
	}
}

func TestSend_StoresUserMessageBefore202(t *testing.T) {
	env := newLiveEnv(t)
	if status, _ := env.send(t, "block"); status != 202 {
		t.Fatalf("send = %d, want 202", status)
	}
	_, body := do(t, http.MethodGet, env.url+"/messages?channel=1", "")
	if !strings.Contains(body, `"author":"user","body":"block"`) {
		t.Errorf("messages right after 202 = %s, want the user message block", body)
	}
	env.fake.release <- struct{}{}
}

func TestStop_WhileOpening(t *testing.T) {
	tests := []struct {
		name       string
		give       func(env *liveEnv) func(ctx context.Context) (session, error)
		wantCloses int32
	}{
		{
			name: "open returns the cancelled context's error",
			give: func(*liveEnv) func(context.Context) (session, error) {
				return func(ctx context.Context) (session, error) {
					<-ctx.Done()
					return nil, ctx.Err()
				}
			},
			wantCloses: 0,
		},
		{
			name: "open ignores the cancel and returns a session",
			give: func(env *liveEnv) func(context.Context) (session, error) {
				return func(ctx context.Context) (session, error) {
					<-ctx.Done()
					return env.fake, nil
				}
			},
			wantCloses: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newLiveEnv(t)
			opening := make(chan struct{})
			give := tt.give(env)
			env.open = func(ctx context.Context) (session, error) {
				close(opening)
				return give(ctx)
			}
			type reply struct {
				status int
				body   string
			}
			sent := make(chan reply, 1)
			go func() {
				status, body := env.send(t, "hello")
				sent <- reply{status, body}
			}()
			<-opening

			if status, _ := do(t, http.MethodPost, env.url+"/channels/1/stop", ""); status != 204 {
				t.Fatalf("stop = %d, want 204", status)
			}
			got := <-sent
			if got.status != 409 || got.body != `{"error":"channel 1 was stopped: conflict"}` {
				t.Errorf("send = %d %s, want 409 stopped", got.status, got.body)
			}
			env.fake.mu.Lock()
			ran := env.fake.ran
			env.fake.mu.Unlock()
			if len(ran) != 0 {
				t.Errorf("ran = %q, want no turn", ran)
			}
			_, body := do(t, http.MethodGet, env.url+"/messages?channel=1", "")
			if body != "[]" {
				t.Errorf("messages = %s, want none", body)
			}
			if got := env.fake.closed.Load(); got != tt.wantCloses {
				t.Errorf("session closes = %d, want %d", got, tt.wantCloses)
			}
		})
	}
}

func TestSubscribe_StateOnConnect(t *testing.T) {
	env := newLiveEnv(t)
	sub := subscribe(t, env.url)
	env.send(t, "cap")
	sub.want(t, _idleFrame, `turn {"role":"lead","status":"running"}`,
		`turn {"role":"lead","status":"failed","error":"capped: more than 25 tool calls"}`,
		`pause {"reason":"capped: more than 25 tool calls"}`)

	late := subscribe(t, env.url)
	late.want(t, _idleFrame, `pause {"reason":"capped: more than 25 tool calls"}`)

	do(t, http.MethodPost, env.url+"/channels/1/continue", "")
	again := subscribe(t, env.url)
	again.want(t, _idleFrame)
	if status, _ := env.send(t, "hello"); status != 202 {
		t.Fatalf("send after continue = %d, want 202", status)
	}
	again.want(t, `turn {"role":"lead","status":"running"}`)
}

func TestSend_CapKeepsReasonWhenChannelReadFails(t *testing.T) {
	env := newLiveEnv(t)
	sub := subscribe(t, env.url)
	env.send(t, "cap-lost")
	sub.want(t, _idleFrame, `turn {"role":"lead","status":"running"}`,
		`turn {"role":"lead","status":"failed","error":"capped: more than 25 tool calls"}`,
		`pause {"reason":"capped: more than 25 tool calls"}`)
}

func TestWrap_QuickSession(t *testing.T) {
	env := newLiveEnv(t)
	const want = `{"error":"invalid: channel 1 is a quick session, promote it before wrapping up"}`
	if status, body := do(t, http.MethodPost, env.url+"/channels/1/wrap", ""); status != 400 || body != want {
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
	id := env.featureChannel(t)
	sub := subscribeTo(t, env.url, id)
	env.sendTo(t, id, "/wrap")
	sub.want(t, _idleFrame, `turn {"role":"lead","status":"running"}`, `agent {"kind":"text","text":"wrapped"}`,
		`turn {"role":"lead","status":"done","note":"no proposal made"}`)
}

func TestDelete_StopsLiveChannel(t *testing.T) {
	env := newLiveEnv(t)
	sub := subscribe(t, env.url)
	env.send(t, "block")
	<-env.fake.started
	sub.want(t, _idleFrame, `turn {"role":"lead","status":"running"}`, `agent {"kind":"text","text":"part1"}`)

	if status, _ := do(t, http.MethodDelete, env.url+"/channels/1", ""); status != 204 {
		t.Fatalf("delete = %d, want 204", status)
	}
	if got := env.fake.closed.Load(); got != 1 {
		t.Errorf("session closes = %d, want 1", got)
	}
	env.h.s.mu.Lock()
	lives := len(env.h.s.lives)
	env.h.s.mu.Unlock()
	if lives != 0 {
		t.Errorf("live channels = %d, want 0 after delete", lives)
	}
}
