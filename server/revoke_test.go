package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
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

	finished, err := env.st.CreateTurn(ctx, store.Turn{
		SpaceID: 1, BotID: &botID, BotSessionID: &bs.ID, Role: team.Roles[0].Name, Backend: "claude", WakeText: "earlier",
	})
	if err != nil {
		t.Fatal(err)
	}
	quiet, err := env.st.CreateToolCall(ctx, finished.ID, 0, "Bash", `{"command":"true"}`)
	if err != nil {
		t.Fatal(err)
	}
	finished.Status = "done"
	if _, err := env.st.EndTurn(ctx, finished); err != nil {
		t.Fatal(err)
	}

	env.send(t, "block")
	<-env.fake.started
	status, body := do(t, http.MethodPost, env.url+"/bots/"+strconv.FormatInt(botID, 10)+"/revoke", "")
	if status != 200 || body != `{"sessions":1,"tokens":1,"toolCalls":1}` {
		t.Fatalf("revoke = %d %q, want 200 with 1 session, 1 tool call, 1 token", status, body)
	}
	env.waitIdle(t, 1)

	if n := env.fake.closed.Load(); n != 0 || !slices.Equal(env.fake.dropped, []int64{botID}) {
		t.Errorf("sessions closed = %d, dropped = %v, want the Bot's agent dropped and the session kept", n, env.fake.dropped)
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
	if calls, err := env.st.ToolCalls(ctx, finished.ID); err != nil || len(calls) != 1 || calls[0].ID != quiet.ID || calls[0].Result != "" {
		t.Errorf("tool calls of a finished turn = %+v, %v, want the empty result unchanged", calls, err)
	}
	env.wantMessages(t, 1,
		"user: block",
		"reef: "+team.Roles[0].Name+" was revoked: its sessions stopped. Tool activity that was running may "+
			"have finished or not; revoking does not undo its effects.")
}

// turnFrames reads the stream until n turn frames have passed and returns them as role, status and error.
func turnFrames(t *testing.T, sub *stream, n int) []string {
	t.Helper()
	var turns []string
	for len(turns) < n {
		f := sub.next(t)
		if strings.HasPrefix(f, "turn ") {
			var s struct{ Role, Status, Error string }
			if err := json.Unmarshal([]byte(strings.TrimPrefix(f, "turn ")), &s); err != nil {
				t.Fatal(err)
			}
			turns = append(turns, s.Role+" "+s.Status+" "+s.Error)
		}
	}
	return turns
}

func TestRevokeBot_LeavesAnotherBotsTurnRunning(t *testing.T) {
	env := newLiveEnv(t)
	id, _ := env.teamSpace(t)
	architect := *testServerBot(t, env.st, "architect")
	if _, err := env.st.AdoptBotSession(context.Background(), int64(id), architect); err != nil {
		t.Fatal(err)
	}
	sub := subscribeTo(t, env.url, id)
	sub.want(t, _idleFrame)

	env.sendTo(t, id, "block")
	<-env.fake.started
	if status, body := do(t, http.MethodPost, env.url+"/bots/"+strconv.FormatInt(architect, 10)+"/revoke", ""); status != 200 {
		t.Fatalf("revoke = %d %q, want 200", status, body)
	}
	env.fake.release <- struct{}{}
	if got := turnFrames(t, sub, 2); !slices.Equal(got, []string{"lead running ", "lead done "}) {
		t.Errorf("turns = %q, want the lead's turn to finish", got)
	}
	env.sendTo(t, id, "next")
	env.waitIdle(t, id)
	if n := env.fake.closed.Load(); n != 0 || !slices.Equal(env.fake.dropped, []int64{architect}) {
		t.Errorf("sessions closed = %d, dropped = %v, want only the architect's agent dropped", n, env.fake.dropped)
	}
}

func TestRevokeBot_InterruptsOnlyItsOwnTurn(t *testing.T) {
	env := newLiveEnv(t)
	id, _ := env.teamSpace(t)
	lead := *testServerBot(t, env.st, "lead")
	if _, err := env.st.AdoptBotSession(context.Background(), int64(id), lead); err != nil {
		t.Fatal(err)
	}
	sub := subscribeTo(t, env.url, id)
	sub.want(t, _idleFrame)

	env.sendTo(t, id, "block")
	<-env.fake.started
	if status, _ := env.sendTo(t, id, "@architect design it"); status != 202 {
		t.Fatalf("send while busy = %d, want 202", status)
	}
	if status, body := do(t, http.MethodPost, env.url+"/bots/"+strconv.FormatInt(lead, 10)+"/revoke", ""); status != 200 {
		t.Fatalf("revoke = %d %q, want 200", status, body)
	}
	want := []string{"lead running ", "lead failed interrupted", "architect running ", "architect done "}
	if got := turnFrames(t, sub, len(want)); !slices.Equal(got, want) {
		t.Errorf("turns = %q, want %q", got, want)
	}
	if n := env.fake.closed.Load(); n != 0 || !slices.Equal(env.fake.dropped, []int64{lead}) {
		t.Errorf("sessions closed = %d, dropped = %v, want only the lead's agent dropped", n, env.fake.dropped)
	}
}
