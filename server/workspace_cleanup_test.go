package server

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/bhanuprakaash/reef/store"
)

// cleanupEnv is Work item 1 over two repositories, each with its workspace and one request:
// the GitHub pull request #1 of App and the GitLab merge request !2 of Lib. App's workspace
// holds a staged file, an unstaged edit, an untracked file and a commit no remote has.
func cleanupEnv(t *testing.T) (env *liveEnv, app, lib store.Workspace, repoApp string, answer func(bin, body, stderr string)) {
	t.Helper()
	ctx := context.Background()
	env = newLiveEnv(t)
	answer = fakeCLIs(t)
	repoApp, _ = originRepo(t, "develop")
	repoLib, _ := originRepo(t, "develop")
	addProject(t, env.url, 2, "App", repoApp, "develop")
	addProject(t, env.url, 3, "Lib", repoLib, "develop")
	do(t, http.MethodPost, env.url+"/work-items", `{"projectId":2,"title":"Demo"}`)
	do(t, http.MethodPost, env.url+"/work-items/1/targets", `{"projectId":3}`)
	ws := workspaces(t, http.MethodPost, env.url+"/work-items/1/workspaces")
	if len(ws) != 2 {
		t.Fatalf("workspaces = %+v, want 2", ws)
	}
	app, lib = ws[0], ws[1]
	one := int64(1)
	for _, r := range []store.RepoRequest{
		{Provider: store.RepoGitHub, Host: "github.com", Path: "x/app", Number: 1, URL: "https://github.com/x/app/pull/1", WorkspaceID: app.ID},
		{Provider: store.RepoGitLab, Host: "gitlab.com", Path: "x/lib", Number: 2, URL: "https://gitlab.com/x/lib/-/merge_requests/2", WorkspaceID: lib.ID},
	} {
		r.State, r.Head, r.Base, r.WorkItemID = store.RequestOpened, "work-item/1", "develop", &one
		if _, err := env.st.PutRepoRequest(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	writeIn(t, app.Dir, "committed.txt", "c")
	gitIn(t, app.Dir, "add", ".")
	gitIn(t, app.Dir, "commit", "-m", "unpushed")
	writeIn(t, app.Dir, "staged.txt", "s")
	gitIn(t, app.Dir, "add", "staged.txt")
	writeIn(t, app.Dir, "a.txt", "edited")
	writeIn(t, app.Dir, "untracked.txt", "u")
	return env, app, lib, repoApp, answer
}

func workspaceRow(t *testing.T, env *liveEnv, id int64) store.Workspace {
	t.Helper()
	w, err := env.st.Workspace(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func TestCleanup_MergedRequestRemovesOnlyItsWorkspaceAndKeepsTheChanges(t *testing.T) {
	ctx := context.Background()
	env, app, lib, repo, answer := cleanupEnv(t)
	tip := gitIn(t, app.Dir, "rev-parse", "HEAD")
	answer("gh", ghAnswer("MERGED", "", "SUCCESS", "1"), "")
	answer("glab", `{"state":"closed"}`, "")

	env.h.s.trackRepoRequests(ctx)

	got := workspaceRow(t, env, app.ID)
	if got.State != store.WorkspaceRemoved || got.CleanupError != "" || got.SnapshotRef == "" {
		t.Fatalf("merged workspace = %+v, want removed with a snapshot ref and no error", got)
	}
	if _, err := os.Stat(app.Dir); !os.IsNotExist(err) {
		t.Errorf("merged workspace folder: %v, want gone", err)
	}
	if got := workspaceRow(t, env, lib.ID); got.State != store.WorkspaceReady {
		t.Errorf("closed-unmerged workspace state = %s, want ready", got.State)
	}
	if _, err := os.Stat(lib.Dir); err != nil {
		t.Errorf("closed-unmerged workspace folder: %v, want kept", err)
	}
	for file, want := range map[string]string{"staged.txt": "s", "a.txt": "edited", "untracked.txt": "u", "committed.txt": "c"} {
		if out := gitIn(t, repo, "show", got.SnapshotRef+":"+file); out != want {
			t.Errorf("snapshot %s = %q, want %q", file, out, want)
		}
	}
	if out := gitIn(t, repo, "show", got.SnapshotRef+"^2:staged.txt"); out != "s" {
		t.Errorf("staged snapshot staged.txt = %q, want s", out)
	}
	if out := gitIn(t, repo, "show", got.SnapshotRef+"^2:a.txt"); out != "a" {
		t.Errorf("staged snapshot a.txt = %q, want the unedited a", out)
	}
	if out := gitIn(t, repo, "rev-parse", got.SnapshotRef+"^1"); out != tip {
		t.Errorf("snapshot parent = %s, want the unpushed tip %s", out, tip)
	}
	if out := gitIn(t, repo, "rev-parse", app.Branch); out != tip {
		t.Errorf("branch %s = %s, want the unpushed tip %s", app.Branch, out, tip)
	}

	env.h.s.trackRepoRequests(ctx)
	if again := workspaceRow(t, env, app.ID); again.SnapshotRef != got.SnapshotRef || !again.UpdatedAt.Equal(got.UpdatedAt) {
		t.Errorf("second read changed the removed workspace: %+v, was %+v", again, got)
	}
}

func TestCleanup_FailedSaveKeepsTheWorkspaceAndIsRetried(t *testing.T) {
	ctx := context.Background()
	env, app, _, repo, answer := cleanupEnv(t)
	answer("gh", ghAnswer("MERGED", "", "SUCCESS", "1"), "")
	answer("glab", `{"state":"opened"}`, "")
	refs := filepath.Join(repo, ".git", "refs")
	if err := os.Chmod(refs, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(refs, 0o755) })

	env.h.s.trackRepoRequests(ctx)

	got := workspaceRow(t, env, app.ID)
	if got.State != store.WorkspaceReady || got.CleanupError == "" || got.SnapshotRef != "" {
		t.Fatalf("workspace after a failed save = %+v, want ready with a cleanup error and no snapshot", got)
	}
	if out := gitIn(t, app.Dir, "status", "--porcelain"); out == "" {
		t.Error("changes are gone from the kept workspace")
	}

	if err := os.Chmod(refs, 0o755); err != nil {
		t.Fatal(err)
	}
	env.h.s.trackRepoRequests(ctx)

	got = workspaceRow(t, env, app.ID)
	if got.State != store.WorkspaceRemoved || got.CleanupError != "" || got.SnapshotRef == "" {
		t.Errorf("workspace after the retry = %+v, want removed with a snapshot ref and no error", got)
	}
}

func TestCleanup_WaitsForARunningWriter(t *testing.T) {
	ctx := context.Background()
	env, app, _, _, answer := cleanupEnv(t)
	answer("gh", ghAnswer("MERGED", "", "SUCCESS", "1"), "")
	answer("glab", `{"state":"opened"}`, "")
	unlock, ok := env.h.s.trees.TryLock(env.h.s.writerKey(app))
	if !ok {
		t.Fatal("work item root is already locked")
	}

	env.h.s.trackRepoRequests(ctx)

	if got := workspaceRow(t, env, app.ID); got.State != store.WorkspaceReady || got.CleanupError == "" {
		t.Errorf("workspace under a writer = %+v, want ready with a cleanup error", got)
	}
	unlock()
	env.h.s.trackRepoRequests(ctx)
	if got := workspaceRow(t, env, app.ID); got.State != store.WorkspaceRemoved || got.CleanupError != "" {
		t.Errorf("workspace after the writer ended = %+v, want removed", got)
	}
}
