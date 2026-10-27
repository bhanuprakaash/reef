package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

const _hopPause = "hop cap: 6 agent hops without a user message"

// pingPong makes the lead and the architect answer each other until the hop cap pauses the
// Space, and returns the Space's id and a stream of it, after the pause.
func (e *liveEnv) pingPong(t *testing.T) (int, *stream) {
	t.Helper()
	id, post := e.teamSpace(t)
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
	sub.wantTurns(t, "lead", "architect", "lead", "architect", "lead", "architect", "lead")
	sub.want(t, `pause {"reason":"`+_hopPause+`"}`)
	return id, sub
}

func TestSend_HopCapPausesSpace(t *testing.T) {
	env := newLiveEnv(t)
	id, sub := env.pingPong(t)
	if got := len(env.ranTexts()); got != 7 {
		t.Errorf("turns run = %d, want 7: the user's turn and 6 hops", got)
	}
	if ch, err := env.st.Space(context.Background(), int64(id)); err != nil || ch.Paused != _hopPause {
		t.Errorf("space = %+v, %v, want paused with the hop cap reason", ch, err)
	}

	if status, _ := do(t, http.MethodPost, fmt.Sprintf("%s/spaces/%d/continue", env.url, id), ""); status != 204 {
		t.Fatalf("continue = %d, want 204", status)
	}
	sub.want(t, `pause {"reason":""}`)
	sub.wantTurns(t, "architect", "lead", "architect", "lead", "architect", "lead")
	sub.want(t, `pause {"reason":"`+_hopPause+`"}`)
	if got := len(env.ranTexts()); got != 13 {
		t.Errorf("turns run after continue = %d, want 13: the held wake and 5 more hops", got)
	}
}

// pingPongTools makes the lead and the architect answer each other, each calling its tool from
// tools at the start of a turn, until the hop cap pauses the Space or the 13th turn, which posts
// nothing, ends the chain. It returns the Space's id and how many turns ran.
func (e *liveEnv) pingPongTools(t *testing.T, tools map[string]string) (int, int) {
	t.Helper()
	id, post := e.teamSpace(t)
	e.fake.tools = tools
	var runs atomic.Int32
	e.fake.onRun = func(role, _ string, _ *int64) {
		if runs.Add(1) == 13 {
			return
		}
		if role == "lead" {
			post("lead", "@architect again", nil)
		} else {
			post("architect", "@lead again", nil)
		}
	}
	sub := subscribeTo(t, e.url, id)
	sub.want(t, _idleFrame)
	e.sendTo(t, id, "go")
	done := 0
	for done < 13 {
		frame := sub.next(t)
		if strings.HasPrefix(frame, "pause ") {
			break
		}
		if strings.HasPrefix(frame, "turn ") && strings.HasSuffix(frame, `"status":"done"}`) {
			done++
		}
	}
	return id, len(e.ranTexts())
}

func TestSend_ReefToolTurnsHitHopCap(t *testing.T) {
	env := newLiveEnv(t)
	id, ran := env.pingPongTools(t, map[string]string{
		"lead": "mcp__reef__post_to_space", "architect": "mcp__reef__remember"})
	if ch, err := env.st.Space(context.Background(), int64(id)); err != nil || ch.Paused != _hopPause {
		t.Errorf("space = %+v, %v, want paused with the hop cap reason", ch, err)
	}
	if ran != 7 {
		t.Errorf("turns run = %d, want 7: the user's turn and 6 hops", ran)
	}
}

func TestSend_WorkToolTurnsResetHops(t *testing.T) {
	env := newLiveEnv(t)
	id, ran := env.pingPongTools(t, map[string]string{"lead": "Edit", "architect": "Bash"})
	if ch, err := env.st.Space(context.Background(), int64(id)); err != nil || ch.Paused != "" {
		t.Errorf("space = %+v, %v, want not paused", ch, err)
	}
	if ran != 13 {
		t.Errorf("turns run = %d, want 13: the user's turn and 12 hops", ran)
	}
}

func TestSend_UserMessageClearsHopPause(t *testing.T) {
	env := newLiveEnv(t)
	id, sub := env.pingPong(t)

	if status, body := env.sendTo(t, id, "more"); status != 202 || body != "" {
		t.Fatalf("send while hop paused = %d %q, want 202", status, body)
	}
	sub.want(t, `pause {"reason":""}`)
	sub.wantTurns(t, "lead", "architect", "lead", "architect", "lead", "architect", "lead")
	sub.want(t, `pause {"reason":"`+_hopPause+`"}`)
	if got := env.ranTexts(); len(got) != 14 || got[7] != "[Space 2, message 9]\nmore" {
		t.Errorf("ran = %q, want 14 turns, the eighth the user's: the held wake is dropped and the count starts at 0", got)
	}
}

func TestSend_UserMessageClearsHopPauseWithDifferentCount(t *testing.T) {
	env := newLiveEnv(t)
	sub := subscribe(t, env.url)
	sub.want(t, _idleFrame)
	if err := env.st.PauseSpace(context.Background(), 1,
		"hop cap: 9 agent hops without a user message"); err != nil {
		t.Fatal(err)
	}

	if status, body := env.send(t, "more"); status != 202 || body != "" {
		t.Fatalf("send while hop paused = %d %q, want 202", status, body)
	}
	sub.want(t, `pause {"reason":""}`)
	if ch, err := env.st.Space(context.Background(), 1); err != nil || ch.Paused != "" {
		t.Errorf("space = %+v, %v, want pause cleared", ch, err)
	}
}

func TestContinue_RunsHopHeldWakeAfterRestart(t *testing.T) {
	env := newLiveEnv(t)
	id, _ := env.pingPong(t)

	env.restart(t)
	sub := subscribeTo(t, env.url, id)
	sub.want(t, _idleFrame, `pause {"reason":"`+_hopPause+`"}`)
	if status, _ := do(t, http.MethodPost, fmt.Sprintf("%s/spaces/%d/continue", env.url, id), ""); status != 204 {
		t.Fatalf("continue = %d, want 204", status)
	}
	sub.want(t, `pause {"reason":""}`)
	sub.wantTurns(t, "architect", "lead", "architect", "lead", "architect", "lead")
	sub.want(t, `pause {"reason":"`+_hopPause+`"}`)
	got := env.ranTexts()
	if len(got) != 13 || got[7] != "[Space 2, message 8]\nlead wrote: @architect again" {
		t.Errorf("ran = %q, want 13 turns, the eighth the wake held before the restart", got)
	}
}

func TestSend_UserMessageResetsHops(t *testing.T) {
	env := newLiveEnv(t)
	id, post := env.teamSpace(t)
	var runs atomic.Int32
	env.fake.onRun = func(role, _ string, _ *int64) {
		if runs.Add(1)%5 == 0 {
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
		for _, role := range []string{"lead", "architect", "lead", "architect", "lead"} {
			sub.want(t, `turn {"role":"`+role+`","status":"running"}`, `agent {"kind":"text","text":"hi"}`,
				`turn {"role":"`+role+`","status":"done"}`)
		}
	}
	if ch, err := env.st.Space(context.Background(), int64(id)); err != nil || ch.Paused != "" {
		t.Errorf("space = %+v, %v, want not paused after 8 hops in 2 chains", ch, err)
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
		{"user wake resets at the cap", 6, "user", true, 0},
		{"first agent wake", 0, "lead", true, 1},
		{"sixth agent wake", 5, "architect", true, 6},
		{"seventh agent wake is over", 6, "lead", false, 6},
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
