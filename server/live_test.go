package server

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/channel"
	"github.com/bhanuprakaash/reef/store"
)

// fakeSession answers the text "block" with one text event and then waits for
// release or for its context to end, the text "cap" like a runner that capped the
// turn, and every other text with one text event.
type fakeSession struct {
	st      *store.Store
	started chan struct{}
	release chan struct{}
	closed  atomic.Int32
}

func (f *fakeSession) SendAs(
	ctx context.Context, _, text string, onEvent func(agent.Event),
) error {
	if text == "cap" {
		const reason = "capped: more than 25 tool calls"
		err := fmt.Errorf("%w: more than 25 tool calls", channel.ErrCapped)
		return errors.Join(err, f.st.PauseChannel(ctx, 1, reason))
	}
	if text != "block" {
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
	onEvent(agent.Event{Kind: agent.KindText, Text: "wrapped"})
	return store.Proposal{}, nil
}

func (f *fakeSession) Promote(_ context.Context, onEvent func(agent.Event)) (store.Proposal, error) {
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
	env.h = newHandler(st, func(context.Context, int64) (session, error) {
		env.opened.Add(1)
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
	return do(t, http.MethodPost, e.url+"/channels/1/send", `{"text":"`+text+`"}`)
}

// stream is a subscriber to a channel's events. It yields "event data" lines.
type stream struct {
	frames chan string
}

func subscribe(t *testing.T, url string) *stream {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url+"/channels/1/events", nil)
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
		`turn {"status":"running"}`,
		`agent {"kind":"text","text":"hi"}`,
		`turn {"status":"done"}`,
	)

	late := subscribe(t, env.url)
	if status, _ := env.send(t, "hello"); status != 202 {
		t.Fatalf("second send = %d, want 202", status)
	}
	late.want(t,
		`turn {"status":"running"}`,
		`agent {"kind":"text","text":"hi"}`,
		`turn {"status":"done"}`,
	)
	if got := env.opened.Load(); got != 1 {
		t.Errorf("sessions opened = %d, want 1: the agent stays alive between turns", got)
	}
}

func TestSend_Wrap(t *testing.T) {
	env := newLiveEnv(t)
	sub := subscribe(t, env.url)
	if status, _ := env.send(t, "/wrap"); status != 202 {
		t.Fatalf("send = %d, want 202", status)
	}
	sub.want(t,
		`turn {"status":"running"}`,
		`agent {"kind":"text","text":"wrapped"}`,
		`turn {"status":"done"}`,
	)
}

func TestSubscribe_ReplaysRunningTurn(t *testing.T) {
	env := newLiveEnv(t)
	first := subscribe(t, env.url)
	env.send(t, "block")
	<-env.fake.started
	first.want(t, `turn {"status":"running"}`, `agent {"kind":"text","text":"part1"}`)

	second := subscribe(t, env.url)
	second.want(t, `turn {"status":"running"}`, `agent {"kind":"text","text":"part1"}`)

	env.fake.release <- struct{}{}
	for _, s := range []*stream{first, second} {
		s.want(t, `agent {"kind":"text","text":"part2"}`, `turn {"status":"done"}`)
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
	sub.want(t, `turn {"status":"running"}`, `agent {"kind":"text","text":"part1"}`)

	if status, body := do(t, http.MethodPost, env.url+"/channels/1/stop", ""); status != 204 || body != "" {
		t.Fatalf("stop = %d %q, want 204", status, body)
	}
	sub.want(t, `turn {"status":"failed","error":"interrupted"}`)
	if got := env.fake.closed.Load(); got != 1 {
		t.Errorf("session closes = %d, want 1", got)
	}

	if status, _ := env.send(t, "hello"); status != 202 {
		t.Fatalf("send after stop = %d, want 202", status)
	}
	sub.want(t, `turn {"status":"running"}`, `agent {"kind":"text","text":"hi"}`, `turn {"status":"done"}`)
	if got := env.opened.Load(); got != 2 {
		t.Errorf("sessions opened = %d, want 2: a send after stop opens the agent again", got)
	}
}

func TestSend_ToolCapPausesChannel(t *testing.T) {
	env := newLiveEnv(t)
	sub := subscribe(t, env.url)
	env.send(t, "cap")
	sub.want(t,
		`turn {"status":"running"}`,
		`turn {"status":"failed","error":"capped: more than 25 tool calls"}`,
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
	sub.want(t, `turn {"status":"running"}`, `agent {"kind":"text","text":"hi"}`, `turn {"status":"done"}`)

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
	sub.want(t, `pause {"reason":"`+reason+`"}`)
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
	sub.want(t, `turn {"status":"running"}`, `agent {"kind":"text","text":"part1"}`)

	env.h.Close()
	sub.want(t, `turn {"status":"failed","error":"interrupted"}`)
	if got := env.fake.closed.Load(); got != 1 {
		t.Errorf("session closes = %d, want 1", got)
	}
	if status, body := env.send(t, "hello"); status != 503 || body != `{"error":"server is stopping"}` {
		t.Errorf("send after close = %d %s, want 503", status, body)
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
		{"channel without backend", http.MethodPost, "/channels", `{"projectId":1,"name":"c"}`, 400,
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
