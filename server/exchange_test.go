package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/bhanuprakaash/reef/store"
)

// exchangeEnv is a chat (Space 1) and a Space with the lead and the architect (Space 2).
func exchangeEnv(t *testing.T) *liveEnv {
	t.Helper()
	env := newLiveEnv(t)
	env.teamSpace(t)
	return env
}

func (e *liveEnv) newExchange(t *testing.T, key string) store.DirectMessage {
	t.Helper()
	origin := int64(1)
	dm, created, err := e.st.CreateExchange(context.Background(), store.DirectMessage{
		SpaceID: 2, OriginSpaceID: &origin, From: "lead", To: "architect",
		FromBotID: testServerBot(t, e.st, "lead"), ToBotID: testServerBot(t, e.st, "architect"),
		Body: "review the plan", RequestKey: &key,
	}, "Asked architect: review the plan")
	if err != nil || !created {
		t.Fatalf("create exchange %q = %+v, %v, %v", key, dm, created, err)
	}
	return dm
}

func (e *liveEnv) exchange(t *testing.T, id int64) store.Exchange {
	t.Helper()
	status, body := do(t, http.MethodGet, fmt.Sprintf("%s/exchanges/%d", e.url, id), "")
	var ex store.Exchange
	if status != http.StatusOK || json.Unmarshal([]byte(body), &ex) != nil {
		t.Fatalf("GET exchange %d = %d %s", id, status, body)
	}
	return ex
}

