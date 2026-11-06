package server

import (
	"context"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/store"
)

func (e *liveEnv) failedTurn(t *testing.T, role, text string, parent *int64) store.Turn {
	t.Helper()
	ctx := context.Background()
	turn, err := e.st.CreateTurn(ctx, store.Turn{
		SpaceID: 1, Role: role, BotID: testServerBot(t, e.st, role), Backend: "claude", WakeText: text, WakeParent: parent,
	})
	if err != nil {
		t.Fatal(err)
	}
	turn.Status, turn.Error = "failed", "server restarted"
	if turn, err = e.st.EndTurn(ctx, turn); err != nil {
		t.Fatal(err)
	}
	return turn
}

func TestRetry_RunsTheWakeAgain(t *testing.T) {
	env := newLiveEnv(t)
	env.failedTurn(t, "lead", "build it", nil)
	sub := subscribe(t, env.url)

	if status, body := do(t, http.MethodPost, env.url+"/turns/1/retry", ""); status != 202 || body != "" {
		t.Fatalf("retry = %d %q, want 202", status, body)
	}
	sub.want(t, _idleFrame, `turn {"role":"lead","status":"running"}`,
		`agent {"kind":"text","text":"hi"}`, `turn {"role":"lead","status":"done"}`)
	if got := env.ranTexts(); !slices.Equal(got, []string{"build it"}) {
		t.Errorf("ran = %q, want the wake of the failed turn", got)
	}
	if _, body := do(t, http.MethodGet, env.url+"/turns/1", ""); !strings.Contains(body, `"retried":true`) {
		t.Errorf("turn = %s, want retried", body)
	}

	status, body := do(t, http.MethodPost, env.url+"/turns/1/retry", "")
	if status != 409 || body != `{"error":"turn 1 cannot be retried: conflict"}` {
		t.Errorf("retry again = %d %s, want 409", status, body)
	}
	if got := len(env.ranTexts()); got != 1 {
		t.Errorf("turns run = %d, want 1: a second retry runs nothing", got)
	}
}

func TestRetry_RoutedWakeThatNeverSentIsBuiltAgain(t *testing.T) {
	scopes := store.Scopes{{Thread: 7, Asked: []string{"Asked together: @lead, @architect"}}}
	tests := []struct {
		name     string
		sent     bool
		wantWoke []store.Scopes
	}{
		{"never sent", false, []store.Scopes{scopes}},
		{"sent", true, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			env := newLiveEnv(t)
			turn, err := env.st.CreateTurn(ctx, store.Turn{
				SpaceID: 1, Role: "lead", BotID: testServerBot(t, env.st, "lead"), Backend: "claude", WakeText: "routed text", Scopes: scopes,
			})
			if err != nil {
				t.Fatal(err)
			}
			if tt.sent {
				if _, err := env.st.SetTurnSent(ctx, turn.ID, "prompt"); err != nil {
					t.Fatal(err)
				}
			}
			turn.Status, turn.Error = "failed", "no binary"
			if _, err := env.st.EndTurn(ctx, turn); err != nil {
				t.Fatal(err)
			}
			sub := subscribe(t, env.url)

			if status, body := do(t, http.MethodPost, env.url+"/turns/1/retry", ""); status != 202 {
				t.Fatalf("retry = %d %q, want 202", status, body)
			}
			sub.want(t, _idleFrame, `turn {"role":"lead","status":"running"}`,
				`agent {"kind":"text","text":"hi"}`, `turn {"role":"lead","status":"done"}`)
			env.fake.mu.Lock()
			defer env.fake.mu.Unlock()
			if !reflect.DeepEqual(env.fake.woke, tt.wantWoke) {
				t.Errorf("woke = %+v, want %+v", env.fake.woke, tt.wantWoke)
			}
		})
	}
}

