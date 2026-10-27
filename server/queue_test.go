package server

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func (e *liveEnv) lastRan(t *testing.T) string {
	t.Helper()
	e.fake.mu.Lock()
	defer e.fake.mu.Unlock()
	if len(e.fake.ran) == 0 {
		t.Fatal("no turn ran")
	}
	return e.fake.ran[len(e.fake.ran)-1]
}

func TestQueuedItemStartsWithItsFirstPrompt(t *testing.T) {
	env := newLiveEnv(t)
	do(t, http.MethodPost, env.url+"/work-items",
		`{"projectId":1,"title":"Login","description":"email and password","teamId":1,`+
			`"acceptanceLines":[{"text":"wrong password is refused"},{"text":"session lasts a day"}]}`)
	do(t, http.MethodPost, env.url+"/work-items", `{"projectId":1,"title":"Logout","teamId":1}`)
	do(t, http.MethodPut, env.url+"/work-items/1/brief", `{"body":"sessions, not JWT","author":"user"}`)

	if status, body := do(t, http.MethodPatch, env.url+"/work-items/1", `{"status":"queued"}`); status != 200 ||
		!strings.Contains(body, `"status":"in_progress"`) {
		t.Fatalf("queue the first item = %d %s, want 200 in_progress", status, body)
	}
	if status, body := do(t, http.MethodPatch, env.url+"/work-items/2", `{"status":"queued"}`); status != 200 ||
		!strings.Contains(body, `"status":"queued"`) || !strings.Contains(body, `"queuePosition":1`) {
		t.Fatalf("queue the second item = %d %s, want 200 queued at 1", status, body)
	}
	env.waitIdle(t, 2)

	want := "Work item 1: Login\nDescription: email and password\nAcceptance lines:\n" +
		"1. wrong password is refused\n2. session lasts a day\nBrief: sessions, not JWT"
	if got := env.lastRan(t); !strings.HasSuffix(got, want) {
		t.Errorf("first prompt = %q, want it to end with %q", got, want)
	}
	if _, body := do(t, http.MethodGet, env.url+"/spaces?workItem=2", ""); body != "[]" {
		t.Errorf("Spaces of the queued item = %s, want none", body)
	}
	if _, body := do(t, http.MethodGet, env.url+"/messages?space=2", ""); !strings.Contains(body, "Started from the queue.") {
		t.Errorf("messages of Space 2 = %s, want the started note", body)
	}
}

func TestQueuedItemStartsAfterRestart(t *testing.T) {
	env := newLiveEnv(t)
	do(t, http.MethodPost, env.url+"/work-items", `{"projectId":1,"title":"Login","teamId":1,"status":"queued"}`)
	env.restart(t)

	deadline := time.Now().Add(5 * time.Second)
	for {
		_, body := do(t, http.MethodGet, env.url+"/spaces?workItem=1", "")
		if body != "[]" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("queued item was not started after the restart")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, body := do(t, http.MethodGet, env.url+"/work-items/1", ""); !strings.Contains(body, `"status":"in_progress"`) {
		t.Errorf("work item after the restart = %s, want in_progress", body)
	}
}
