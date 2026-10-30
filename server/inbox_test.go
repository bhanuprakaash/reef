package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bhanuprakaash/reef/store"
)

const _pauseReason = "token budget of 1000000 used: 1000312 tokens"

func describe(items []store.InboxItem) []string {
	out := []string{}
	for _, i := range items {
		out = append(out, fmt.Sprintf("%s %d %s", i.Kind, i.SpaceID, i.Text))
	}
	return out
}

func (e *liveEnv) inbox(t *testing.T) []string {
	t.Helper()
	status, body := do(t, http.MethodGet, e.url+"/inbox", "")
	var items []store.InboxItem
	if status != 200 || json.Unmarshal([]byte(body), &items) != nil {
		t.Fatalf("GET /inbox = %d %s, want 200 and a list", status, body)
	}
	if len(items) == 0 && strings.TrimSpace(body) != "[]" {
		t.Fatalf("GET /inbox = %s, want []", body)
	}
	return describe(items)
}

// wantInbox waits for GET /inbox to list these items: a chain's last frame comes just before
// its Space is idle again, and a Space that is not idle lists no waiting item.
func (e *liveEnv) wantInbox(t *testing.T, want ...string) {
	t.Helper()
	if want == nil {
		want = []string{}
	}
	var got []string
	for end := time.Now().Add(5 * time.Second); time.Now().Before(end); time.Sleep(5 * time.Millisecond) {
		if got = e.inbox(t); slices.Equal(got, want) {
			return
		}
	}
	t.Errorf("inbox = %q, want %q", got, want)
}

func (s *stream) wantInbox(t *testing.T, want ...string) {
	t.Helper()
	data, ok := strings.CutPrefix(s.next(t), "inbox ")
	var items []store.InboxItem
	if !ok || json.Unmarshal([]byte(data), &items) != nil {
		t.Fatalf("frame = %q, want an inbox event with a list", data)
	}
	if len(items) == 0 && data != "[]" {
		t.Fatalf("inbox frame = %q, want []", data)
	}
	if want == nil {
		want = []string{}
	}
	if got := describe(items); !slices.Equal(got, want) {
		t.Fatalf("inbox frame = %q, want %q", got, want)
	}
}

// handoffTeam makes the lead hand work to the architect, who posts a plan.
func (e *liveEnv) handoffTeam(t *testing.T, wake string) int {
	t.Helper()
	id, post := e.teamSpace(t)
	roles := []store.Role{
		{Name: "lead", Lead: true, Backend: "claude"}, {Name: "architect", Backend: "claude"},
	}
	_, err := testServerTeam(t, e.st, context.Background(), store.Team{Name: "core", Roles: roles})
	if err != nil {
		t.Fatal(err)
	}
	e.fake.onRun = func(role, text string, _ *int64) {
		switch {
		case role == "lead" && strings.HasSuffix(text, "\ngo"):
			post("lead", wake, nil)
		case role == "architect":
			post("architect", "the plan", nil)
		}
	}
	return id
}

func TestInbox_ListsEachKindAndSurvivesRestart(t *testing.T) {
	ctx := context.Background()
	env := newLiveEnv(t)
	_ = env.handoffTeam(t, "@architect plan it")
	do(t, http.MethodPost, env.url+"/spaces", `{"projectId":1,"name":"capped","team":"core"}`)
	do(t, http.MethodPost, env.url+"/spaces", `{"projectId":1,"name":"asked","team":"core"}`)
	env.failedTurn(t, "lead", "build it", nil)
	if err := env.st.PauseSpace(ctx, 3, _pauseReason); err != nil {
		t.Fatal(err)
	}
	if _, err := env.st.CreateMessage(ctx,
		store.Message{SpaceID: 4, Author: "architect", Body: "which database?"}); err != nil {
		t.Fatal(err)
	}

	want := []string{
		"waiting 4 which database?",
		"failed 1 lead failed: server restarted", "paused 3 " + _pauseReason,
	}
	env.wantInbox(t, want...)
	_, body := do(t, http.MethodGet, env.url+"/inbox", "")
	for _, field := range []string{`"turnId":1`, `"workItemId":null`} {
		if !strings.Contains(body, field) {
			t.Errorf("inbox = %s, want %s", body, field)
		}
	}

	env.restart(t)
	env.wantInbox(t, want...)
}

func TestInbox_HandoffRunsWithoutUserAction(t *testing.T) {
	env := newLiveEnv(t)
	id := env.handoffTeam(t, "@architect plan it")
	_ = subscribeTo(t, env.url, id)
	env.sendTo(t, id, "go")
	for end := time.Now().Add(5 * time.Second); len(env.ranTexts()) < 2 && time.Now().Before(end); {
		time.Sleep(5 * time.Millisecond)
	}
	if got := env.ranTexts(); !slices.Equal(got, []string{
		"[Space 2, message 1]\ngo", "[Space 2, message 2]\nlead wrote: @architect plan it",
	}) {
		t.Errorf("ran = %q, want lead then architect without user action", got)
	}
	env.wantInbox(t, "waiting 2 the plan")
}

