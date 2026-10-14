package server

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestDeleteFeature_RemovesWorktreeAndCheckpointRefs(t *testing.T) {
	env, tree := rollbackEnv(t)
	gitIn(t, tree, "add", ".")
	gitIn(t, tree, "commit", "-m", "work")
	repo := gitIn(t, tree, "rev-parse", "--path-format=absolute", "--git-common-dir")
	gitIn(t, repo, "rev-parse", "--verify", "refs/reef/checkpoints/1")

	if status, body := do(t, http.MethodDelete, env.url+"/features/1", ""); status != 204 || body != "" {
		t.Fatalf("delete = %d %q, want 204", status, body)
	}

	if _, err := os.Stat(tree); !os.IsNotExist(err) {
		t.Errorf("worktree folder after delete: err = %v, want not exist", err)
	}
	if got := gitIn(t, repo, "for-each-ref", "refs/reef/checkpoints"); got != "" {
		t.Errorf("checkpoint refs = %q, want none", got)
	}
	gitIn(t, repo, "rev-parse", "--verify", "refs/heads/feature/login")
	if status, _ := do(t, http.MethodGet, env.url+"/features/1", ""); status != 404 {
		t.Errorf("GET feature after delete = %d, want 404", status)
	}
}

func TestDeleteFeature_FolderGoneRemovesRowsAndWorktree(t *testing.T) {
	env, tree := rollbackEnv(t)
	p, err := env.st.Project(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(p.Path); err != nil {
		t.Fatal(err)
	}

	if status, body := do(t, http.MethodDelete, env.url+"/features/1", ""); status != 204 || body != "" {
		t.Fatalf("delete = %d %q, want 204", status, body)
	}

	if _, err := os.Stat(tree); !os.IsNotExist(err) {
		t.Errorf("worktree folder after delete: err = %v, want not exist", err)
	}
	if status, _ := do(t, http.MethodGet, env.url+"/features/1", ""); status != 404 {
		t.Errorf("GET feature after delete = %d, want 404", status)
	}
}

func TestDeleteFeature_RefusedWhileTheTreeIsLocked(t *testing.T) {
	env, tree := rollbackEnv(t)
	unlock, err := env.h.s.trees.Lock(context.Background(), tree)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()

	status, body := do(t, http.MethodDelete, env.url+"/features/1", "")
	if want := `{"error":"` + tree + ` has a turn running: conflict"}`; status != 409 || body != want {
		t.Errorf("delete while the tree is locked = %d %s, want 409 %s", status, body, want)
	}
	if status, _ := do(t, http.MethodGet, env.url+"/features/1", ""); status != 200 {
		t.Errorf("GET feature after refused delete = %d, want 200", status)
	}
}

func TestDeleteFeature_RefusedWhileReefChatHasTheChannel(t *testing.T) {
	env, _ := rollbackEnv(t)
	lock, err := os.Create(filepath.Join(env.dir, "chat-2.lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Close() }()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}

	status, body := do(t, http.MethodDelete, env.url+"/features/1", "")
	if want := `{"error":"channel 2 is open elsewhere: conflict"}`; status != 409 || body != want {
		t.Errorf("delete while reef chat has the channel = %d %s, want 409 %s", status, body, want)
	}
	if status, _ := do(t, http.MethodGet, env.url+"/features/1", ""); status != 200 {
		t.Errorf("GET feature after refused delete = %d, want 200", status)
	}
}

func TestDeleteFeature_DirtyWorktreeRemovesNothing(t *testing.T) {
	env, tree := rollbackEnv(t)
	repo := gitIn(t, tree, "rev-parse", "--path-format=absolute", "--git-common-dir")

	status, body := do(t, http.MethodDelete, env.url+"/features/1", "")

	want := `{"error":"conflict: feature login has uncommitted changes in ` + tree +
		`; commit or discard them first"}`
	if status != 409 || strings.TrimSpace(body) != want {
		t.Fatalf("delete = %d %s, want 409 %s", status, body, want)
	}
	if got := gitIn(t, tree, "status", "--porcelain"); got != "M a.txt\n?? new.txt" {
		t.Errorf("worktree status = %q, want its changes kept", got)
	}
	gitIn(t, repo, "rev-parse", "--verify", "refs/reef/checkpoints/1")
	if status, _ := do(t, http.MethodGet, env.url+"/features/1", ""); status != 200 {
		t.Errorf("GET feature after refused delete = %d, want 200", status)
	}
}
