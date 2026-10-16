package server

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/store"
)

// gatedDeveloperChannel makes the lead hold a wake for the developer, which the team gates by
// default, and post one for the architect, which blocks, when it runs "go".
func (e *liveEnv) gatedDeveloperChannel(t *testing.T) (id int, sub *stream, approvals string) {
	t.Helper()
	id, post := e.teamChannel(t)
	roles := []store.Role{
		{Name: "lead", Lead: true, Backend: "claude"}, {Name: "architect", Backend: "claude"},
		{Name: "developer", Write: true, Backend: "claude"},
	}
	if _, err := e.st.ApplyTeam(context.Background(), store.Team{Name: "core", Roles: roles}); err != nil {
		t.Fatal(err)
	}
	e.fake.onRun = func(role, text string, _ *int64) {
		if role == "lead" && strings.HasSuffix(text, "\ngo") {
			post("lead", "@architect block", nil)
			post("lead", "@developer plan it", nil)
		}
	}
	sub = subscribeTo(t, e.url, id)
	e.sendTo(t, id, "go")
	<-e.fake.started
	sub.want(t, _idleFrame,
		`turn {"role":"lead","status":"running"}`, `agent {"kind":"text","text":"hi"}`)
	sub.next(t)
	sub.want(t, `turn {"role":"lead","status":"done"}`,
		`turn {"role":"architect","status":"running"}`, `agent {"kind":"text","text":"part1"}`)
	return id, sub, fmt.Sprintf("%s/channels/%d/approvals", e.url, id)
}

func TestApproval_ApproveWhileAnotherMemberRuns(t *testing.T) {
	env := newLiveEnv(t)
	_, sub, approvals := env.gatedDeveloperChannel(t)

	if status, body := do(t, http.MethodPost, approvals+"/1", `{"approve":true}`); status != 204 || body != "" {
		t.Fatalf("approve while the architect runs = %d %s, want 204", status, body)
	}
	if got := sub.next(t); !strings.Contains(got, `"status":"approved"`) {
		t.Errorf("frame = %s, want the approved approval", got)
	}
	if status, body := do(t, http.MethodGet, approvals, ""); status != 200 || body != "[]" {
		t.Errorf("approvals = %d %s, want none pending", status, body)
	}
	if got := len(env.ranTexts()); got != 2 {
		t.Errorf("turns run = %d, want 2: the developer waits for the architect", got)
	}
	env.fake.release <- struct{}{}
	sub.want(t, `agent {"kind":"text","text":"part2"}`, `turn {"role":"architect","status":"done"}`)
	sub.wantTurns(t, "developer")
	if got := env.ranTexts(); len(got) != 3 || !strings.HasSuffix(got[2], "lead wrote: @developer plan it") {
		t.Errorf("ran = %q, want the developer to run after the architect", got)
	}
}

func TestSend_WhileRunningGoesBeforeRoutedWakes(t *testing.T) {
	env := newLiveEnv(t)
	id, post := env.teamChannel(t)
	env.fake.onRun = func(role, text string, _ *int64) {
		if role == "lead" && strings.HasSuffix(text, "\ngo") {
			one := int64(1)
			post("lead", "@architect block", nil)
			post("lead", "@architect two", &one)
		}
	}
	sub := subscribeTo(t, env.url, id)
	env.sendTo(t, id, "go")
	<-env.fake.started
	sub.want(t, _idleFrame,
		`turn {"role":"lead","status":"running"}`, `agent {"kind":"text","text":"hi"}`,
		`turn {"role":"lead","status":"done"}`,
		`turn {"role":"architect","status":"running"}`, `agent {"kind":"text","text":"part1"}`)

	env.sendTo(t, id, "first")
	env.sendTo(t, id, "second")
	env.fake.release <- struct{}{}
	sub.want(t, `agent {"kind":"text","text":"part2"}`, `turn {"role":"architect","status":"done"}`)
	sub.wantTurns(t, "lead", "lead", "architect")
	want := []string{
		"[channel 2, message 1]\ngo",
		"[channel 2, message 2]\nlead wrote: @architect block",
		"[channel 2, message 4]\nfirst",
		"[channel 2, message 5]\nsecond",
		"[channel 2, message 3, thread 1]\nThread so far:\nuser: go\n\nlead wrote: @architect two",
	}
	if got := env.ranTexts(); !slices.Equal(got, want) {
		t.Errorf("ran = %q, want %q: the user's messages run in order, before the routed wake", got, want)
	}
}

func TestStop_PutsQueuedApprovalBackAndDropsQueuedMessage(t *testing.T) {
	env := newLiveEnv(t)
	id, _, approvals := env.gatedDeveloperChannel(t)
	if status, _ := do(t, http.MethodPost, approvals+"/1", `{"approve":true}`); status != 204 {
		t.Fatalf("approve = %d, want 204", status)
	}
	if status, _ := env.sendTo(t, id, "ping"); status != 202 {
		t.Fatalf("send = %d, want 202", status)
	}

	if status, _ := do(t, http.MethodPost, fmt.Sprintf("%s/channels/%d/stop", env.url, id), ""); status != 204 {
		t.Fatalf("stop = %d, want 204", status)
	}
	status, body := do(t, http.MethodGet, approvals, "")
	if status != 200 || !strings.Contains(body, `"id":1,`) || !strings.Contains(body, `"status":"pending"`) {
		t.Errorf("approvals = %d %s, want approval 1 pending again", status, body)
	}
	if got := env.ranTexts(); len(got) != 2 {
		t.Errorf("ran = %q, want only the lead and the architect: queued items do not run after a stop", got)
	}
	msgs, err := env.st.Messages(context.Background(), int64(id), 0)
	if err != nil || !slices.ContainsFunc(msgs, func(m store.Message) bool { return m.Body == "ping" }) {
		t.Errorf("messages = %+v, %v, want the queued message stored", msgs, err)
	}
	if l := env.h.s.lookup(int64(id)); l != nil {
		l.mu.Lock()
		defer l.mu.Unlock()
		if len(l.queued) != 0 || len(l.held) != 0 {
			t.Errorf("queued = %v, held = %v, want neither", l.queued, l.held)
		}
	}
}
