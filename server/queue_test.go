package server

import (
	"net/http"
	"slices"
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

	want := "Work item 1: Login\nDescription: email and password\nBrief: sessions, not JWT"
	if got := env.lastRan(t); !strings.HasSuffix(got, want) {
		t.Errorf("first prompt = %q, want it to end with %q", got, want)
	}
	if _, body := do(t, http.MethodGet, env.url+"/spaces?workItem=2", ""); body != "[]" {
		t.Errorf("Spaces of the queued item = %s, want none", body)
	}
	if _, body := do(t, http.MethodGet, env.url+"/messages?space=2", ""); strings.Contains(body, `"author":"reef"`) {
		t.Errorf("messages of Space 2 = %s, want no note from reef", body)
	}
	if got := groupKinds(env.inboxItems(t)); !slices.Equal(got, []string{"fyi started"}) {
		t.Errorf("inbox = %q, want the started item", got)
	}
}

func TestQueuedItemStartsAfterRestart(t *testing.T) {
	env := newLiveEnv(t)
	do(t, http.MethodPost, env.url+"/work-items", `{"projectId":1,"title":"Login","teamId":1}`)
	do(t, http.MethodPost, env.url+"/work-items", `{"projectId":1,"title":"Logout","teamId":1}`)
	do(t, http.MethodPatch, env.url+"/work-items/1", `{"status":"queued"}`)
	do(t, http.MethodPatch, env.url+"/work-items/2", `{"status":"queued"}`)
	env.waitIdle(t, 2)
	env.exec(t, "UPDATE work_item SET status = 'done' WHERE id = 1")
	env.restart(t)

	deadline := time.Now().Add(5 * time.Second)
	for {
		_, body := do(t, http.MethodGet, env.url+"/spaces?workItem=2", "")
		if body != "[]" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("queued item was not started after the restart")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, body := do(t, http.MethodGet, env.url+"/work-items/2", ""); !strings.Contains(body, `"status":"in_progress"`) {
		t.Errorf("work item after the restart = %s, want in_progress", body)
	}
}

func TestDeleteStartsTheNextQueuedItem(t *testing.T) {
	env := newLiveEnv(t)
	do(t, http.MethodPost, env.url+"/work-items", `{"projectId":1,"title":"Login","teamId":1}`)
	do(t, http.MethodPost, env.url+"/work-items", `{"projectId":1,"title":"Logout","teamId":1}`)
	do(t, http.MethodPatch, env.url+"/work-items/1", `{"status":"queued"}`)
	do(t, http.MethodPatch, env.url+"/work-items/2", `{"status":"queued"}`)
	env.waitIdle(t, 2)

	if status, body := do(t, http.MethodDelete, env.url+"/work-items/1", ""); status != 204 {
		t.Fatalf("delete = %d %s, want 204", status, body)
	}

	if _, body := do(t, http.MethodGet, env.url+"/work-items/2", ""); !strings.Contains(body, `"status":"in_progress"`) {
		t.Errorf("work item 2 after the delete = %s, want in_progress", body)
	}
	if _, body := do(t, http.MethodGet, env.url+"/spaces?workItem=2", ""); body == "[]" {
		t.Error("no Space opened for work item 2 after the delete")
	}
}

func TestPutKeepsAQueuedItemInItsQueue(t *testing.T) {
	env := newLiveEnv(t)
	do(t, http.MethodPost, env.url+"/work-items", `{"projectId":1,"title":"Login","teamId":1}`)
	do(t, http.MethodPost, env.url+"/work-items", `{"projectId":1,"title":"Logout","teamId":1}`)
	do(t, http.MethodPatch, env.url+"/work-items/1", `{"status":"queued"}`)
	do(t, http.MethodPatch, env.url+"/work-items/2", `{"status":"queued"}`)
	env.waitIdle(t, 2)

	status, body := do(t, http.MethodPut, env.url+"/work-items/2", `{"title":"Logout","teamId":null,"status":"backlog"}`)
	if want := `{"error":"invalid: work item 2 is queued and keeps its team"}`; status != 400 || body != want {
		t.Errorf("put without the team = %d %s, want 400 %s", status, body, want)
	}
	status, body = do(t, http.MethodPut, env.url+"/work-items/2", `{"title":"Sign out","teamId":1,"status":"backlog"}`)
	if status != 200 || !strings.Contains(body, `"title":"Sign out"`) ||
		!strings.Contains(body, `"status":"queued"`) || !strings.Contains(body, `"queuePosition":1`) {
		t.Errorf("put = %d %s, want 200 still queued at 1", status, body)
	}
}

func TestDropFreesTheTeamAndClosesTheSpace(t *testing.T) {
	env := newLiveEnv(t)
	do(t, http.MethodPost, env.url+"/work-items", `{"projectId":1,"title":"Login","teamId":1}`)
	do(t, http.MethodPost, env.url+"/work-items", `{"projectId":1,"title":"Logout","teamId":1}`)
	do(t, http.MethodPatch, env.url+"/work-items/1", `{"status":"queued"}`)
	do(t, http.MethodPatch, env.url+"/work-items/2", `{"status":"queued"}`)
	env.waitIdle(t, 2)

	if status, body := do(t, http.MethodPatch, env.url+"/work-items/1", `{"status":"dropped"}`); status != 200 ||
		!strings.Contains(body, `"status":"dropped"`) || !strings.Contains(body, `"closedAt":"`) {
		t.Fatalf("drop = %d %s, want 200 dropped and closed", status, body)
	}

	if _, body := do(t, http.MethodGet, env.url+"/spaces/2", ""); !strings.Contains(body, `"closedAt":"`) {
		t.Errorf("Space 2 = %s, want closedAt set", body)
	}
	if _, body := do(t, http.MethodGet, env.url+"/work-items/2", ""); !strings.Contains(body, `"status":"in_progress"`) {
		t.Errorf("work item 2 after the drop = %s, want in_progress", body)
	}
	if status, _ := do(t, http.MethodPost, env.url+"/spaces/2/reopen", ""); status != 204 {
		t.Fatalf("reopen = %d, want 204", status)
	}
	const want = `{"error":"work item 1 is dropped: conflict"}`
	if status, body := env.sendTo(t, 2, "again"); status != 409 || body != want {
		t.Errorf("send to the reopened Space = %d %s, want 409 %s", status, body, want)
	}
}
