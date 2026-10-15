package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/store"
	"github.com/bhanuprakaash/reef/worktree"
)

func TestEnsureFeatureWorktree_MakesItAndReusesIt(t *testing.T) {
	dataDir := t.TempDir()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	h := New(st, dataDir, "/bin/reef")
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	t.Cleanup(h.Close)

	repo := t.TempDir()
	gitIn(t, repo, "init", "-b", "main")
	writeIn(t, repo, "a.txt", "a")
	gitIn(t, repo, "add", ".")
	gitIn(t, repo, "commit", "-m", "first")
	do(t, http.MethodPost, ts.URL+"/projects", `{"name":"app","path":"`+repo+`"}`)
	do(t, http.MethodPost, ts.URL+"/features", `{"projectId":1,"slug":"demo","name":"Demo"}`)

	want := worktree.Path(dataDir, 1, "demo")
	if status, body := do(t, http.MethodGet, ts.URL+"/features/1/worktree", ""); status != 200 || body != `{"path":""}` {
		t.Fatalf("get worktree before = %d %s, want 200 {\"path\":\"\"}", status, body)
	}
	status, body := do(t, http.MethodPost, ts.URL+"/features/1/worktree", "")
	if wantBody := fmt.Sprintf(`{"path":%q}`, want); status != 200 || body != wantBody {
		t.Fatalf("ensure worktree = %d %s, want 200 %s", status, body, wantBody)
	}
	if got := gitIn(t, want, "branch", "--show-current"); got != "feature/demo" {
		t.Errorf("branch = %q, want feature/demo", got)
	}

	if status, body := do(t, http.MethodGet, ts.URL+"/features/1/worktree", ""); status != 200 || body != fmt.Sprintf(`{"path":%q}`, want) {
		t.Fatalf("get worktree after = %d %s, want 200 path %s", status, body, want)
	}

	// A second call reuses the same tree.
	status, body = do(t, http.MethodPost, ts.URL+"/features/1/worktree", "")
	if wantBody := fmt.Sprintf(`{"path":%q}`, want); status != 200 || body != wantBody {
		t.Fatalf("ensure worktree again = %d %s, want 200 %s", status, body, wantBody)
	}
}

func TestEnsureFeatureWorktree_UnknownFeature(t *testing.T) {
	ts := newTestServer(t)
	status, body := do(t, http.MethodPost, ts.URL+"/features/9/worktree", "")
	if want := `{"error":"feature 9: not found"}`; status != 404 || body != want {
		t.Errorf("ensure worktree of unknown feature = %d %s, want 404 %s", status, body, want)
	}
}

func TestEnsureFeatureWorktree_RepoWithoutCommit(t *testing.T) {
	ts := newTestServer(t)
	repo := t.TempDir()
	gitIn(t, repo, "init", "-b", "main")
	do(t, http.MethodPost, ts.URL+"/projects", `{"name":"app","path":"`+repo+`"}`)
	do(t, http.MethodPost, ts.URL+"/features", `{"projectId":1,"slug":"demo","name":"Demo"}`)

	status, body := do(t, http.MethodPost, ts.URL+"/features/1/worktree", "")
	want := "features need the project folder to be a git repo with a commit: "
	if status != 409 || !strings.Contains(body, want) {
		t.Errorf("ensure worktree in a repo with no commit = %d %s, want 409 containing %q", status, body, want)
	}
}
