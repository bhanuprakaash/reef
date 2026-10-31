package server

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"testing"

	"github.com/bhanuprakaash/reef/store"
)

func TestRevokeBot_StopsLiveTurnAndKeepsHistory(t *testing.T) {
	ctx := context.Background()
	env := newLiveEnv(t)
	team, err := env.st.SpaceTeam(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	botID := team.Roles[0].BotID
	bs, err := env.st.AdoptBotSession(ctx, 1, botID)
	if err != nil {
		t.Fatal(err)
	}
	tok, err := env.st.CreateToken(ctx, 1, botID)
	if err != nil {
		t.Fatal(err)
	}
	turn, err := env.st.CreateTurn(ctx, store.Turn{
		SpaceID: 1, BotID: &botID, BotSessionID: &bs.ID, Role: team.Roles[0].Name, Backend: "claude", WakeText: "go",
	})
	if err != nil {
		t.Fatal(err)
	}
	call, err := env.st.CreateToolCall(ctx, turn.ID, 0, "Bash", `{"command":"make deploy"}`)
	if err != nil {
		t.Fatal(err)
	}

	env.send(t, "block")
	<-env.fake.started
	status, body := do(t, http.MethodPost, env.url+"/bots/"+strconv.FormatInt(botID, 10)+"/revoke", "")
	if status != 200 || body != `{"sessions":1,"tokens":1,"toolCalls":1}` {
		t.Fatalf("revoke = %d %q, want 200 with 1 session, 1 tool call, 1 token", status, body)
	}
	env.waitIdle(t, 1)

	if n := env.fake.closed.Load(); n != 1 {
		t.Errorf("sessions closed = %d, want the live one closed", n)
	}
	if _, err := env.st.Token(ctx, tok.Token); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("token after revoke = %v, want ErrNotFound", err)
	}
	if _, err := env.st.ActiveBotSession(ctx, 1, botID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("active bot session after revoke = %v, want ErrNotFound", err)
	}
	calls, err := env.st.ToolCalls(ctx, turn.ID)
	if err != nil || len(calls) != 1 || calls[0].ID != call.ID || calls[0].Input != `{"command":"make deploy"}` ||
		calls[0].Result != "stopped: the Bot was revoked; the call may or may not have finished" {
		t.Errorf("tool calls = %+v, %v, want the call kept and marked stopped", calls, err)
	}
	env.wantMessages(t, 1,
		"user: block",
		"reef: "+team.Roles[0].Name+" was revoked: its sessions stopped. Tool activity that was running may "+
			"have finished or not; revoking does not undo its effects.")
}
