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

const _capReason = "capped: more than 25 tool calls"

func describe(items []store.InboxItem) []string {
	out := []string{}
	for _, i := range items {
		out = append(out, fmt.Sprintf("%s %d %s", i.Kind, i.ChannelID, i.Text))
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
// its channel is idle again, and a channel that is not idle lists no waiting item.
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

// gatedTeam makes the lead post a wake for the architect when it runs "go", across a gate that
// is on, and the architect post a plan when it runs. It returns the team channel's id.
func (e *liveEnv) gatedTeam(t *testing.T, wake string) int {
	t.Helper()
	id, post := e.teamChannel(t)
	roles := []store.Role{
		{Name: "lead", Lead: true, Backend: "claude"}, {Name: "architect", Backend: "claude"},
	}
	_, err := e.st.ApplyTeam(context.Background(), store.Team{Name: "core", Roles: roles,
		Gates: []store.Gate{{From: "lead", To: "architect", On: true}}})
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
	id := env.gatedTeam(t, "@architect plan it")
	do(t, http.MethodPost, env.url+"/channels", `{"projectId":1,"name":"capped","team":"core"}`)
	do(t, http.MethodPost, env.url+"/channels", `{"projectId":1,"name":"asked","team":"core"}`)
	env.failedTurn(t, "lead", "build it", nil)
	sub := subscribeTo(t, env.url, id)
	env.sendTo(t, id, "go")
	sub.want(t, _idleFrame, `turn {"role":"lead","status":"running"}`,
		`agent {"kind":"text","text":"hi"}`)
	sub.next(t)
	sub.want(t, `turn {"role":"lead","status":"done"}`)
	if err := env.st.PauseChannel(ctx, 3, _capReason); err != nil {
		t.Fatal(err)
	}
	if _, err := env.st.CreateMessage(ctx,
		store.Message{ChannelID: 4, Author: "architect", Body: "which database?"}); err != nil {
		t.Fatal(err)
	}

	want := []string{
		"waiting 4 which database?", "approval 2 lead wants to wake architect",
		"failed 1 lead failed: server restarted", "paused 3 " + _capReason,
	}
	env.wantInbox(t, want...)
	_, body := do(t, http.MethodGet, env.url+"/inbox", "")
	for _, field := range []string{`"approvalId":1`, `"turnId":1`, `"featureId":null`} {
		if !strings.Contains(body, field) {
			t.Errorf("inbox = %s, want %s", body, field)
		}
	}

	env.restart(t)
	env.wantInbox(t, want...)
}

func TestInbox_ApproveClearsAndRuns(t *testing.T) {
	env := newLiveEnv(t)
	id := env.gatedTeam(t, "@architect plan it")
	sub := subscribeTo(t, env.url, id)
	env.sendTo(t, id, "go")
	sub.want(t, _idleFrame, `turn {"role":"lead","status":"running"}`,
		`agent {"kind":"text","text":"hi"}`)
	sub.next(t)
	sub.want(t, `turn {"role":"lead","status":"done"}`)
	env.wantInbox(t, "approval 2 lead wants to wake architect")

	if status, _ := do(t, http.MethodPost, fmt.Sprintf("%s/channels/%d/approvals/1", env.url, id),
		`{"approve":true}`); status != 204 {
		t.Fatalf("approve = %d, want 204", status)
	}
	for range 4 {
		sub.next(t)
	}
	if got := env.ranTexts(); !slices.Equal(got, []string{
		"[channel 2, message 1]\ngo", "[channel 2, message 2]\nlead wrote: @architect plan it",
	}) {
		t.Errorf("ran = %q, want the architect to run the held wake", got)
	}
	env.wantInbox(t, "waiting 2 the plan")
}

func TestInbox_ContinueClearsAndRuns(t *testing.T) {
	env := newLiveEnv(t)
	sub := subscribe(t, env.url)
	env.send(t, "cap")
	sub.want(t, _idleFrame, `turn {"role":"lead","status":"running"}`,
		`turn {"role":"lead","status":"failed","error":"capped: more than 25 tool calls"}`,
		`pause {"reason":"`+_capReason+`"}`)
	env.wantInbox(t, "paused 1 "+_capReason)

	if status, _ := do(t, http.MethodPost, env.url+"/channels/1/continue", ""); status != 204 {
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
	id, post := env.teamChannel(t)
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

func TestInboxEvents_AcrossChannels(t *testing.T) {
	env := newLiveEnv(t)
	id := env.gatedTeam(t, "@architect block")
	inbox := subscribePath(t, env.url+"/inbox/events")
	inbox.wantInbox(t)
	lead := subscribe(t, env.url)

	env.send(t, "cap")
	lead.want(t, _idleFrame, `turn {"role":"lead","status":"running"}`,
		`turn {"role":"lead","status":"failed","error":"`+_capReason+`"}`,
		`pause {"reason":"`+_capReason+`"}`)
	inbox.wantInbox(t, "paused 1 "+_capReason)
	env.sendTo(t, id, "go")
	inbox.wantInbox(t, "approval 2 lead wants to wake architect", "paused 1 "+_capReason)

	do(t, http.MethodPost, fmt.Sprintf("%s/channels/%d/approvals/1", env.url, id), `{"approve":true}`)
	<-env.fake.started
	inbox.wantInbox(t, "paused 1 "+_capReason)
	env.fake.release <- struct{}{}
	inbox.wantInbox(t, "waiting 2 the plan", "paused 1 "+_capReason)

	env.sendTo(t, id, "thanks")
	inbox.wantInbox(t, "paused 1 "+_capReason)
	if status, _ := do(t, http.MethodPost, env.url+"/channels/1/continue", ""); status != 204 {
		t.Fatalf("continue = %d, want 204", status)
	}
	inbox.wantInbox(t)
}

func TestInboxEvents_RejectAndDeleteSendFrames(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"reject", http.MethodPost, "/channels/2/approvals/1", `{"approve":false}`},
		{"delete", http.MethodDelete, "/channels/2", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newLiveEnv(t)
			id := env.gatedTeam(t, "@architect plan it")
			sub := subscribeTo(t, env.url, id)
			env.sendTo(t, id, "go")
			sub.want(t, _idleFrame, `turn {"role":"lead","status":"running"}`,
				`agent {"kind":"text","text":"hi"}`)
			sub.next(t)
			sub.want(t, `turn {"role":"lead","status":"done"}`)
			inbox := subscribePath(t, env.url+"/inbox/events")
			inbox.wantInbox(t, "approval 2 lead wants to wake architect")

			do(t, tt.method, env.url+tt.path, tt.body)
			inbox.wantInbox(t)
		})
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
