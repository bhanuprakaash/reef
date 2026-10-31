package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/store"
)

// originRepo is a repo whose origin is a bare repo, with main and a branch base that has one
// more commit than main, both pushed.
func originRepo(t *testing.T, base string) (repo, remote string) {
	t.Helper()
	remote = t.TempDir()
	gitIn(t, remote, "init", "--bare", "-b", "main")
	repo = t.TempDir()
	gitIn(t, repo, "init", "-b", "main")
	writeIn(t, repo, "a.txt", "a")
	gitIn(t, repo, "add", ".")
	gitIn(t, repo, "commit", "-m", "first")
	gitIn(t, repo, "remote", "add", "origin", remote)
	gitIn(t, repo, "push", "origin", "main")
	gitIn(t, repo, "checkout", "-b", base)
	writeIn(t, repo, base+".txt", base)
	gitIn(t, repo, "add", ".")
	gitIn(t, repo, "commit", "-m", base)
	gitIn(t, repo, "push", "origin", base)
	gitIn(t, repo, "checkout", "main")
	return repo, remote
}

// addProject adds the project that becomes number id and sets its base branch.
func addProject(t *testing.T, url string, id int, name, path, base string) {
	t.Helper()
	body := fmt.Sprintf(`{"name":%q,"path":%q,"baseBranch":%q}`, name, path, base)
	if status, out := do(t, http.MethodPost, url+"/projects", body); status != 201 {
		t.Fatalf("add project = %d %s", status, out)
	}
	if status, out := do(t, http.MethodPut, fmt.Sprintf("%s/projects/%d", url, id), body); status != 200 {
		t.Fatalf("set base branch = %d %s", status, out)
	}
}

func workspaces(t *testing.T, method, url string) []store.Workspace {
	t.Helper()
	status, body := do(t, method, url, "")
	if status != 200 {
		t.Fatalf("%s %s = %d %s, want 200", method, url, status, body)
	}
	var ws []store.Workspace
	if err := json.Unmarshal([]byte(body), &ws); err != nil {
		t.Fatal(err)
	}
	return ws
}

func TestWorkspaces_OneBranchPerRepositoryFromTheProjectBase(t *testing.T) {
	ts := newTestServer(t)
	repoA, remoteA := originRepo(t, "develop")
	repoB, _ := originRepo(t, "develop")
	addProject(t, ts.URL, 1, "App", repoA, "develop")
	addProject(t, ts.URL, 2, "Lib", repoB, "develop")
	do(t, http.MethodPost, ts.URL+"/work-items", `{"projectId":1,"title":"Demo"}`)
	do(t, http.MethodPost, ts.URL+"/work-items/1/targets", `{"projectId":2}`)

	other := t.TempDir()
	gitIn(t, other, "clone", "-b", "develop", remoteA, ".")
	writeIn(t, other, "newer.txt", "n")
	gitIn(t, other, "add", ".")
	gitIn(t, other, "commit", "-m", "newer")
	gitIn(t, other, "push", "origin", "develop")
	want := gitIn(t, other, "rev-parse", "HEAD")

	ws := workspaces(t, http.MethodPost, ts.URL+"/work-items/1/workspaces")
	if len(ws) != 2 {
		t.Fatalf("workspaces = %d, want 2", len(ws))
	}
	for i, w := range ws {
		if w.State != "ready" || w.BaseSource != "origin" || w.BaseRef != "refs/remotes/origin/develop" || w.Note != "" {
			t.Errorf("workspace %d = %s %s %s %q, want ready origin refs/remotes/origin/develop and no note",
				i, w.State, w.BaseSource, w.BaseRef, w.Note)
		}
		if got := gitIn(t, w.Dir, "branch", "--show-current"); got != "work-item/1" {
			t.Errorf("workspace %d branch = %q, want work-item/1", i, got)
		}
		if !strings.HasSuffix(filepath.Dir(w.Dir), "item-1") {
			t.Errorf("workspace %d dir = %s, want it inside item-1", i, w.Dir)
		}
	}
	if ws[0].Dir == ws[1].Dir {
		t.Errorf("both workspaces are in %s", ws[0].Dir)
	}
	if got := gitIn(t, ws[0].Dir, "rev-parse", "HEAD"); got != want {
		t.Errorf("first workspace HEAD = %s, want the refreshed origin/develop %s", got, want)
	}

	again := workspaces(t, http.MethodPost, ts.URL+"/work-items/1/workspaces")
	if len(again) != 2 || again[0].ID != ws[0].ID || again[1].ID != ws[1].ID {
		t.Errorf("second ensure = %+v, want the same two rows", again)
	}
}

