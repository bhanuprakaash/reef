package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
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

func TestContinue_WhileRunning(t *testing.T) {
	env := newLiveEnv(t)
	env.send(t, "block")
	<-env.fake.started
	if err := env.st.PauseChannel(context.Background(), 1, "capped"); err != nil {
		t.Fatal(err)
	}

	status, body := do(t, http.MethodPost, env.url+"/channels/1/continue", "")
	if status != 409 || body != `{"error":"channel 1 has a turn running: conflict"}` {
		t.Errorf("continue while running = %d %s, want 409 turn running", status, body)
	}
	if ch, err := env.st.Channel(context.Background(), 1); err != nil || ch.Paused != "capped" {
		t.Errorf("channel = %+v, %v, want still paused", ch, err)
	}
	env.fake.release <- struct{}{}
}

func TestContinue_StartFailureKeepsHeldWakes(t *testing.T) {
	env := newLiveEnv(t)
	id, sub := env.pingPong(t)
	continueURL := fmt.Sprintf("%s/channels/%d/continue", env.url, id)
	do(t, http.MethodPost, fmt.Sprintf("%s/channels/%d/stop", env.url, id), "")
	env.open = func(context.Context) (session, error) { return nil, errors.New("boom") }

	if status, body := do(t, http.MethodPost, continueURL, ""); status != 500 {
		t.Fatalf("continue = %d %s, want 500: the agent did not open", status, body)
	}
	if ch, err := env.st.Channel(context.Background(), int64(id)); err != nil || ch.Paused != _hopPause {
		t.Fatalf("channel = %+v, %v, want still paused with the hop cap reason", ch, err)
	}

	env.open = nil
	if status, _ := do(t, http.MethodPost, continueURL, ""); status != 204 {
		t.Fatalf("second continue = %d, want 204", status)
	}
	sub.want(t, `pause {"reason":""}`)
	sub.wantTurns(t, "architect", "lead", "architect", "lead", "architect", "lead")
	sub.want(t, `pause {"reason":"`+_hopPause+`"}`)
	if got := len(env.ranTexts()); got != 13 {
		t.Errorf("turns run = %d, want 13: the wakes held before the failed continue ran", got)
	}
}

func TestContinue_RunsBudgetHeldWakeAfterRestart(t *testing.T) {
	ctx := context.Background()
	env := newLiveEnv(t)
	id, post := env.teamChannel(t)
	env.fake.onRun = func(role, _ string, _ *int64) {
		if role != "lead" {
			return
		}
		post("lead", "@architect plan", nil)
		turn, err := env.st.CreateTurn(ctx, store.Turn{ChannelID: int64(id), Role: "lead", Backend: "claude"})
		if err != nil {
			t.Error(err)
			return
		}
		turn.Status = "done"
		turn.Usage = agent.Usage{InputTokens: 1000000}
		if _, err := env.st.EndTurn(ctx, turn); err != nil {
			t.Error(err)
		}
	}
	sub := subscribeTo(t, env.url, id)
	env.sendTo(t, id, "go")
	const reason = "token budget of 1000000 used: 1000000 tokens"
	sub.want(t, _idleFrame)
	sub.wantTurns(t, "lead")
	sub.want(t, `pause {"reason":"`+reason+`"}`)

	env.restart(t)
	sub = subscribeTo(t, env.url, id)
	sub.want(t, _idleFrame, `pause {"reason":"`+reason+`"}`)
	if status, _ := do(t, http.MethodPost, fmt.Sprintf("%s/channels/%d/continue", env.url, id), ""); status != 204 {
		t.Fatalf("continue = %d, want 204", status)
	}
	sub.want(t, `pause {"reason":""}`)
	sub.wantTurns(t, "architect")
	if got := env.ranTexts(); !slices.Equal(got, []string{
		"[channel 2, message 1]\ngo",
		"[channel 2, message 2]\nlead wrote: @architect plan",
	}) {
		t.Errorf("ran = %q, want the architect to run the wake held before the restart", got)
	}
	if ch, err := env.st.Channel(ctx, int64(id)); err != nil || ch.Paused != "" || ch.TokenBudget != 2000000 {
		t.Errorf("channel = %+v, %v, want not paused with budget 2000000", ch, err)
	}
}

func TestContinue_TwoAtOnceRunHeldWakeOnce(t *testing.T) {
	env := newLiveEnv(t)
	id, _ := env.pingPong(t)
	env.restart(t)
	opening, proceed := make(chan struct{}), make(chan struct{})
	env.open = func(context.Context) (session, error) {
		opening <- struct{}{}
		<-proceed
		return env.fake, nil
	}
	sub := subscribeTo(t, env.url, id)
	sub.want(t, _idleFrame, `pause {"reason":"`+_hopPause+`"}`)
	continueURL := fmt.Sprintf("%s/channels/%d/continue", env.url, id)

	first := make(chan int)
	go func() {
		status, _ := do(t, http.MethodPost, continueURL, "")
		first <- status
	}()
	select {
	case <-opening:
	case <-time.After(5 * time.Second):
		t.Fatal("the first continue opened no agent after 5s")
	}
	if status, body := do(t, http.MethodPost, continueURL, ""); status != 409 ||
		body != `{"error":"channel 2 has a turn running: conflict"}` {
		t.Errorf("second continue = %d %s, want 409 turn running", status, body)
	}
	close(proceed)
	if status := <-first; status != 204 {
		t.Fatalf("first continue = %d, want 204", status)
	}
	sub.want(t, `pause {"reason":""}`)
	sub.wantTurns(t, "architect", "lead", "architect", "lead", "architect", "lead")
	sub.want(t, `pause {"reason":"`+_hopPause+`"}`)
	if got := len(env.ranTexts()); got != 13 {
		t.Errorf("turns run = %d, want 13: the held wake ran once", got)
	}
}

