package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/worktree"
)

func TestGetWorkItemDiff_UncommittedFile(t *testing.T) {
	dataDir := t.TempDir()
	st, err := openStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	h := New(st, testMemory(t, st), dataDir, "/bin/reef")
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	t.Cleanup(h.Close)

	repo := t.TempDir()
	gitIn(t, repo, "init", "-b", "main")
	writeIn(t, repo, "a.txt", "a")
	gitIn(t, repo, "add", ".")
	gitIn(t, repo, "commit", "-m", "first")
	do(t, http.MethodPost, ts.URL+"/projects", `{"name":"app","path":"`+repo+`"}`)
	do(t, http.MethodPost, ts.URL+"/work-items", `{"projectId":1,"title":"Demo"}`)
	do(t, http.MethodPost, ts.URL+"/work-items/1/worktree", "")

	dir := worktree.Path(dataDir, 1, "1")
	writeIn(t, dir, "new.txt", "one\ntwo\n")

	status, body := do(t, http.MethodGet, ts.URL+"/work-items/1/diff", "")
	if status != 200 {
		t.Fatalf("status = %d, body = %s", status, body)
	}
	want := `[{"path":"new.txt","status":"added","hunks":[{"header":"@@ -0,0 +1,2 @@",` +
		`"lines":[{"kind":"add","new":1,"text":"one"},{"kind":"add","new":2,"text":"two"}]}]}]`
	if body != want {
		t.Errorf("diff = %s, want %s", body, want)
	}
}

func TestGetWorkItemDiff_ScopeCurrent(t *testing.T) {
	dataDir := t.TempDir()
	st, err := openStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	h := New(st, testMemory(t, st), dataDir, "/bin/reef")
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	t.Cleanup(h.Close)

	repo := t.TempDir()
	gitIn(t, repo, "init", "-b", "main")
	writeIn(t, repo, "a.txt", "a")
	gitIn(t, repo, "add", ".")
	gitIn(t, repo, "commit", "-m", "first")
	do(t, http.MethodPost, ts.URL+"/projects", `{"name":"app","path":"`+repo+`"}`)
	do(t, http.MethodPost, ts.URL+"/work-items", `{"projectId":1,"title":"Demo"}`)
	do(t, http.MethodPost, ts.URL+"/work-items/1/worktree", "")

	dir := worktree.Path(dataDir, 1, "1")
	writeIn(t, dir, "old.txt", "old\n")
	gitIn(t, dir, "add", ".")
	gitIn(t, dir, "commit", "-m", "old work")
	writeIn(t, dir, "new.txt", "new\n")

	paths := func(query string) string {
		status, body := do(t, http.MethodGet, ts.URL+"/work-items/1/diff"+query, "")
		if status != 200 {
			t.Fatalf("status = %d, body = %s", status, body)
		}
		var files []worktree.FileDiff
		if err := json.Unmarshal([]byte(body), &files); err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, f := range files {
			out = append(out, f.Path)
		}
		return strings.Join(out, ",")
	}
	if got, want := paths(""), "new.txt,old.txt"; got != want {
		t.Errorf("full diff = %q, want %q", got, want)
	}
	if got, want := paths("?scope=current"), "new.txt"; got != want {
		t.Errorf("current diff = %q, want %q", got, want)
	}
}

func TestGetWorkItemDiff_NoWorktree(t *testing.T) {
	ts := newTestServer(t)
	repo := t.TempDir()
	gitIn(t, repo, "init", "-b", "main")
	writeIn(t, repo, "a.txt", "a")
	gitIn(t, repo, "add", ".")
	gitIn(t, repo, "commit", "-m", "first")
	do(t, http.MethodPost, ts.URL+"/projects", `{"name":"app","path":"`+repo+`"}`)
	do(t, http.MethodPost, ts.URL+"/work-items", `{"projectId":1,"title":"Demo"}`)

	status, body := do(t, http.MethodGet, ts.URL+"/work-items/1/diff", "")
	if want := `[]`; status != 200 || body != want {
		t.Errorf("diff = %d %s, want 200 %s", status, body, want)
	}
}

func TestGetWorkItemDiff_UnknownWorkItem(t *testing.T) {
	ts := newTestServer(t)
	status, body := do(t, http.MethodGet, ts.URL+"/work-items/9/diff", "")
	if want := `{"error":"work item 9: not found"}`; status != 404 || body != want {
		t.Errorf("diff of unknown work item = %d %s, want 404 %s", status, body, want)
	}
}