func TestWorkspaces_LocalFallbackAndPausedRepository(t *testing.T) {
	ts := newTestServer(t)
	repoA, _ := originRepo(t, "develop")
	repoB, _ := originRepo(t, "develop")
	repoC, _ := originRepo(t, "develop")
	gitIn(t, repoA, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "gone"))
	addProject(t, ts.URL, 1, "App", repoA, "develop")
	addProject(t, ts.URL, 2, "Lib", repoB, "develop")
	addProject(t, ts.URL, 3, "Docs", repoC, "release")
	do(t, http.MethodPost, ts.URL+"/work-items", `{"projectId":1,"title":"Demo"}`)
	do(t, http.MethodPost, ts.URL+"/work-items/1/targets", `{"projectId":2}`)
	do(t, http.MethodPost, ts.URL+"/work-items/1/targets", `{"projectId":3}`)
	do(t, http.MethodPost, ts.URL+"/spaces", `{"projectId":1,"workItemId":1,"name":"main"}`)

	ws := workspaces(t, http.MethodPost, ts.URL+"/work-items/1/workspaces")
	if len(ws) != 3 {
		t.Fatalf("workspaces = %d, want 3", len(ws))
	}
	short := gitIn(t, repoA, "rev-parse", "--short", "develop")
	if w := ws[0]; w.State != "ready" || w.BaseSource != "local" || w.BaseRef != "refs/heads/develop" ||
		!strings.HasPrefix(w.Note, "origin could not be refreshed: ") ||
		!strings.HasSuffix(w.Note, "; using local develop at "+short) {
		t.Errorf("fallback workspace = %s %s %s %q, want ready local refs/heads/develop and the fallback note",
			w.State, w.BaseSource, w.BaseRef, w.Note)
	}
	if w := ws[1]; w.State != "ready" || w.BaseSource != "origin" {
		t.Errorf("second workspace = %s %s, want ready origin: another target's fallback must not reach it", w.State, w.BaseSource)
	}
	if w := ws[2]; w.State != "paused" || !strings.Contains(w.Note, "base branch not found: release") {
		t.Errorf("third workspace = %s %q, want paused with the missing base named", w.State, w.Note)
	}
	if _, err := os.Stat(ws[2].Dir); err == nil {
		t.Errorf("paused workspace has a folder %s", ws[2].Dir)
	}
	_, msgs := do(t, http.MethodGet, ts.URL+"/messages?space=1", "")
	if !strings.Contains(msgs, "Workspace for Docs: base branch not found: release") ||
		!strings.Contains(msgs, "Workspace for App: origin could not be refreshed") {
		t.Errorf("messages = %s, want a note for the paused and the fallback workspace", msgs)
	}
}

func TestWorkspaces_RetryAfterCutCreationKeepsEdits(t *testing.T) {
	ts, st := newEmbedServer(t, nil)
	repo, _ := originRepo(t, "develop")
	addProject(t, ts.URL, 1, "App", repo, "develop")
	do(t, http.MethodPost, ts.URL+"/work-items", `{"projectId":1,"title":"Demo"}`)

	topID := int64(1)
	cut, err := st.EnsureWorkspace(t.Context(), store.Workspace{
		ProjectID: 1, WorkItemID: &topID, Dir: filepath.Join(t.TempDir(), "item-1", "1-app"),
		Branch: "work-item/1", Base: "develop",
	})
	if err != nil {
		t.Fatal(err)
	}
	gitIn(t, repo, "worktree", "add", "-b", "work-item/1", cut.Dir, "develop")
	writeIn(t, cut.Dir, "edit.txt", "mine")

	ws := workspaces(t, http.MethodPost, ts.URL+"/work-items/1/workspaces")
	if len(ws) != 1 || ws[0].ID != cut.ID || ws[0].Dir != cut.Dir || ws[0].State != "ready" {
		t.Fatalf("workspaces = %+v, want the cut row %d ready in %s", ws, cut.ID, cut.Dir)
	}
	if got, err := os.ReadFile(filepath.Join(cut.Dir, "edit.txt")); err != nil || string(got) != "mine" {
		t.Errorf("edit.txt = %q, %v; want the edit kept", got, err)
	}
	if got := workspaces(t, http.MethodGet, ts.URL+"/work-items/1/workspaces"); len(got) != 1 {
		t.Errorf("GET workspaces = %d rows, want 1", len(got))
	}
}
