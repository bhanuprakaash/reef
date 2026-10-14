package server

import (
	"net/http"
	"os"
	"strings"
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
