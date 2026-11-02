package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/store"
)

// wantFrame reads frames until one starts with prefix, and returns it.
func (s *stream) wantFrame(t *testing.T, prefix string) string {
	t.Helper()
	for {
		if f := s.next(t); strings.HasPrefix(f, prefix) {
			return f
		}
	}
}

// wantTurn reads frames until the turn of role has status.
func (s *stream) wantTurn(t *testing.T, role, status string) {
	t.Helper()
	for {
		f := s.wantFrame(t, "turn ")
		if strings.Contains(f, `"role":"`+role+`"`) && strings.Contains(f, `"status":"`+status+`"`) {
			return
		}
	}
}

func (e *liveEnv) prompts(t *testing.T, id int) []store.PermissionPrompt {
	t.Helper()
	status, body := do(t, http.MethodGet, fmt.Sprintf("%s/spaces/%d/prompts", e.url, id), "")
	var out []store.PermissionPrompt
	if err := json.Unmarshal([]byte(body), &out); status != 200 || err != nil {
		t.Fatalf("GET prompts = %d %s, %v, want 200 and a list", status, body, err)
	}
	return out
}

func (e *liveEnv) answers() []agent.Answer {
	e.fake.mu.Lock()
	defer e.fake.mu.Unlock()
	return slices.Clone(e.fake.answers)
}

func TestPrompt_WaitingTurnDoesNotBlockOthersAndAnswerContinuesIt(t *testing.T) {
	env := newLiveEnv(t)
	id, post := env.teamSpace(t)
	env.fake.onRun = func(role, text string, _ *int64) {
		if role == "lead" && strings.HasSuffix(text, "park") {
			post("lead", "@architect plan it", nil)
		}
	}
	sub := subscribeTo(t, env.url, id)
	sub.want(t, _idleFrame)

	env.sendTo(t, id, "park")
	if f := sub.wantFrame(t, "prompt "); !strings.Contains(f, `"status":"waiting"`) ||
		!strings.Contains(f, `"tool":"Bash"`) || !strings.Contains(f, `"input":{"command":"gzip -l /x"}`) ||
		!strings.Contains(f, `"rule":"Bash(gzip -l /x)"`) {
		t.Fatalf("prompt frame = %s, want a waiting Bash prompt", f)
	}
	sub.want(t, `turn {"role":"lead","status":"waiting"}`)
	sub.want(t, `turn {"role":"architect","status":"running"}`, `agent {"kind":"text","text":"hi"}`,
		`turn {"role":"architect","status":"done"}`)

	got := env.prompts(t, id)
	if len(got) != 1 || got[0].Status != store.PromptWaiting || got[0].Tool != "Bash" {
		t.Fatalf("prompts = %+v, want one waiting Bash prompt", got)
	}
	if status, body := do(t, http.MethodPost, fmt.Sprintf("%s/prompts/%d/answer", env.url, got[0].ID),
		`{"answer":"once"}`); status != 200 || !strings.Contains(body, `"status":"allowed"`) {
		t.Fatalf("answer = %d %s, want 200 allowed", status, body)
	}
	sub.wantFrame(t, "prompt ")
	sub.want(t, `turn {"role":"lead","status":"running"}`, `agent {"kind":"text","text":"answered"}`,
		`turn {"role":"lead","status":"done"}`)

	if a := env.answers(); len(a) != 1 || !a[0].Allow || a[0].Rule != "" {
		t.Errorf("answers = %+v, want one allow once", a)
	}
	if status, _ := do(t, http.MethodPost, fmt.Sprintf("%s/prompts/%d/answer", env.url, got[0].ID),
		`{"answer":"deny"}`); status != 409 {
		t.Errorf("second answer = %d, want 409", status)
	}
	if got := env.prompts(t, id); len(got) != 0 {
		t.Errorf("prompts after the answer = %+v, want none", got)
	}
}

func TestPrompt_UnansweredExpiresAndLateAllowAlwaysAddsRuleAndWakesBot(t *testing.T) {
	env := newLiveEnv(t)
	env.h.SetPromptWait(50 * time.Millisecond)
	id, _ := env.teamSpace(t)
	sub := subscribeTo(t, env.url, id)
	sub.want(t, _idleFrame)

	env.sendTo(t, id, "park")
	sub.wantTurn(t, "lead", "waiting")
	sub.wantTurn(t, "lead", "running")
	sub.want(t, `agent {"kind":"text","text":"answered"}`)
	sub.wantTurn(t, "lead", "done")

	if a := env.answers(); len(a) != 1 || a[0].Allow || a[0].Reason != "no answer from the user" {
		t.Fatalf("answers = %+v, want one denial, no answer from the user", a)
	}
	got := env.prompts(t, id)
	if len(got) != 1 || got[0].Status != store.PromptExpired || got[0].Reason != "no answer from the user" {
		t.Fatalf("prompts = %+v, want the expired prompt to stay", got)
	}

	if status, body := do(t, http.MethodPost, fmt.Sprintf("%s/prompts/%d/answer", env.url, got[0].ID),
		`{"answer":"always"}`); status != 200 || !strings.Contains(body, `"status":"allowed"`) ||
		!strings.Contains(body, `"granted":true`) {
		t.Fatalf("late answer = %d %s, want 200 allowed and granted", status, body)
	}
	lead := *testServerBot(t, env.st, "lead")
	status, body := do(t, http.MethodGet, fmt.Sprintf("%s/bots/%d", env.url, lead), "")
	if !strings.Contains(body, `"Bash(gzip -l /x)"`) {
		t.Errorf("GET bot = %d %s, want the rule in its tools", status, body)
	}
	deadline := time.Now().Add(5 * time.Second)
	for !slices.ContainsFunc(env.ranTexts(), func(s string) bool { return strings.Contains(s, "Try it again") }) {
		if time.Now().After(deadline) {
			t.Fatalf("ran = %q, want the lead woken to retry", env.ranTexts())
		}
		time.Sleep(5 * time.Millisecond)
	}
}
