package server

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/store"
)

func TestSpaceRefTypedWhileATurnRunsIsSaved(t *testing.T) {
	ctx := context.Background()
	env := exchangeEnv(t)
	env.sendTo(t, 2, "go block")
	<-env.fake.started
	env.sendTo(t, 2, "compare with #[scratch](space:1)")
	env.fake.release <- struct{}{}
	env.waitIdle(t, 2)

	msgs, err := env.st.Messages(ctx, 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	var refs []store.SpaceRef
	for _, m := range msgs {
		if strings.HasPrefix(m.Body, "compare with") {
			refs, err = env.st.MessageRefs(ctx, m.ID)
		}
	}
	if err != nil || len(refs) != 1 || refs[0].SpaceID != 1 || refs[0].Name != "scratch" {
		t.Errorf("refs of the message typed during the turn = %+v, %v, want Space 1 scratch", refs, err)
	}
}

func TestAskerIsWokenAfterItsChatResumesAndARestart(t *testing.T) {
	ctx := context.Background()
	e := newConsultEnv(t)
	key := "q"
	ex, _, err := e.st.Consult(ctx, store.DirectMessage{
		OriginSpaceID: ptr(int64(_adaChat)), From: "ada", To: "ben", FromBotID: &e.ada, ToBotID: &e.ben,
		Body: "q1", RequestKey: &key}, "Consultation from ada", "Asked ben: q1")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.st.PauseSpace(ctx, _adaChat, "hold"); err != nil {
		t.Fatal(err)
	}
	if err := e.h.s.deliverExchange(ctx, ex.SpaceID, ex.ID); err != nil {
		t.Fatal(err)
	}
	e.waitState(t, ex.ID, store.DirectAnswered)
	e.waitIdle(t, int(ex.SpaceID))
	wake := "[Space 2, consultation 4, exchange 1]\nben replied: hi"
	if ran := e.ranTexts(); slices.Contains(ran, wake) {
		t.Fatalf("ran = %q: Ada woke while her chat was paused", ran)
	}

	e.restart(t)
	if err := e.st.PauseWaitingExchangeSpaces(ctx); err != nil {
		t.Fatal(err)
	}
	if status, body := do(t, http.MethodPost, e.url+"/spaces/2/continue", ""); status != http.StatusNoContent {
		t.Fatalf("continue = %d %s", status, body)
	}
	e.waitIdle(t, _adaChat)
	if got := slices.Index(e.ranTexts(), wake); got < 0 {
		t.Errorf("ran = %q, want the wake of Ada %q", e.ranTexts(), wake)
	}
	for range 2 {
		e.h.s.wakeAsker(ctx, ex.ID, _adaChat)
	}
	e.waitIdle(t, _adaChat)
	n := 0
	for _, text := range e.ranTexts() {
		if text == wake {
			n++
		}
	}
	if n != 1 {
		t.Errorf("Ada woke %d times for the one reply, want 1", n)
	}
}

func TestFailedAskingTurnWithdrawsWhatItAsked(t *testing.T) {
	ctx := context.Background()
	e := newConsultEnv(t)
	run := e.fake.onRun
	e.fake.onRun = func(role, text string, parent *int64) {
		run(role, strings.TrimSuffix(text, " fail"), parent)
	}
	e.say(t, `consult:[{"bot":"ben","body":"q1"}] fail`)
	e.waitIdle(t, _adaChat)
	e.waitState(t, 1, store.DirectCancelled)
	e.say(t, "hello")
	e.waitIdle(t, _adaChat)

	if n := e.turnCount(t, 4); n != 0 {
		t.Errorf("turns in the consultation = %d, want 0: the withdrawn question did not run", n)
	}
	want := "reef: No reply: the asking turn failed (exchange 1)"
	lines := e.lines(t, _adaChat)
	if !slices.Contains(lines, want) {
		t.Errorf("chat = %q, want the line %q", lines, want)
	}
	if ex, err := e.st.Exchange(ctx, 1); err != nil || ex.Response != nil {
		t.Errorf("exchange 1 = %+v, %v, want no response", ex, err)
	}
}

func TestGuidanceSentUnderAClaimWaitsForContinue(t *testing.T) {
	ctx := context.Background()
	env := exchangeEnv(t)
	c, _, err := env.h.s.claim(ctx, 2, _noTurn)
	if err != nil {
		t.Fatal(err)
	}
	g := env.guide(t, "architect", "k1", "use sessions")
	_ = c.release(nil)
	space, err := env.st.Space(ctx, 2)
	if err != nil || g.Request.State != store.DirectWaiting || space.Paused == "" || env.turnCount(t, 2) != 0 {
		t.Fatalf("under a claim: state = %s, paused = %q, turns = %d, %v, want waiting, paused, 0 turns",
			g.Request.State, space.Paused, env.turnCount(t, 2), err)
	}
	if status, body := do(t, http.MethodPost, env.url+"/spaces/2/continue", ""); status != http.StatusNoContent {
		t.Fatalf("continue = %d %s", status, body)
	}
	env.waitState(t, g.Request.ID, store.DirectAnswered)
}

func TestStopKeepsGuidanceWaitingForTheNextWake(t *testing.T) {
	env := exchangeEnv(t)
	env.sendTo(t, 2, "go block")
	<-env.fake.started
	g := env.guide(t, "architect", "k1", "use sessions")
	if status, body := do(t, http.MethodPost, env.url+"/spaces/2/stop", ""); status != http.StatusNoContent {
		t.Fatalf("stop = %d %s", status, body)
	}
	env.waitIdle(t, 2)
	if got := env.exchange(t, g.Request.ID).Request.State; got != store.DirectWaiting {
		t.Fatalf("guidance after stop = %s, want waiting", got)
	}

	env.sendTo(t, 2, "next")
	env.waitState(t, g.Request.ID, store.DirectAnswered)
	env.waitIdle(t, 2)
	ran := env.ranTexts()
	if len(ran) != 3 || !strings.HasPrefix(ran[1], "Guidance from the user") || !strings.HasSuffix(ran[2], "next") {
		t.Errorf("turns ran %q, want the stopped one, the guidance, then the next message", ran)
	}
}

func TestStoppedExchangeCanBeRetried(t *testing.T) {
	env := exchangeEnv(t)
	dm := env.newExchange(t, "k1")
	if err := env.st.SetDirectRequestState(context.Background(), dm.ID, store.DirectStopped); err != nil {
		t.Fatal(err)
	}
	status, body := do(t, http.MethodPost, fmt.Sprintf("%s/exchanges/%d/retry", env.url, dm.ID), "")
	if status != http.StatusOK {
		t.Fatalf("retry = %d %s", status, body)
	}
	env.waitState(t, dm.ID, store.DirectAnswered)
}

func TestConsultMakesNoSpaceWhenItsExchangeFails(t *testing.T) {
	ctx := context.Background()
	e := newConsultEnv(t)
	before, err := e.st.Spaces(ctx, store.SpaceFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.st.Consult(ctx, store.DirectMessage{
		OriginSpaceID: ptr(int64(_adaChat)), From: "ada", To: "ben", FromBotID: ptr(int64(999)),
		ToBotID: &e.ben, Body: "q1"}, "Consultation from ada", "Asked ben: q1"); err == nil {
		t.Fatal("consult by an unknown Bot succeeded")
	}
	after, err := e.st.Spaces(ctx, store.SpaceFilter{})
	if err != nil || len(after) != len(before) {
		t.Errorf("Spaces after the failed consult = %d, %v, want %d", len(after), err, len(before))
	}
}
