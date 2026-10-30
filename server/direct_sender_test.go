package server

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/bhanuprakaash/reef/store"
)

// TestDirectSenderTurnEndsUnrun gives a request its sender sent in a turn that is stopped or
// fails a failed response, with body "stopped" or "failed".
func TestDirectSenderTurnEndsUnrun(t *testing.T) {
	for _, tt := range []struct{ name, text, want string }{
		{"stopped", "go block", "stopped"},
		{"failed", "go fail", "failed"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			env := newLiveEnv(t)
			id, _ := env.teamSpace(t)
			env.fake.onRun = func(role, _ string, _ *int64) {
				if role != "lead" {
					return
				}
				if _, err := env.st.CreateDirectMessage(ctx, store.DirectMessage{
					SpaceID: int64(id), From: "lead", To: "architect", FromBotID: testServerBot(t, env.st, "lead"), ToBotID: testServerBot(t, env.st, "architect"), Body: "review login",
				}); err != nil {
					t.Error(err)
				}
			}
			env.sendTo(t, id, tt.text)
			if tt.want == "stopped" {
				<-env.fake.started
				do(t, http.MethodPost, fmt.Sprintf("%s/spaces/%d/stop", env.url, id), "")
			}
			env.waitIdle(t, id)
			direct, err := env.st.DirectMessages(ctx, int64(id), 0)
			if err != nil || len(direct) != 2 {
				t.Fatalf("direct = %+v, %v, want the request and one response", direct, err)
			}
			if got := direct[1]; got.Status != store.DirectFailed || got.Body != tt.want ||
				got.From != "architect" || got.To != "lead" {
				t.Errorf("response = %+v, want failed %q from architect to lead", got, tt.want)
			}
		})
	}
}

// TestDirectSenderRestart gives a request sent by a turn a restart cuts a failed response
// with body "failed", and leaves a request an earlier turn sent alone.
func TestDirectSenderRestart(t *testing.T) {
	ctx := context.Background()
	env := newLiveEnv(t)
	id, _ := env.teamSpace(t)
	send := func(body string) {
		t.Helper()
		if _, err := env.st.CreateDirectMessage(ctx, store.DirectMessage{
			SpaceID: int64(id), From: "lead", To: "architect", FromBotID: testServerBot(t, env.st, "lead"), ToBotID: testServerBot(t, env.st, "architect"), Body: body,
		}); err != nil {
			t.Fatal(err)
		}
	}
	send("earlier")
	if _, err := env.st.CreateTurn(ctx, store.Turn{
		SpaceID: int64(id), Role: "lead", BotID: testServerBot(t, env.st, "lead"), Backend: "claude", WakeText: "go",
	}); err != nil {
		t.Fatal(err)
	}
	send("review login")
	if _, err := env.st.FailRunningTurns(ctx, nil); err != nil {
		t.Fatal(err)
	}
	direct, err := env.st.DirectMessages(ctx, int64(id), 0)
	if err != nil || len(direct) != 3 {
		t.Fatalf("direct = %+v, %v, want 2 requests and one response", direct, err)
	}
	if got := direct[2]; got.Status != store.DirectFailed || got.Body != "failed" ||
		got.From != "architect" || got.To != "lead" || got.ReplyToID == nil || *got.ReplyToID != direct[1].ID {
		t.Errorf("response = %+v, want failed \"failed\" answering %q", got, "review login")
	}
}
