package server

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/bhanuprakaash/reef/memory"
	"github.com/bhanuprakaash/reef/space"
)

// seedMemory saves two accepted items and one candidate in scope through the server and
// returns their ids, candidate last.
func seedMemory(t *testing.T, env *liveEnv, scope string) []int64 {
	t.Helper()
	for _, body := range []string{"sessions, not JWT", "tabs"} {
		if status, out := do(t, http.MethodPost, env.url+"/memory",
			`{"kind":"fact","body":"`+body+`","scope":"`+scope+`"}`); status != 201 {
			t.Fatalf("create %q = %d %s", body, status, out)
		}
	}
	kind, id, _ := strings.Cut(scope, ":")
	res, err := env.h.s.ms.Remember(context.Background(),
		memory.Writer{Author: "ada", Role: "developer", Scope: memory.Ref{Kind: kind, ID: id}},
		memory.Note{Kind: memory.KindFact, Body: "use sqlite", NeedsApproval: true})
	if err != nil {
		t.Fatal(err)
	}
	return []int64{res.Item.ID - 2, res.Item.ID - 1, res.Item.ID}
}

func wantStatuses(t *testing.T, env *liveEnv, ids []int64, want ...string) {
	t.Helper()
	for i, id := range ids {
		it, err := env.h.s.ms.Read(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if it.Status != want[i] {
			t.Errorf("item %d (%s) status = %q, want %q", id, it.Body, it.Status, want[i])
		}
	}
}

func TestDeleteWorkItem_RemovesWorktreeAndCheckpointRefs(t *testing.T) {
	env, tree := rollbackEnv(t)
	gitIn(t, tree, "add", ".")
	gitIn(t, tree, "commit", "-m", "work")
	repo := gitIn(t, tree, "rev-parse", "--path-format=absolute", "--git-common-dir")
	gitIn(t, repo, "rev-parse", "--verify", "refs/reef/checkpoints/1")

	if status, body := do(t, http.MethodDelete, env.url+"/work-items/1", ""); status != 204 || body != "" {
		t.Fatalf("delete = %d %q, want 204", status, body)
	}

	if _, err := os.Stat(tree); !os.IsNotExist(err) {
		t.Errorf("worktree folder after delete: err = %v, want not exist", err)
	}
	if got := gitIn(t, repo, "for-each-ref", "refs/reef/checkpoints"); got != "" {
		t.Errorf("checkpoint refs = %q, want none", got)
	}
	gitIn(t, repo, "rev-parse", "--verify", "refs/heads/work-item/1")
	if status, _ := do(t, http.MethodGet, env.url+"/work-items/1", ""); status != 404 {
		t.Errorf("GET work item after delete = %d, want 404", status)
	}
}

func TestDeleteWorkItem_RetiresItsAndItsSubItemsMemoryItems(t *testing.T) {
	env, tree := rollbackEnv(t)
	gitIn(t, tree, "add", ".")
	gitIn(t, tree, "commit", "-m", "work")
	do(t, http.MethodPost, env.url+"/work-items", `{"projectId":2,"parentId":1,"type":"task","title":"Sub"}`)
	mine := seedMemory(t, env, "work_item:1")
	sub := seedMemory(t, env, "work_item:2")
	other := seedMemory(t, env, "work_item:3")

	if status, body := do(t, http.MethodDelete, env.url+"/work-items/1", ""); status != 204 || body != "" {
		t.Fatalf("delete = %d %q, want 204", status, body)
	}

	wantStatuses(t, env, mine, "retired", "retired", "rejected")
	wantStatuses(t, env, sub, "retired", "retired", "rejected")
	wantStatuses(t, env, other, "accepted", "accepted", "candidate")
	pending, err := env.h.s.ms.Pending(context.Background())
	if err != nil || len(pending) != 1 || pending[0].ID != other[2] {
		t.Errorf("pending = %+v, %v; want only item %d", pending, err, other[2])
	}
}

func TestDeleteWorkItem_FolderGoneRemovesRowsAndWorktree(t *testing.T) {
	env, tree := rollbackEnv(t)
	p, err := env.st.Project(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(p.Path); err != nil {
		t.Fatal(err)
	}

	if status, body := do(t, http.MethodDelete, env.url+"/work-items/1", ""); status != 204 || body != "" {
		t.Fatalf("delete = %d %q, want 204", status, body)
	}

	if _, err := os.Stat(tree); !os.IsNotExist(err) {
		t.Errorf("worktree folder after delete: err = %v, want not exist", err)
	}
	if status, _ := do(t, http.MethodGet, env.url+"/work-items/1", ""); status != 404 {
		t.Errorf("GET work item after delete = %d, want 404", status)
	}
}

func TestDeleteWorkItem_RefusedWhileTheTreeIsLocked(t *testing.T) {
	env, _ := rollbackEnv(t)
	root := space.WorkRoot(env.dir, "1")
	unlock, err := env.h.s.trees.Lock(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()

	status, body := do(t, http.MethodDelete, env.url+"/work-items/1", "")
	if want := `{"error":"` + root + ` has a turn running: conflict"}`; status != 409 || body != want {
		t.Errorf("delete while the tree is locked = %d %s, want 409 %s", status, body, want)
	}
	if status, _ := do(t, http.MethodGet, env.url+"/work-items/1", ""); status != 200 {
		t.Errorf("GET work item after refused delete = %d, want 200", status)
	}
}

func TestDeleteWorkItem_RefusedWhileReefChatHasTheSpace(t *testing.T) {
	env, _ := rollbackEnv(t)
	lock, err := os.Create(filepath.Join(env.dir, "chat-2.lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Close() }()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}

	status, body := do(t, http.MethodDelete, env.url+"/work-items/1", "")
	if want := `{"error":"space 2 is open elsewhere: conflict"}`; status != 409 || body != want {
		t.Errorf("delete while reef chat has the Space = %d %s, want 409 %s", status, body, want)
	}
	if status, _ := do(t, http.MethodGet, env.url+"/work-items/1", ""); status != 200 {
		t.Errorf("GET work item after refused delete = %d, want 200", status)
	}
}

func TestDeleteWorkItem_DirtyWorktreeRemovesNothing(t *testing.T) {
	env, tree := rollbackEnv(t)
	repo := gitIn(t, tree, "rev-parse", "--path-format=absolute", "--git-common-dir")

	status, body := do(t, http.MethodDelete, env.url+"/work-items/1", "")

	want := `{"error":"conflict: work item 1 has uncommitted changes in ` + tree +
		`; commit or discard them first"}`
	if status != 409 || strings.TrimSpace(body) != want {
		t.Fatalf("delete = %d %s, want 409 %s", status, body, want)
	}
	if got := gitIn(t, tree, "status", "--porcelain"); got != "M a.txt\n?? new.txt" {
		t.Errorf("worktree status = %q, want its changes kept", got)
	}
	gitIn(t, repo, "rev-parse", "--verify", "refs/reef/checkpoints/1")
	if status, _ := do(t, http.MethodGet, env.url+"/work-items/1", ""); status != 200 {
		t.Errorf("GET work item after refused delete = %d, want 200", status)
	}
}
