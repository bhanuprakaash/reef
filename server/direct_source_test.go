package server

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/bhanuprakaash/reef/store"
)

// TestDirectSourceMessage records the public message a sender was answering on the direct
// request it sends, and keeps it through a direct wake.
func TestDirectSourceMessage(t *testing.T) {
	ctx := context.Background()
	st, _ := mcpFixture(t)
	if _, err := st.CreateTeam(ctx, store.Team{Name: "core", Roles: []store.Role{
		{Name: "lead", Lead: true, Backend: "claude"},
		{Name: "developer", Backend: "claude"},
		{Name: "tester", Backend: "claude"},
	}}); err != nil {
		t.Fatal(err)
	}
	one := int64(1)
	ch, err := st.CreateSpace(ctx, store.Space{
		ProjectID: 1, WorkItemID: &one, Name: "team", Team: "core", Role: "lead", Backend: "claude",
	})
	if err != nil {
		t.Fatal(err)
	}
	public, err := st.CreateMessage(ctx, store.Message{SpaceID: ch.ID, Author: "user", Body: "build it"})
	if err != nil {
		t.Fatal(err)
	}
	send := func(role, to string) {
		t.Helper()
		tok, err := st.CreateToken(ctx, ch.ID, role)
		if err != nil {
			t.Fatal(err)
		}
		if got, isErr := callTool(t, st, tok)("send_to_role", map[string]any{"to": to, "body": "hi"}); isErr {
			t.Fatalf("send_to_role = %q, want success", got)
		}
	}
	endTurn := func(turn store.Turn) {
		t.Helper()
		turn.Status = "done"
		if _, err := st.EndTurn(ctx, turn); err != nil {
			t.Fatal(err)
		}
	}

	wake := fmt.Sprintf("[Space %d, message %d]\nbuild it", ch.ID, public.ID)
	turn, err := st.CreateTurn(ctx, store.Turn{SpaceID: ch.ID, Role: "lead", Backend: "claude", WakeText: wake})
	if err != nil {
		t.Fatal(err)
	}
	send("lead", "tester")
	endTurn(turn)

	direct, err := st.DirectMessages(ctx, ch.ID, 0)
	if err != nil || len(direct) != 1 {
		t.Fatalf("direct = %+v, %v, want 1", direct, err)
	}
	if got := direct[0].SourceMessageID; got == nil || *got != public.ID {
		t.Errorf("request sent while answering message %d has source %v", public.ID, got)
	}

	answer, err := st.CreateDirectResponse(ctx, direct[0].ID, "tester", "ok")
	if err != nil {
		t.Fatal(err)
	}
	turn, err = st.CreateTurn(ctx, store.Turn{
		SpaceID: ch.ID, Role: "lead", Backend: "claude",
		WakeText: "[Space 1, direct message 2, chain 1, source: direct response]", DirectMessageID: answer.IDRef(),
	})
	if err != nil {
		t.Fatal(err)
	}
	send("lead", "developer")
	endTurn(turn)
	direct, err = st.DirectMessages(ctx, ch.ID, 0)
	if err != nil || len(direct) != 3 {
		t.Fatalf("direct = %+v, %v, want 3", direct, err)
	}
	if got := direct[2].SourceMessageID; got == nil || *got != public.ID {
		t.Errorf("request sent from a direct wake has source %v, want %d", got, public.ID)
	}
}

// TestDirectFrameAfterShedWake tells an open stream when a stop gives a queued direct wake a
// failed response after the last turn frame.
func TestDirectFrameAfterShedWake(t *testing.T) {
	ctx := context.Background()
	env := newLiveEnv(t)
	id, _ := env.teamSpace(t)
	env.fake.directRelease = make(chan struct{})
	env.fake.onRun = func(role, text string, parent *int64) {
		if role != "lead" {
			return
		}
		for _, body := range []string{"first", "second"} {
			if _, err := env.st.CreateDirectMessage(ctx, store.DirectMessage{
				SpaceID: int64(id), From: "lead", To: "architect", Body: body,
			}); err != nil {
				t.Error(err)
			}
		}
	}
	sub := subscribeTo(t, env.url, id)
	env.sendTo(t, id, "go")
	select {
	case <-env.fake.directStart:
	case <-time.After(5 * time.Second):
		t.Fatal("direct wake did not start")
	}
	do(t, http.MethodPost, fmt.Sprintf("%s/spaces/%d/stop", env.url, id), "")
	for f := sub.next(t); f != "direct {}"; f = sub.next(t) {
	}
	direct, err := env.st.DirectMessages(ctx, int64(id), 0)
	if err != nil {
		t.Fatal(err)
	}
	var bodies []string
	for _, m := range direct {
		if m.Status == store.DirectFailed {
			bodies = append(bodies, m.Body)
		}
	}
	if len(bodies) != 2 || bodies[1] != "stopped" {
		t.Errorf("failed response bodies = %q, want two with \"stopped\" on the one never run", bodies)
	}
}
