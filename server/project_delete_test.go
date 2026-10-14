package server

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/store"
	"github.com/bhanuprakaash/reef/worktree"
)

// projectTrees extends rollbackEnv (project 2, feature 1 "login", worktree login, ref
// checkpoints/1) with feature 2 "signup" and a clean worktree whose turn has ref
// checkpoints/2, and a quick-session channel whose turn has ref checkpoints/3 taken in the
// main checkout. It returns the main checkout's .git folder and signup's worktree.
func projectTrees(t *testing.T, env *liveEnv, login string) (repo, signup string) {
	t.Helper()
	ctx := context.Background()
	repo = gitIn(t, login, "rev-parse", "--path-format=absolute", "--git-common-dir")
	main := filepath.Dir(repo)
	do(t, http.MethodPost, env.url+"/features", `{"projectId":2,"slug":"signup","name":"Signup"}`)
	do(t, http.MethodPost, env.url+"/channels", `{"projectId":2,"featureId":2,"name":"s","backend":"claude"}`)
	do(t, http.MethodPost, env.url+"/channels", `{"projectId":2,"name":"quick","backend":"claude"}`)
	signup = worktree.Path(env.dir, 2, "signup")
	if err := worktree.Ensure(ctx, main, signup, worktree.Branch("signup")); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		channel int64
		dir     string
	}{{3, signup}, {4, main}} {
		turn, err := env.st.CreateTurn(ctx, store.Turn{ChannelID: c.channel, Role: "lead", Backend: "claude"})
		if err != nil {
			t.Fatal(err)
		}
		ref := "refs/reef/checkpoints/" + strconv.FormatInt(turn.ID, 10)
		if err := worktree.Checkpoint(ctx, c.dir, ref); err != nil {
			t.Fatal(err)
		}
		if _, err := env.st.SetTurnCheckpoint(ctx, turn.ID, ref); err != nil {
			t.Fatal(err)
		}
	}
	return repo, signup
}

func TestDeleteProject_RemovesWorktreesAndCheckpointRefs(t *testing.T) {
	env, login := rollbackEnv(t)
	repo, signup := projectTrees(t, env, login)
	gitIn(t, login, "add", ".")
	gitIn(t, login, "commit", "-m", "work")

	if status, body := do(t, http.MethodDelete, env.url+"/projects/2", ""); status != 204 || body != "" {
		t.Fatalf("delete = %d %q, want 204", status, body)
	}

	for _, dir := range []string{login, signup} {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("worktree %s after delete: err = %v, want not exist", dir, err)
		}
	}
	if got := gitIn(t, repo, "for-each-ref", "refs/reef/checkpoints"); got != "" {
		t.Errorf("checkpoint refs = %q, want none", got)
	}
	for _, branch := range []string{"feature/login", "feature/signup"} {
		gitIn(t, repo, "rev-parse", "--verify", "refs/heads/"+branch)
	}
	if status, _ := do(t, http.MethodGet, env.url+"/projects/2", ""); status != 404 {
		t.Errorf("GET project after delete = %d, want 404", status)
	}
}

func TestDeleteProject_DirtyWorktreeRemovesNothing(t *testing.T) {
	env, login := rollbackEnv(t)
	repo, signup := projectTrees(t, env, login)
	gitIn(t, login, "add", ".")
	gitIn(t, login, "commit", "-m", "work")
	writeIn(t, signup, "new.txt", "n")

	status, body := do(t, http.MethodDelete, env.url+"/projects/2", "")

	want := `{"error":"conflict: feature signup has uncommitted changes in ` + signup +
		`; commit or discard them first"}`
	if status != 409 || strings.TrimSpace(body) != want {
		t.Fatalf("delete = %d %s, want 409 %s", status, body, want)
	}
	for _, dir := range []string{login, signup} {
		if _, err := os.Stat(dir); err != nil {
			t.Errorf("worktree %s after refused delete: %v, want kept", dir, err)
		}
	}
	got := gitIn(t, repo, "for-each-ref", "--format=%(refname)", "refs/reef/checkpoints")
	if got != "refs/reef/checkpoints/1\nrefs/reef/checkpoints/2\nrefs/reef/checkpoints/3" {
		t.Errorf("checkpoint refs = %q, want all three", got)
	}
	if status, _ := do(t, http.MethodGet, env.url+"/projects/2", ""); status != 200 {
		t.Errorf("GET project after refused delete = %d, want 200", status)
	}
}