func TestContinue_UsedFarOverBudget(t *testing.T) {
	ctx := context.Background()
	env := newLiveEnv(t)
	turn, err := env.st.CreateTurn(ctx, store.Turn{ChannelID: 1, Role: "lead", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	turn.Status = "done"
	turn.Usage = agent.Usage{InputTokens: 5000000}
	if _, err := env.st.EndTurn(ctx, turn); err != nil {
		t.Fatal(err)
	}
	if status, _ := env.send(t, "hello"); status != 409 {
		t.Fatalf("send over budget = %d, want 409", status)
	}

	if status, _ := do(t, http.MethodPost, env.url+"/channels/1/continue", ""); status != 204 {
		t.Fatalf("continue = %d, want 204", status)
	}
	if ch, err := env.st.Channel(ctx, 1); err != nil || ch.Paused != "" || ch.TokenBudget != 6000000 {
		t.Errorf("channel after continue = %+v, %v, want not paused with budget 6000000", ch, err)
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
	env.h.s.mu.Lock()
	marks := len(env.h.s.deleting)
	env.h.s.mu.Unlock()
	if marks != 0 {
		t.Errorf("deleting marks = %d, want 0 after delete", marks)
	}
}

func TestStop_AfterPrepareTheTurnStillRuns(t *testing.T) {
	tests := []struct {
		name      string
		inPrepare bool
	}{
		{name: "in prepare", inPrepare: true},
		{name: "in say", inPrepare: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newLiveEnv(t)
			stopWhile, stopped, ag := stopAfterOpen(env)
			if !tt.inPrepare {
				env.fake.say = stopWhile
			}
			prepare := func() error {
				_, err := env.st.PromoteChannel(
					context.Background(), 1, store.Feature{Slug: "login", Name: "Login"})
				if tt.inPrepare {
					stopWhile()
				}
				return err
			}

			err := env.h.s.startUserTurn(context.Background(), 1,
				userTurn{kind: _promote, text: channel.PromoteCommand, prepare: prepare})
			if err != nil {
				t.Fatalf("startUserTurn = %v, want nil", err)
			}
			<-stopped
			env.wantInterruptedPromote(t, ag)
			if status, _ := env.send(t, "again"); status != 202 {
				t.Errorf("send after the stop = %d, want 202", status)
			}
		})
	}
}

func TestStop_DuringTurnCheck(t *testing.T) {
	env := newLiveEnv(t)
	ctx := &gateCtx{Context: context.Background()}
	ctx.hook = func() {
		l := env.h.s.lookup(1)
		if l == nil {
			t.Error("no live entry while the turn is checked, so a stop finds nothing")
			return
		}
		l.mu.Lock()
		cancel := l.cancel
		l.mu.Unlock()
		cancel()
	}

	err := env.h.s.startUserTurn(ctx, 1, userTurn{kind: _message, text: "hello"})
	if !errors.Is(err, store.ErrConflict) || !strings.Contains(err.Error(), "was stopped") {
		t.Fatalf("startUserTurn = %v, want stopped conflict", err)
	}
	if got := env.opened.Load(); got != 0 {
		t.Errorf("sessions opened = %d, want 0", got)
	}
	_, body := do(t, http.MethodGet, env.url+"/messages?channel=1", "")
	if body != "[]" {
		t.Errorf("messages = %s, want none", body)
	}
	env.h.s.mu.Lock()
	entries := len(env.h.s.lives)
	env.h.s.mu.Unlock()
	if entries != 0 {
		t.Errorf("entries = %d, want 0 after a send that never ran", entries)
	}
}

func TestDelete_FeatureOrProjectStopsLiveChannel(t *testing.T) {
	tests := []struct {
		name string
		give string
	}{
		{name: "feature", give: "/features/1"},
		{name: "project", give: "/projects/1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newLiveEnv(t)
			id := env.featureChannel(t)
			sub := subscribeTo(t, env.url, id)
			env.sendTo(t, id, "block")
			<-env.fake.started
			sub.want(t, _idleFrame, `turn {"role":"lead","status":"running"}`, `agent {"kind":"text","text":"part1"}`)

			if status, _ := do(t, http.MethodDelete, env.url+tt.give, ""); status != 204 {
				t.Fatalf("delete = %d, want 204", status)
			}
			sub.want(t, `turn {"role":"lead","status":"failed","error":"interrupted"}`)
			if got := env.fake.closed.Load(); got != 1 {
				t.Errorf("session closes = %d, want 1", got)
			}
			env.h.s.mu.Lock()
			_, ok := env.h.s.lives[int64(id)]
			env.h.s.mu.Unlock()
			if ok {
				t.Error("live entry kept after delete")
			}
		})
	}
}

func TestDelete_DropsHeldWakes(t *testing.T) {
	env := newLiveEnv(t)
	id, _ := env.pingPong(t)

	if status, _ := do(t, http.MethodDelete, fmt.Sprintf("%s/channels/%d", env.url, id), ""); status != 204 {
		t.Fatalf("delete = %d, want 204", status)
	}
	if l := env.h.s.lookup(int64(id)); l != nil {
		t.Errorf("live entry kept after delete, with %d held wakes", len(l.held))
	}
	if got := env.fake.closed.Load(); got != 1 {
		t.Errorf("session closes = %d, want 1", got)
	}
}