func TestInbox_ContinueClearsAndRuns(t *testing.T) {
	env := newLiveEnv(t)
	sub := subscribe(t, env.url)
	reason := env.overBudget(t)
	sub.want(t, _idleFrame, `pause {"reason":"`+reason+`"}`)
	env.wantInbox(t, "paused 1 "+reason)

	if status, _ := do(t, http.MethodPost, env.url+"/spaces/1/continue", ""); status != 204 {
		t.Fatalf("continue = %d, want 204", status)
	}
	sub.want(t, `pause {"reason":""}`)
	env.wantInbox(t)
	if status, _ := env.send(t, "hello"); status != 202 {
		t.Errorf("send after continue = %d, want 202", status)
	}
	sub.want(t, `turn {"role":"lead","status":"running"}`, `agent {"kind":"text","text":"hi"}`,
		`turn {"role":"lead","status":"done"}`)
}

func TestInbox_RetryClearsAndRuns(t *testing.T) {
	env := newLiveEnv(t)
	env.failedTurn(t, "lead", "build it", nil)
	sub := subscribe(t, env.url)
	env.wantInbox(t, "failed 1 lead failed: server restarted")

	if status, _ := do(t, http.MethodPost, env.url+"/turns/1/retry", ""); status != 202 {
		t.Fatalf("retry = %d, want 202", status)
	}
	sub.want(t, _idleFrame, `turn {"role":"lead","status":"running"}`,
		`agent {"kind":"text","text":"hi"}`, `turn {"role":"lead","status":"done"}`)
	env.wantInbox(t)
}

func TestInbox_SendClearsWaiting(t *testing.T) {
	env := newLiveEnv(t)
	id, post := env.teamSpace(t)
	env.fake.onRun = func(role, _ string, _ *int64) {
		if role == "lead" {
			post("lead", "what next?", nil)
		}
	}
	sub := subscribeTo(t, env.url, id)
	env.sendTo(t, id, "hi")
	sub.want(t, _idleFrame, `turn {"role":"lead","status":"running"}`,
		`agent {"kind":"text","text":"hi"}`, `turn {"role":"lead","status":"done"}`)
	env.wantInbox(t, "waiting 2 what next?")

	env.fake.onRun = nil
	env.sendTo(t, id, "thanks")
	sub.want(t, `turn {"role":"lead","status":"running"}`, `agent {"kind":"text","text":"hi"}`,
		`turn {"role":"lead","status":"done"}`)
	env.wantInbox(t)
}

func TestInbox_DismissWaiting(t *testing.T) {
	env := newLiveEnv(t)
	id, post := env.teamSpace(t)
	env.fake.onRun = func(role, _ string, _ *int64) {
		if role == "lead" {
			post("lead", "what next?", nil)
		}
	}
	sub := subscribeTo(t, env.url, id)
	env.sendTo(t, id, "hi")
	sub.want(t, _idleFrame, `turn {"role":"lead","status":"running"}`,
		`agent {"kind":"text","text":"hi"}`, `turn {"role":"lead","status":"done"}`)
	env.wantInbox(t, "waiting 2 what next?")

	url := env.url + "/inbox/dismiss"
	if status, _ := do(t, http.MethodPost, url, fmt.Sprintf(`{"kind":"waiting","spaceId":%d,"messageId":999}`, id)); status != 404 {
		t.Errorf("dismiss of an unknown message = %d, want 404", status)
	}
	_, list := do(t, http.MethodGet, env.url+"/inbox", "")
	var items []store.InboxItem
	if err := json.Unmarshal([]byte(list), &items); err != nil || len(items) != 1 || items[0].MessageID == nil {
		t.Fatalf("GET /inbox = %s, want one waiting item with a messageId", list)
	}
	if status, body := do(t, http.MethodPost, url, fmt.Sprintf(`{"kind":"waiting","spaceId":%d,"messageId":%d}`, id, *items[0].MessageID)); status != 204 {
		t.Fatalf("dismiss = %d %s, want 204", status, body)
	}
	env.wantInbox(t)
}

func TestInboxEvents_PausedOnlyOnceIdle(t *testing.T) {
	env := newLiveEnv(t)
	inbox := subscribePath(t, env.url+"/inbox/events")
	inbox.wantInbox(t)

	for range 20 {
		reason := env.overBudget(t)
		inbox.wantInbox(t, "paused 1 "+reason)
		if status, _ := do(t, http.MethodPost, env.url+"/spaces/1/continue", ""); status != 204 {
			t.Fatalf("continue on the first paused frame = %d, want 204", status)
		}
		inbox.wantInbox(t)
	}
}

func TestInboxEvents_ServerCloseEndsStream(t *testing.T) {
	env := newLiveEnv(t)
	inbox := subscribePath(t, env.url+"/inbox/events")
	inbox.wantInbox(t)

	env.h.Close()
	select {
	case f, ok := <-inbox.frames:
		if ok {
			t.Errorf("frame = %q, want the stream to end", f)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("inbox stream still open 5s after the server closed")
	}
}