func TestRetry_DirectTurnKeepsReplyPrivate(t *testing.T) {
	ctx := context.Background()
	env := newLiveEnv(t)
	spaceID, _ := env.teamSpace(t)
	req, err := env.st.CreateDirectMessage(ctx, store.DirectMessage{
		SpaceID: int64(spaceID), From: "lead", To: "architect", FromBotID: testServerBot(t, env.st, "lead"), ToBotID: testServerBot(t, env.st, "architect"), Body: "plan it",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.st.CreateFailedDirectResponse(ctx, req.ID, req.ToBotID, "architect", "partial"); err != nil {
		t.Fatal(err)
	}
	turn, err := env.st.CreateTurn(ctx, store.Turn{
		SpaceID: int64(spaceID), Role: "architect", BotID: testServerBot(t, env.st, "architect"), Backend: "claude",
		WakeText: "direct wake", DirectMessageID: &req.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	turn.Status, turn.Error = "failed", "server restarted"
	if _, err := env.st.EndTurn(ctx, turn); err != nil {
		t.Fatal(err)
	}
	sub := subscribeTo(t, env.url, spaceID)
	l := env.h.s.lookup(int64(spaceID))
	l.mu.Lock()
	l.hops = store.HopCap
	l.mu.Unlock()

	if status, body := do(t, http.MethodPost, env.url+"/turns/1/retry", ""); status != 202 || body != "" {
		t.Fatalf("retry = %d %q, want 202", status, body)
	}
	sub.want(t, _idleFrame)
	sub.want(t,
		`turn {"role":"architect","status":"running","directMessageId":1}`,
		`agent {"kind":"text","text":"hi"}`,
		`turn {"role":"architect","status":"done","directMessageId":1}`,
		`turn {"role":"lead","status":"running","directMessageId":3}`,
		`agent {"kind":"text","text":"hi"}`,
		`turn {"role":"lead","status":"done","directMessageId":3}`)
	direct, err := env.st.DirectMessages(ctx, int64(spaceID), 0)
	if err != nil || len(direct) != 3 || direct[1].Status != store.DirectFailed ||
		direct[2].Status != store.DirectDone || direct[2].ReplyToID == nil ||
		*direct[2].ReplyToID != req.ID {
		t.Errorf("direct messages = %+v, %v, want failed attempt then retried response", direct, err)
	}
	public, err := env.st.Messages(ctx, int64(spaceID), 0)
	if err != nil || len(public) != 0 {
		t.Errorf("public messages = %+v, %v, want none", public, err)
	}
	if got := env.ranTexts(); len(got) != 2 {
		t.Errorf("turns run = %q, want recipient retry and exactly one sender wake", got)
	}
}

func TestRetry_Refused(t *testing.T) {
	ctx := context.Background()
	env := newLiveEnv(t)
	done, err := env.st.CreateTurn(ctx, store.Turn{
		SpaceID: 1, Role: "lead", BotID: testServerBot(t, env.st, "lead"), Backend: "claude", WakeText: "hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	done.Status = "done"
	if _, err := env.st.EndTurn(ctx, done); err != nil {
		t.Fatal(err)
	}
	if _, err := env.st.CreateTurn(ctx, store.Turn{
		SpaceID: 1, Role: "lead", BotID: testServerBot(t, env.st, "lead"), Backend: "claude", WakeText: "hello",
	}); err != nil {
		t.Fatal(err)
	}
	env.failedTurn(t, "lead", "", nil)
	env.failedTurn(t, "lead", "build it", nil)

	tests := []struct {
		name       string
		turn       string
		wantStatus int
		wantBody   string
	}{
		{"done", "1", 409, `{"error":"turn 1 cannot be retried: conflict"}`},
		{"running", "2", 409, `{"error":"turn 2 cannot be retried: conflict"}`},
		{"no wake", "3", 409, `{"error":"turn 3 cannot be retried: conflict"}`},
		{"unknown", "9", 404, `{"error":"turn 9: not found"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := do(t, http.MethodPost, env.url+"/turns/"+tt.turn+"/retry", "")
			if status != tt.wantStatus || body != tt.wantBody {
				t.Errorf("retry = %d %s, want %d %s", status, body, tt.wantStatus, tt.wantBody)
			}
		})
	}

	const reason = "token budget of 1000000 used: 1000312 tokens"
	if err := env.st.PauseSpace(ctx, 1, reason); err != nil {
		t.Fatal(err)
	}
	status, body := do(t, http.MethodPost, env.url+"/turns/4/retry", "")
	if status != 409 || body != `{"error":"space 1 is paused: `+reason+`: conflict"}` {
		t.Errorf("retry on a paused Space = %d %s, want 409", status, body)
	}
	if _, body := do(t, http.MethodGet, env.url+"/turns/4", ""); !strings.Contains(body, `"retried":false`) {
		t.Errorf("turn = %s, want not retried after a refused retry", body)
	}
	if err := env.st.ContinueSpace(ctx, 1, store.TokenBudget); err != nil {
		t.Fatal(err)
	}

	env.send(t, "block")
	<-env.fake.started
	status, body = do(t, http.MethodPost, env.url+"/turns/4/retry", "")
	if status != 409 || body != `{"error":"space 1 has a turn running: conflict"}` {
		t.Errorf("retry while a turn runs = %d %s, want 409", status, body)
	}
	env.fake.release <- struct{}{}
	if got := env.ranTexts(); !slices.Equal(got, []string{"[Space 1, message 1]\nblock"}) {
		t.Errorf("ran = %q, want only the send: no refused retry ran", got)
	}
}

func TestRetry_RefusedWhenSuperseded(t *testing.T) {
	ctx := context.Background()
	env := newLiveEnv(t)
	env.failedTurn(t, "lead", "build it", nil)
	later, err := env.st.CreateTurn(ctx, store.Turn{
		SpaceID: 1, Role: "lead", BotID: testServerBot(t, env.st, "lead"), Backend: "claude", WakeText: "build it again",
	})
	if err != nil {
		t.Fatal(err)
	}
	later.Status = "done"
	if _, err := env.st.EndTurn(ctx, later); err != nil {
		t.Fatal(err)
	}

	status, body := do(t, http.MethodPost, env.url+"/turns/1/retry", "")
	if status != 409 || body != `{"error":"turn 1 cannot be retried: conflict"}` {
		t.Errorf("retry = %d %s, want 409", status, body)
	}
	if _, body := do(t, http.MethodGet, env.url+"/turns?space=1&retryable=1", ""); body != "[]" {
		t.Errorf("retryable turns = %s, want none", body)
	}
	env.wantInbox(t)
}

func TestRetry_StoppedTurnIsRetryableButNotInTheInbox(t *testing.T) {
	ctx := context.Background()
	env := newLiveEnv(t)
	turn, err := env.st.CreateTurn(ctx, store.Turn{
		SpaceID: 1, Role: "lead", BotID: testServerBot(t, env.st, "lead"), Backend: "claude", WakeText: "build it",
	})
	if err != nil {
		t.Fatal(err)
	}
	turn.Status, turn.Error = "failed", "interrupted"
	if _, err := env.st.EndTurn(ctx, turn); err != nil {
		t.Fatal(err)
	}
	env.wantInbox(t)

	_, body := do(t, http.MethodGet, env.url+"/turns?space=1&retryable=1", "")
	if !strings.Contains(body, `"id":1`) || !strings.Contains(body, `"error":"interrupted"`) {
		t.Errorf("retryable turns = %s, want the stopped turn", body)
	}
	if status, _ := do(t, http.MethodPost, env.url+"/turns/1/retry", ""); status != 202 {
		t.Errorf("retry = %d, want 202", status)
	}
}
