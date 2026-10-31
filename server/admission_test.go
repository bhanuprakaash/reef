package server

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func (e *liveEnv) count(t *testing.T, query string) int {
	t.Helper()
	var n int
	if err := e.st.DB().QueryRow(query).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// waitCount loops until query counts want, never longer than the deadline.
func (e *liveEnv) waitCount(t *testing.T, query string, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for e.count(t, query) != want {
		if time.Now().After(deadline) {
			t.Fatalf("%s = %d, want %d", query, e.count(t, query), want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestTwoApprovedItemsOfOneOwnerRunAtTheSameTime(t *testing.T) {
	env := newLiveEnv(t)
	for i, key := range []string{"k1", "k2"} {
		do(t, http.MethodPost, env.url+"/work-items", `{"projectId":1,"title":"Item","ownerBotId":1}`)
		do(t, http.MethodPut, env.url+"/work-items/"+string(rune('1'+i))+"/brief", `{"body":"block","author":"user"}`)
		do(t, http.MethodPost, env.url+"/work-items/"+string(rune('1'+i))+"/approve", `{"key":"`+key+`"}`)
	}
	<-env.fake.started
	<-env.fake.started

	for _, id := range []int64{2, 3} {
		if l := env.h.s.lookup(id); l == nil || !l.running() {
			t.Errorf("chain of Space %d is not running while both items run", id)
		}
	}
	if n := env.count(t, "SELECT COUNT(*) FROM work_item WHERE status = 'in_progress'"); n != 2 {
		t.Errorf("items in progress = %d, want 2", n)
	}
	env.fake.release <- struct{}{}
	env.fake.release <- struct{}{}
	env.waitIdle(t, 2)
	env.waitIdle(t, 3)
}

func TestItemWithNoReadyRepositoryWaitsInNeedsYouAndOthersStart(t *testing.T) {
	env := newLiveEnv(t)
	bare := t.TempDir()
	do(t, http.MethodPost, env.url+"/projects", `{"name":"bare","path":"`+bare+`"}`)
	for i, project := range []string{"2", "1"} {
		do(t, http.MethodPost, env.url+"/work-items", `{"projectId":`+project+`,"title":"Item","ownerBotId":1}`)
		do(t, http.MethodPost, env.url+"/work-items/"+string(rune('1'+i))+"/approve", `{"key":"k`+string(rune('1'+i))+`"}`)
	}
	env.waitCount(t, "SELECT COUNT(*) FROM work_item WHERE id = 1 AND status = 'needs_you'", 1)
	env.waitCount(t, "SELECT COUNT(*) FROM message WHERE space_id = 3 AND author != 'reef'", 1)
	env.waitIdle(t, 3)

	if _, body := do(t, http.MethodGet, env.url+"/work-items/1", ""); !strings.Contains(body, `"status":"needs_you"`) {
		t.Errorf("item with no ready repository = %s, want needs_you", body)
	}
	if _, body := do(t, http.MethodGet, env.url+"/work-items/2", ""); !strings.Contains(body, `"status":"in_progress"`) {
		t.Errorf("next item = %s, want in_progress", body)
	}
	if _, body := do(t, http.MethodGet, env.url+"/messages?space=2", ""); !strings.Contains(body, "bare: ") ||
		!strings.Contains(body, "Not started: no repository of this Work item is ready") {
		t.Errorf("messages of the blocked Space = %s, want a note naming the repository", body)
	}
	if n := env.count(t, "SELECT COUNT(*) FROM message WHERE space_id = 2 AND author != 'reef'"); n != 0 {
		t.Errorf("prompts in the blocked Space = %d, want 0", n)
	}

	gitIn(t, bare, "init", "-b", "main")
	writeIn(t, bare, "README", "x")
	gitIn(t, bare, "add", ".")
	gitIn(t, bare, "commit", "-m", "init")
	if status, body := do(t, http.MethodPost, env.url+"/spaces/2/continue", ""); status != 204 {
		t.Fatalf("continue = %d %s, want 204", status, body)
	}
	env.waitCount(t, "SELECT COUNT(*) FROM message WHERE space_id = 2 AND author != 'reef'", 1)
	env.waitIdle(t, 2)
}

func TestItemWhoseBranchIsTakenWaitsInNeedsYouWithTheError(t *testing.T) {
	env := newLiveEnv(t)
	p, err := env.st.Project(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	gitIn(t, p.Path, "worktree", "add", filepath.Join(t.TempDir(), "other"), "-b", "work-item/1")
	do(t, http.MethodPost, env.url+"/work-items", `{"projectId":1,"title":"Item","ownerBotId":1}`)
	do(t, http.MethodPost, env.url+"/work-items/1/approve", `{"key":"k1"}`)
	env.waitCount(t, "SELECT COUNT(*) FROM work_item WHERE id = 1 AND status = 'needs_you'", 1)

	if _, body := do(t, http.MethodGet, env.url+"/messages?space=2", ""); !strings.Contains(body, "reef: ") ||
		!strings.Contains(body, "checked out") {
		t.Errorf("messages = %s, want a note naming the repository and the error", body)
	}
	if _, body := do(t, http.MethodGet, env.url+"/work-items/1", ""); !strings.Contains(body, `"status":"needs_you"`) {
		t.Errorf("item = %s, want needs_you", body)
	}
}

func TestRestartAfterTheClaimOpensTheSpaceOnce(t *testing.T) {
	env := newLiveEnv(t)
	do(t, http.MethodPost, env.url+"/work-items", `{"projectId":1,"title":"Login","ownerBotId":1}`)
	env.exec(t, "UPDATE work_item SET status = 'in_progress', approved_at = '2026-01-01T00:00:00Z', approval_key = 'k' WHERE id = 1")
	env.restart(t)
	env.waitCount(t, "SELECT COUNT(*) FROM message WHERE space_id = 2 AND author != 'reef'", 1)
	env.waitIdle(t, 2)
	env.restart(t)
	env.h.s.openStarted(context.Background())

	if n := env.count(t, "SELECT COUNT(*) FROM space WHERE work_item_id = 1"); n != 1 {
		t.Errorf("Spaces of the item = %d, want 1", n)
	}
	if n := env.count(t, "SELECT COUNT(*) FROM message WHERE space_id = 2 AND author != 'reef'"); n != 1 {
		t.Errorf("prompts in the Space = %d, want 1", n)
	}
}

func TestRestartStartsAnApprovedItemStillQueued(t *testing.T) {
	env := newLiveEnv(t)
	do(t, http.MethodPost, env.url+"/work-items", `{"projectId":1,"title":"Login","ownerBotId":1}`)
	env.exec(t, "UPDATE work_item SET status = 'queued', queue_position = 1, approved_at = '2026-01-01T00:00:00Z', approval_key = 'k' WHERE id = 1")
	env.restart(t)
	env.waitCount(t, "SELECT COUNT(*) FROM message WHERE space_id = 2 AND author != 'reef'", 1)
	env.waitIdle(t, 2)

	if _, body := do(t, http.MethodGet, env.url+"/work-items/1", ""); !strings.Contains(body, `"status":"in_progress"`) {
		t.Errorf("item after the restart = %s, want in_progress", body)
	}
}

func TestRestartAfterTheSpaceStartsTheFirstTurnOnce(t *testing.T) {
	env := newLiveEnv(t)
	do(t, http.MethodPost, env.url+"/work-items", `{"projectId":1,"title":"Login","ownerBotId":1}`)
	do(t, http.MethodPost, env.url+"/spaces", `{"projectId":1,"workItemId":1,"name":"by hand"}`)
	env.exec(t, "UPDATE work_item SET status = 'in_progress', approved_at = '2026-01-01T00:00:00Z', approval_key = 'k' WHERE id = 1")
	env.restart(t)
	env.waitCount(t, "SELECT COUNT(*) FROM message WHERE space_id = 2 AND author != 'reef'", 1)
	env.waitIdle(t, 2)
	env.restart(t)
	env.h.s.openStarted(context.Background())

	if n := env.count(t, "SELECT COUNT(*) FROM message WHERE space_id = 2 AND author != 'reef'"); n != 1 {
		t.Errorf("prompts in the Space = %d, want 1", n)
	}
}

func TestRestartKeepsAPausedSpaceAndItsPendingFirstTurnPaused(t *testing.T) {
	env := newLiveEnv(t)
	do(t, http.MethodPost, env.url+"/work-items", `{"projectId":1,"title":"Login","ownerBotId":1}`)
	do(t, http.MethodPost, env.url+"/spaces", `{"projectId":1,"workItemId":1,"name":"by hand"}`)
	env.exec(t, "UPDATE work_item SET status = 'in_progress', approved_at = '2026-01-01T00:00:00Z', approval_key = 'k' WHERE id = 1")
	env.exec(t, "UPDATE space SET paused = 'paused by the user' WHERE id = 2")
	env.restart(t)
	env.h.s.openStarted(context.Background())

	if n := env.count(t, "SELECT COUNT(*) FROM message WHERE space_id = 2 AND author != 'reef'"); n != 0 {
		t.Errorf("prompts in the paused Space = %d, want 0", n)
	}
	if _, body := do(t, http.MethodGet, env.url+"/spaces/2", ""); !strings.Contains(body, `"paused":"paused by the user"`) {
		t.Errorf("Space after the restart = %s, want it still paused by the user", body)
	}
}
