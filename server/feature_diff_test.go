package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bhanuprakaash/reef/store"
	"github.com/bhanuprakaash/reef/worktree"
)

func TestGetFeatureDiff_UncommittedFile(t *testing.T) {
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
	do(t, http.MethodPost, ts.URL+"/features/1/worktree", "")

	dir := worktree.Path(dataDir, 1, "demo")
	writeIn(t, dir, "new.txt", "one\ntwo\n")

	status, body := do(t, http.MethodGet, ts.URL+"/features/1/diff", "")
	if status != 200 {
		t.Fatalf("status = %d, body = %s", status, body)
	}
	want := `[{"path":"new.txt","status":"added","hunks":[{"header":"@@ -0,0 +1,2 @@",` +
		`"lines":[{"kind":"add","new":1,"text":"one"},{"kind":"add","new":2,"text":"two"}]}]}]`
	if body != want {
		t.Errorf("diff = %s, want %s", body, want)
	}
}

func TestGetFeatureDiff_NoWorktree(t *testing.T) {
	ts := newTestServer(t)
	repo := t.TempDir()
	gitIn(t, repo, "init", "-b", "main")
	writeIn(t, repo, "a.txt", "a")
	gitIn(t, repo, "add", ".")
	gitIn(t, repo, "commit", "-m", "first")
	do(t, http.MethodPost, ts.URL+"/projects", `{"name":"app","path":"`+repo+`"}`)
	do(t, http.MethodPost, ts.URL+"/features", `{"projectId":1,"slug":"demo","name":"Demo"}`)

	status, body := do(t, http.MethodGet, ts.URL+"/features/1/diff", "")
	if want := `[]`; status != 200 || body != want {
		t.Errorf("diff = %d %s, want 200 %s", status, body, want)
	}
}

func TestGetFeatureDiff_UnknownFeature(t *testing.T) {
	ts := newTestServer(t)
	status, body := do(t, http.MethodGet, ts.URL+"/features/9/diff", "")
	if want := `{"error":"feature 9: not found"}`; status != 404 || body != want {
		t.Errorf("diff of unknown feature = %d %s, want 404 %s", status, body, want)
	}
}
