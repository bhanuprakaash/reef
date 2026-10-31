package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/store"
)

func (e *liveEnv) guide(t *testing.T, bot, key, body string) store.Exchange {
	t.Helper()
	status, got := do(t, http.MethodPost,
		fmt.Sprintf("%s/spaces/2/bots/%d/guidance", e.url, *testServerBot(t, e.st, bot)),
		fmt.Sprintf(`{"body":%q,"key":%q}`, body, key))
	var ex store.Exchange
	if status != http.StatusCreated || json.Unmarshal([]byte(got), &ex) != nil {
		t.Fatalf("guide %s = %d %s", bot, status, got)
	}
	return ex
}

func TestGuidanceRunsAfterTheCurrentTurnAndBeforeWaitingHandoffs(t *testing.T) {
	ctx := context.Background()
	env := exchangeEnv(t)
	env.sendTo(t, 2, "go block")
	<-env.fake.started
	handoff := env.newExchange(t, "handoff")
	if err := env.h.s.deliverExchange(ctx, 2, handoff.ID); err != nil {
		t.Fatal(err)
	}
	g := env.guide(t, "architect", "k1", "use sessions")
	if dup := env.guide(t, "architect", "k1", "use sessions"); dup.Request.ID != g.Request.ID {
		t.Errorf("same key made exchange %d, want %d", dup.Request.ID, g.Request.ID)
	}
	if g.Request.State != store.DirectWaiting || g.Request.Kind != store.ExchangeGuidance || g.Request.From != "user" {
		t.Errorf("guidance = %+v, want a waiting guidance from user", g.Request)
	}

	env.fake.release <- struct{}{}
	env.waitState(t, g.Request.ID, store.DirectAnswered)
	env.waitState(t, handoff.ID, store.DirectAnswered)
	env.waitIdle(t, 2)

	ran := env.ranTexts()
	if len(ran) < 3 || !strings.HasSuffix(ran[0], "go block") ||
		!strings.HasPrefix(ran[1], "Guidance from the user (sent at ") ||
		!strings.HasSuffix(ran[1], fmt.Sprintf(", message %d): use sessions", *g.Request.OriginMessageID)) ||
		!strings.Contains(ran[2], "review the plan") {
		t.Errorf("turns ran %q, want the first message, the guidance, then the handoff", ran)
	}
	msgs, err := env.st.Messages(ctx, 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, m := range msgs {
		if m.ExchangeID != nil && *m.ExchangeID == g.Request.ID {
			got = append(got, m.Author+": "+m.Body)
		}
	}
	if want := "user: To architect: use sessions"; len(got) != 1 || got[0] != want {
		t.Errorf("messages linking the guidance = %q, want [%q]", got, want)
	}
}

func TestGuidanceRunsBeforeAnotherBotsQueuedHandoff(t *testing.T) {
	ctx := context.Background()
	env := newLiveEnv(t)
	env.teamSpace(t,
		store.Role{Name: "lead", Lead: true, Backend: "claude"},
		store.Role{Name: "architect", Backend: "claude"},
		store.Role{Name: "dev", Backend: "claude"})
	handoff := func(key string) store.DirectMessage {
		origin := int64(1)
		dm, _, err := env.st.CreateExchange(ctx, store.DirectMessage{
			SpaceID: 2, OriginSpaceID: &origin, From: "lead", To: "dev",
			FromBotID: testServerBot(t, env.st, "lead"), ToBotID: testServerBot(t, env.st, "dev"),
			Body: "build " + key, RequestKey: &key,
		}, "Asked dev: build "+key)
		if err != nil {
			t.Fatal(err)
		}
		if err := env.h.s.deliverExchange(ctx, 2, dm.ID); err != nil {
			t.Fatal(err)
		}
		return dm
	}
	env.sendTo(t, 2, "go block")
	<-env.fake.started
	build := handoff("one")
	env.guide(t, "architect", "k1", "use sessions")
	env.fake.release <- struct{}{}
	env.waitState(t, build.ID, store.DirectAnswered)
	env.waitIdle(t, 2)

	ran := env.ranTexts()
	if len(ran) < 3 || !strings.HasSuffix(ran[0], "go block") || !strings.HasPrefix(ran[1], "Guidance from the user") ||
		!strings.Contains(ran[2], "build one") {
		t.Errorf("turns ran %q, want the message, the guidance, then the dev handoff", ran)
	}
}

func TestGuidanceWaitsWhileThePausedSpaceStaysPaused(t *testing.T) {
	ctx := context.Background()
	env := exchangeEnv(t)
	if err := env.st.PauseSpace(ctx, 2, "hold"); err != nil {
		t.Fatal(err)
	}
	handoff := env.newExchange(t, "handoff")
	if err := env.h.s.deliverExchange(ctx, 2, handoff.ID); err != nil {
		t.Fatal(err)
	}
	g := env.guide(t, "architect", "k1", "use sessions")
	if c, err := env.st.Space(ctx, 2); err != nil || c.Paused != "hold" ||
		g.Request.State != store.DirectWaiting || env.turnCount(t, 2) != 0 {
		t.Fatalf("paused: Space = %+v, %v, state = %s, turns = %d, want paused, waiting, 0 turns",
			c, err, g.Request.State, env.turnCount(t, 2))
	}

	if status, body := do(t, http.MethodPost, env.url+"/spaces/2/continue", ""); status != http.StatusNoContent {
		t.Fatalf("continue = %d %s", status, body)
	}
	env.waitState(t, handoff.ID, store.DirectAnswered)
	env.waitIdle(t, 2)
	ran := env.ranTexts()
	if len(ran) < 2 || !strings.HasPrefix(ran[0], "Guidance from the user") || !strings.Contains(ran[1], "review the plan") {
		t.Errorf("turns ran %q, want the guidance, then the handoff", ran)
	}
}

func TestGuidanceRefusals(t *testing.T) {
	env := exchangeEnv(t)
	lead := *testServerBot(t, env.st, "lead")
	for _, c := range []struct {
		name, path, body string
		status           int
	}{
		{"no key", fmt.Sprintf("/spaces/2/bots/%d/guidance", lead), `{"body":"x"}`, http.StatusBadRequest},
		{"not a member", "/spaces/2/bots/99/guidance", `{"body":"x","key":"a"}`, http.StatusNotFound},
	} {
		if status, body := do(t, http.MethodPost, env.url+c.path, c.body); status != c.status {
			t.Errorf("%s = %d %s, want %d", c.name, status, body, c.status)
		}
	}
	if err := env.st.CloseSpace(context.Background(), 2); err != nil {
		t.Fatal(err)
	}
	if status, body := do(t, http.MethodPost, env.url+fmt.Sprintf("/spaces/2/bots/%d/guidance", lead),
		`{"body":"x","key":"b"}`); status != http.StatusConflict {
		t.Errorf("closed Space = %d %s, want 409", status, body)
	}
}
