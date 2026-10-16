package server

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/store"
)

func (e *liveEnv) failedTurn(t *testing.T, role, text string, parent *int64) store.Turn {
	t.Helper()
	ctx := context.Background()
	turn, err := e.st.CreateTurn(ctx, store.Turn{
		ChannelID: 1, Role: role, Backend: "claude", WakeText: text, WakeParent: parent,
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

func TestRetry_Refused(t *testing.T) {
	ctx := context.Background()
	env := newLiveEnv(t)
	done, err := env.st.CreateTurn(ctx, store.Turn{
		ChannelID: 1, Role: "lead", Backend: "claude", WakeText: "hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	done.Status = "done"
	if _, err := env.st.EndTurn(ctx, done); err != nil {
		t.Fatal(err)
	}
	if _, err := env.st.CreateTurn(ctx, store.Turn{
		ChannelID: 1, Role: "lead", Backend: "claude", WakeText: "hello",
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
	if err := env.st.PauseChannel(ctx, 1, reason); err != nil {
		t.Fatal(err)
	}
	status, body := do(t, http.MethodPost, env.url+"/turns/4/retry", "")
	if status != 409 || body != `{"error":"channel 1 is paused: `+reason+`: conflict"}` {
		t.Errorf("retry on a paused channel = %d %s, want 409", status, body)
	}
	if _, body := do(t, http.MethodGet, env.url+"/turns/4", ""); !strings.Contains(body, `"retried":false`) {
		t.Errorf("turn = %s, want not retried after a refused retry", body)
	}
	if err := env.st.ContinueChannel(ctx, 1, store.TokenBudget); err != nil {
		t.Fatal(err)
	}

	env.send(t, "block")
	<-env.fake.started
	status, body = do(t, http.MethodPost, env.url+"/turns/4/retry", "")
	if status != 409 || body != `{"error":"channel 1 has a turn running: conflict"}` {
		t.Errorf("retry while a turn runs = %d %s, want 409", status, body)
	}
	env.fake.release <- struct{}{}
	if got := env.ranTexts(); !slices.Equal(got, []string{"[channel 1, message 1]\nblock"}) {
		t.Errorf("ran = %q, want only the send: no refused retry ran", got)
	}
}

func TestRetry_RefusedWhenSuperseded(t *testing.T) {
	ctx := context.Background()
	env := newLiveEnv(t)
	env.failedTurn(t, "lead", "build it", nil)
	later, err := env.st.CreateTurn(ctx, store.Turn{
		ChannelID: 1, Role: "lead", Backend: "claude", WakeText: "build it again",
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
	if _, body := do(t, http.MethodGet, env.url+"/turns?channel=1&retryable=1", ""); body != "[]" {
		t.Errorf("retryable turns = %s, want none", body)
	}
	env.wantInbox(t)
}

func TestRetry_StoppedTurnIsRetryableButNotInTheInbox(t *testing.T) {
	ctx := context.Background()
	env := newLiveEnv(t)
	turn, err := env.st.CreateTurn(ctx, store.Turn{
		ChannelID: 1, Role: "lead", Backend: "claude", WakeText: "build it",
	})
	if err != nil {
		t.Fatal(err)
	}
	turn.Status, turn.Error = "failed", "interrupted"
	if _, err := env.st.EndTurn(ctx, turn); err != nil {
		t.Fatal(err)
	}
	env.wantInbox(t)

	_, body := do(t, http.MethodGet, env.url+"/turns?channel=1&retryable=1", "")
	if !strings.Contains(body, `"id":1`) || !strings.Contains(body, `"error":"interrupted"`) {
		t.Errorf("retryable turns = %s, want the stopped turn", body)
	}
	if status, _ := do(t, http.MethodPost, env.url+"/turns/1/retry", ""); status != 202 {
		t.Errorf("retry = %d, want 202", status)
	}
}
