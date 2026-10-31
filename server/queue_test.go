package server

import (
	"context"
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
		`{"projectId":1,"title":"Login","description":"email and password",`+
			`"acceptanceLines":[{"text":"wrong password is refused"},{"text":"session lasts a day"}]}`)
	do(t, http.MethodPut, env.url+"/work-items/1/brief", `{"body":"sessions, not JWT","author":"user"}`)

	if status, body := do(t, http.MethodPost, env.url+"/work-items/1/approve", `{"key":"k1"}`); status != 400 ||
		!strings.Contains(body, "needs an owner before it is approved") {
		t.Fatalf("queue without an owner = %d %s, want 400", status, body)
	}
	do(t, http.MethodPut, env.url+"/work-items/1/participants", `{"ownerBotId":1,"participantBotIds":[1]}`)
	if status, body := do(t, http.MethodPost, env.url+"/work-items/1/approve", `{"key":"k1"}`); status != 200 ||
		!strings.Contains(body, `"status":"in_progress"`) {
		t.Fatalf("queue the item = %d %s, want 200 in_progress", status, body)
	}
	env.waitCount(t, "SELECT COUNT(*) FROM message WHERE space_id = 2 AND author != 'reef'", 1)
	env.waitIdle(t, 2)

	want := "Work item 1: Login\nDescription: email and password\nBrief: sessions, not JWT"
	if got := env.lastRan(t); !strings.HasSuffix(got, want) {
		t.Errorf("first prompt = %q, want it to end with %q", got, want)
	}
	if got := groupKinds(env.inboxItems(t)); !slices.Equal(got, []string{"fyi started"}) {
		t.Errorf("inbox = %q, want the started item", got)
	}
}

func TestQueuedItemWithAnAttachedSpaceStartsInIt(t *testing.T) {
	env := newLiveEnv(t)
	do(t, http.MethodPost, env.url+"/work-items", `{"projectId":1,"title":"Login","ownerBotId":1}`)
	if status, body := do(t, http.MethodPost, env.url+"/spaces",
		`{"projectId":1,"workItemId":1,"name":"by hand"}`); status != 201 {
		t.Fatalf("create Space = %d %s, want 201", status, body)
	}

	do(t, http.MethodPost, env.url+"/work-items/1/approve", `{"key":"k1"}`)
	env.waitCount(t, "SELECT COUNT(*) FROM message WHERE space_id = 2 AND author != 'reef'", 1)
	env.waitIdle(t, 2)

	want := "Work item 1: Login\nDescription: none\nBrief: none"
	if got := env.lastRan(t); !strings.HasSuffix(got, want) {
		t.Errorf("turn in the attached Space = %q, want it to end with %q", got, want)
	}
	env.fake.mu.Lock()
	before := len(env.fake.ran)
	env.fake.mu.Unlock()
	env.h.s.openStarted(context.Background())
	env.waitIdle(t, 2)
	env.fake.mu.Lock()
	after := len(env.fake.ran)
	env.fake.mu.Unlock()
	if after != before {
		t.Errorf("turns after a second open = %d, want the %d it had", after, before)
	}
}

func TestQueuedItemStartsAfterRestart(t *testing.T) {
	env := newLiveEnv(t)
	do(t, http.MethodPost, env.url+"/work-items", `{"projectId":1,"title":"Login","ownerBotId":1}`)
	env.exec(t, "UPDATE work_item SET status = 'queued', queue_position = 1, approved_at = '2026-01-01T00:00:00Z' WHERE id = 1")
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

func TestStartedItemSpaceIsLedByTheOwnerWithTheParticipants(t *testing.T) {
	env := newLiveEnv(t)
	do(t, http.MethodPost, env.url+"/bots", `{"name":"ann","backend":"claude"}`)
	do(t, http.MethodPost, env.url+"/work-items", `{"projectId":1,"title":"Login"}`)
	do(t, http.MethodPut, env.url+"/work-items/1/participants", `{"ownerBotId":2,"participantBotIds":[1]}`)
	do(t, http.MethodPost, env.url+"/work-items/1/approve", `{"key":"k1"}`)
	env.waitCount(t, "SELECT COUNT(*) FROM message WHERE space_id = 2 AND author != 'reef'", 1)
	env.waitIdle(t, 2)

	_, body := do(t, http.MethodGet, env.url+"/spaces/2", "")
	if !strings.Contains(body, `"role":"ann"`) {
		t.Errorf("Space of the item = %s, want ann as its lead", body)
	}
	var ids string
	if err := env.st.DB().QueryRow(
		"SELECT group_concat(bot_id || ':' || lead) FROM (SELECT bot_id, lead FROM space_member WHERE space_id = 2 ORDER BY bot_id)").
		Scan(&ids); err != nil || ids != "1:0,2:1" {
		t.Errorf("members of the Space = %q, %v, want 1:0,2:1", ids, err)
	}
}

func TestPutCannotChangeTheOwnerOfAStartedItem(t *testing.T) {
	env := newLiveEnv(t)
	do(t, http.MethodPost, env.url+"/work-items", `{"projectId":1,"title":"Login","ownerBotId":1}`)
	do(t, http.MethodPost, env.url+"/work-items/1/approve", `{"key":"k1"}`)
	env.waitCount(t, "SELECT COUNT(*) FROM message WHERE space_id = 2 AND author != 'reef'", 1)
	env.waitIdle(t, 2)

	status, body := do(t, http.MethodPut, env.url+"/work-items/1", `{"title":"Login","ownerBotId":null,"participantBotIds":[]}`)
	if want := `{"error":"invalid: work item 1 keeps its owner and participants"}`; status != 400 || body != want {
		t.Errorf("put without the owner = %d %s, want 400 %s", status, body, want)
	}
	status, body = do(t, http.MethodPut, env.url+"/work-items/1", `{"title":"Sign in"}`)
	if status != 200 || !strings.Contains(body, `"title":"Sign in"`) || !strings.Contains(body, `"ownerBotId":1`) {
		t.Errorf("put = %d %s, want 200 with the owner kept", status, body)
	}
}

func TestDropClosesTheSpace(t *testing.T) {
	env := newLiveEnv(t)
	do(t, http.MethodPost, env.url+"/work-items", `{"projectId":1,"title":"Login","ownerBotId":1}`)
	do(t, http.MethodPost, env.url+"/work-items/1/approve", `{"key":"k1"}`)
	env.waitCount(t, "SELECT COUNT(*) FROM message WHERE space_id = 2 AND author != 'reef'", 1)
	env.waitIdle(t, 2)

	if status, body := do(t, http.MethodPatch, env.url+"/work-items/1", `{"status":"dropped"}`); status != 200 ||
		!strings.Contains(body, `"status":"dropped"`) || !strings.Contains(body, `"closedAt":"`) {
		t.Fatalf("drop = %d %s, want 200 dropped and closed", status, body)
	}

	if _, body := do(t, http.MethodGet, env.url+"/spaces/2", ""); !strings.Contains(body, `"closedAt":"`) {
		t.Errorf("Space 2 = %s, want closedAt set", body)
	}
	if status, _ := do(t, http.MethodPost, env.url+"/spaces/2/reopen", ""); status != 204 {
		t.Fatalf("reopen = %d, want 204", status)
	}
	const want = `{"error":"work item 1 is dropped: conflict"}`
	if status, body := env.sendTo(t, 2, "again"); status != 409 || body != want {
		t.Errorf("send to the reopened Space = %d %s, want 409 %s", status, body, want)
	}
}
