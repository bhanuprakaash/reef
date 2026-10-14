package server

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
)

const _hopPause = "hop cap: 4 agent hops without a user message"

// pingPong makes the lead and the architect answer each other until the hop cap pauses the
// channel, and returns the channel's id and a stream of it, after the pause.
func (e *liveEnv) pingPong(t *testing.T) (int, *stream) {
	t.Helper()
	id, post := e.teamChannel(t)
	e.fake.onRun = func(role, _ string, _ *int64) {
		if role == "lead" {
			post("lead", "@architect again", nil)
		} else {
			post("architect", "@lead again", nil)
		}
	}
	sub := subscribeTo(t, e.url, id)
	e.sendTo(t, id, "go")
	sub.want(t, _idleFrame)
	sub.wantTurns(t, "lead", "architect", "lead", "architect", "lead")
	sub.want(t, `pause {"reason":"`+_hopPause+`"}`)
	return id, sub
}

func TestSend_HopCapPausesChannel(t *testing.T) {
	env := newLiveEnv(t)
	id, sub := env.pingPong(t)
	if got := len(env.ranTexts()); got != 5 {
		t.Errorf("turns run = %d, want 5: the user's turn and 4 hops", got)
	}
	if ch, err := env.st.Channel(context.Background(), int64(id)); err != nil || ch.Paused != _hopPause {
		t.Errorf("channel = %+v, %v, want paused with the hop cap reason", ch, err)
	}

	if status, _ := do(t, http.MethodPost, fmt.Sprintf("%s/channels/%d/continue", env.url, id), ""); status != 204 {
		t.Fatalf("continue = %d, want 204", status)
	}
	sub.want(t, `pause {"reason":""}`)
	sub.wantTurns(t, "architect", "lead", "architect", "lead")
	sub.want(t, `pause {"reason":"`+_hopPause+`"}`)
	if got := len(env.ranTexts()); got != 9 {
		t.Errorf("turns run after continue = %d, want 9: the held wake and 3 more hops", got)
	}
}

func TestSend_UserMessageClearsHopPause(t *testing.T) {
	env := newLiveEnv(t)
	id, sub := env.pingPong(t)

	if status, body := env.sendTo(t, id, "more"); status != 202 || body != "" {
		t.Fatalf("send while hop paused = %d %q, want 202", status, body)
	}
	sub.want(t, `pause {"reason":""}`)
	sub.wantTurns(t, "lead", "architect", "lead", "architect", "lead")
	sub.want(t, `pause {"reason":"`+_hopPause+`"}`)
	if got := env.ranTexts(); len(got) != 10 || got[5] != "[channel 2, message 7]\nmore" {
		t.Errorf("ran = %q, want 10 turns, the sixth the user's: the held wake is dropped and the count starts at 0", got)
	}
}

func TestSend_UserMessageClearsHopPauseWithDifferentCount(t *testing.T) {
	env := newLiveEnv(t)
	sub := subscribe(t, env.url)
	sub.want(t, _idleFrame)
	if err := env.st.PauseChannel(context.Background(), 1,
		"hop cap: 9 agent hops without a user message"); err != nil {
		t.Fatal(err)
	}

	if status, body := env.send(t, "more"); status != 202 || body != "" {
		t.Fatalf("send while hop paused = %d %q, want 202", status, body)
	}
	sub.want(t, `pause {"reason":""}`)
	if ch, err := env.st.Channel(context.Background(), 1); err != nil || ch.Paused != "" {
		t.Errorf("channel = %+v, %v, want pause cleared", ch, err)
	}
}

func TestContinue_RunsHopHeldWakeAfterRestart(t *testing.T) {
	env := newLiveEnv(t)
	id, _ := env.pingPong(t)

	env.restart(t)
	sub := subscribeTo(t, env.url, id)
	sub.want(t, _idleFrame, `pause {"reason":"`+_hopPause+`"}`)
	if status, _ := do(t, http.MethodPost, fmt.Sprintf("%s/channels/%d/continue", env.url, id), ""); status != 204 {
		t.Fatalf("continue = %d, want 204", status)
	}
	sub.want(t, `pause {"reason":""}`)
	sub.wantTurns(t, "architect", "lead", "architect", "lead")
	sub.want(t, `pause {"reason":"`+_hopPause+`"}`)
	got := env.ranTexts()
	if len(got) != 9 || got[5] != "[channel 2, message 6]\nlead wrote: @architect again" {
		t.Errorf("ran = %q, want 9 turns, the sixth the wake held before the restart", got)
	}
}

func TestSend_UserMessageResetsHops(t *testing.T) {
	env := newLiveEnv(t)
	id, post := env.teamChannel(t)
	var runs atomic.Int32
	env.fake.onRun = func(role, _ string, _ *int64) {
		if runs.Add(1)%4 == 0 {
			return
		}
		if role == "lead" {
			post("lead", "@architect again", nil)
		} else {
			post("architect", "@lead again", nil)
		}
	}
	sub := subscribeTo(t, env.url, id)
	sub.want(t, _idleFrame)
	for range 2 {
		env.sendTo(t, id, "go")
		for _, role := range []string{"lead", "architect", "lead", "architect"} {
			sub.want(t, `turn {"role":"`+role+`","status":"running"}`, `agent {"kind":"text","text":"hi"}`,
				`turn {"role":"`+role+`","status":"done"}`)
		}
	}
	if ch, err := env.st.Channel(context.Background(), int64(id)); err != nil || ch.Paused != "" {
		t.Errorf("channel = %+v, %v, want not paused after 6 hops in 2 chains", ch, err)
	}
}

func TestCountHop(t *testing.T) {
	tests := []struct {
		name     string
		giveHops int
		giveFrom string
		want     bool
		wantHops int
	}{
		{"user wake resets", 3, "user", true, 0},
		{"user wake resets at the cap", 4, "user", true, 0},
		{"first agent wake", 0, "lead", true, 1},
		{"fourth agent wake", 3, "architect", true, 4},
		{"fifth agent wake is over", 4, "lead", false, 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := &live{hops: tt.giveHops}
			if got := l.countHop(wake{from: tt.giveFrom}); got != tt.want || l.hops != tt.wantHops {
				t.Errorf("countHop = %v with hops %d, want %v with hops %d",
					got, l.hops, tt.want, tt.wantHops)
			}
		})
	}
}