// waitState loops until exchange id is in state.
func (e *liveEnv) waitState(t *testing.T, id int64, state string) store.Exchange {
	t.Helper()
	for {
		if ex := e.exchange(t, id); ex.Request.State == state {
			return ex
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (e *liveEnv) turnCount(t *testing.T, spaceID int64) int {
	t.Helper()
	turns, err := e.st.Turns(context.Background(), store.TurnFilter{SpaceID: &spaceID})
	if err != nil {
		t.Fatal(err)
	}
	return len(turns)
}

// originLines is the messages of the chat as "author: body (exchange id)".
func (e *liveEnv) originLines(t *testing.T) []string {
	t.Helper()
	msgs, err := e.st.Messages(context.Background(), 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, m := range msgs {
		id := int64(0)
		if m.ExchangeID != nil {
			id = *m.ExchangeID
		}
		lines = append(lines, fmt.Sprintf("%s: %s (exchange %d)", m.Author, m.Body, id))
	}
	return lines
}

func TestExchangeDeliveredOnceAndSurvivesRestart(t *testing.T) {
	ctx := context.Background()
	env := exchangeEnv(t)
	dm := env.newExchange(t, "k1")
	for range 2 {
		if err := env.h.s.deliverExchange(ctx, 2, dm.ID); err != nil {
			t.Fatal(err)
		}
	}
	env.waitState(t, dm.ID, store.DirectAnswered)
	env.waitIdle(t, 2)

	if _, created, err := env.st.CreateExchange(ctx, store.DirectMessage{
		SpaceID: 2, OriginSpaceID: dm.OriginSpaceID, From: "lead", To: "architect",
		FromBotID: dm.FromBotID, ToBotID: dm.ToBotID, Body: "again", RequestKey: dm.RequestKey,
	}, "again"); err != nil || created {
		t.Fatalf("same key created = %v, %v, want no new exchange", created, err)
	}
	if err := env.h.s.deliverExchange(ctx, 2, dm.ID); err != nil {
		t.Fatal(err)
	}
	if n := env.turnCount(t, 2); n != 1 {
		t.Errorf("turns in Space 2 = %d, want 1", n)
	}
	want := []string{
		fmt.Sprintf("reef: Asked architect: review the plan (exchange %d)", dm.ID),
		fmt.Sprintf("architect: hi (exchange %d)", dm.ID),
	}
	check := func() {
		t.Helper()
		got := env.originLines(t)
		ex := env.exchange(t, dm.ID)
		if fmt.Sprint(got) != fmt.Sprint(want) || ex.Response == nil || ex.Response.Body != "hi" ||
			ex.Request.State != store.DirectAnswered || ex.Request.SpaceID != 2 || *ex.Request.OriginSpaceID != 1 {
			t.Errorf("chat = %q, exchange = %+v, want %q answered, replied hi", got, ex, want)
		}
	}
	check()
	env.restart(t)
	check()
}

func TestExchangeRestartReconciliation(t *testing.T) {
	ctx := context.Background()
	env := exchangeEnv(t)
	answered := env.newExchange(t, "answered")
	if err := env.h.s.deliverExchange(ctx, 2, answered.ID); err != nil {
		t.Fatal(err)
	}
	env.waitState(t, answered.ID, store.DirectAnswered)
	env.waitIdle(t, 2)

	cut := env.newExchange(t, "cut")
	if _, err := env.st.CreateTurn(ctx, store.Turn{
		SpaceID: 2, Role: "architect", Backend: "claude", BotID: cut.ToBotID, DirectMessageID: &cut.ID,
	}); err != nil {
		t.Fatal(err)
	}
	waiting := env.newExchange(t, "waiting")
	if n := env.turnCount(t, 2); n != 2 {
		t.Fatalf("turns before restart = %d, want 2", n)
	}

	env.restart(t)
	if _, err := env.st.FailRunningTurns(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if err := env.st.PauseWaitingExchangeSpaces(ctx); err != nil {
		t.Fatal(err)
	}

	if ex := env.exchange(t, answered.ID); ex.Request.State != store.DirectAnswered || ex.Response.Body != "hi" {
		t.Errorf("answered after restart = %+v", ex)
	}
	ex := env.exchange(t, cut.ID)
	if ex.Request.State != store.DirectFailed || ex.Response == nil || ex.Response.Body != store.ExchangeInterrupted {
		t.Errorf("cut after restart = %+v, want failed with the interruption note", ex)
	}
	if got := env.exchange(t, waiting.ID).Request.State; got != store.DirectWaiting {
		t.Errorf("waiting after restart = %s, want waiting", got)
	}
	if sp, err := env.st.Space(ctx, 2); err != nil || sp.Paused != store.RestartReason {
		t.Errorf("Space 2 = %+v, %v, want paused %q", sp.Paused, err, store.RestartReason)
	}
	if n := env.turnCount(t, 2); n != 2 {
		t.Errorf("turns after reconciliation = %d, want 2: nothing replayed", n)
	}

	if status, body := do(t, http.MethodPost, env.url+"/spaces/2/continue", ""); status != http.StatusNoContent {
		t.Fatalf("continue = %d %s", status, body)
	}
	env.waitState(t, waiting.ID, store.DirectAnswered)
	env.waitIdle(t, 2)
	if n := env.turnCount(t, 2); n != 3 {
		t.Errorf("turns after continue = %d, want 3", n)
	}

	var wg sync.WaitGroup
	statuses := make([]int, 2)
	for i := range statuses {
		wg.Go(func() {
			statuses[i], _ = do(t, http.MethodPost, fmt.Sprintf("%s/exchanges/%d/retry", env.url, cut.ID), "")
		})
	}
	wg.Wait()
	if !(statuses[0] == 200 && statuses[1] == 409 || statuses[0] == 409 && statuses[1] == 200) {
		t.Errorf("two retries = %v, want one 200 and one 409", statuses)
	}
	env.waitState(t, cut.ID, store.DirectAnswered)
	env.waitIdle(t, 2)
	if n := env.turnCount(t, 2); n != 4 {
		t.Errorf("turns after retries = %d, want 4: one more", n)
	}
	if status, _ := do(t, http.MethodPost, fmt.Sprintf("%s/exchanges/%d/retry", env.url, cut.ID), ""); status != 409 {
		t.Errorf("retry of an answered exchange = %d, want 409", status)
	}
}

func TestExchangePausedClosedAndCancelled(t *testing.T) {
	ctx := context.Background()
	env := exchangeEnv(t)

	paused := env.newExchange(t, "paused")
	if err := env.st.PauseSpace(ctx, 2, "hold"); err != nil {
		t.Fatal(err)
	}
	if err := env.h.s.deliverExchange(ctx, 2, paused.ID); err != nil {
		t.Fatal(err)
	}
	if got := env.exchange(t, paused.ID).Request.State; got != store.DirectWaiting || env.turnCount(t, 2) != 0 {
		t.Errorf("paused destination: state = %s, turns = %d, want waiting, 0", got, env.turnCount(t, 2))
	}

	status, _ := do(t, http.MethodPost, fmt.Sprintf("%s/exchanges/%d/cancel", env.url, paused.ID), "")
	again, _ := do(t, http.MethodPost, fmt.Sprintf("%s/exchanges/%d/cancel", env.url, paused.ID), "")
	if status != 200 || again != 409 {
		t.Errorf("cancel twice = %d, %d, want 200, 409", status, again)
	}
	if err := env.h.s.deliverExchange(ctx, 2, paused.ID); err != nil || env.turnCount(t, 2) != 0 {
		t.Errorf("cancelled exchange delivered: %v, turns = %d", err, env.turnCount(t, 2))
	}

	closed := env.newExchange(t, "closed")
	if err := env.st.CloseSpace(ctx, 2); err != nil {
		t.Fatal(err)
	}
	late := env.newExchange(t, "late")
	if err := env.h.s.deliverExchange(ctx, 2, late.ID); err != nil {
		t.Fatal(err)
	}
	ex := env.exchange(t, late.ID)
	if ex.Request.State != store.DirectFailed || ex.Response.Body != "space 2 is closed: conflict" {
		t.Errorf("closed destination = %+v, want failed, closed", ex)
	}
	if got := env.exchange(t, closed.ID).Request.State; got != store.DirectStopped {
		t.Errorf("exchange waiting when its Space closed = %s, want stopped", got)
	}
	lines := env.originLines(t)
	if last := lines[len(lines)-1]; last != fmt.Sprintf("architect: No reply: failed, space 2 is closed: conflict (exchange %d)", late.ID) {
		t.Errorf("last chat line = %q", last)
	}
}
