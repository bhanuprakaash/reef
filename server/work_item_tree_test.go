package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/store"
)

func TestSubItemUsesParentTreeBranchAndPR(t *testing.T) {
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
	do(t, http.MethodPost, ts.URL+"/work-items", `{"projectId":1,"title":"Parent"}`)
	do(t, http.MethodPost, ts.URL+"/work-items", `{"projectId":1,"parentId":1,"title":"Child"}`)

	tree := filepath.Join(dataDir, "worktrees", "item-1", "1-app")
	for _, method := range []string{http.MethodPost, http.MethodGet} {
		status, body := do(t, method, ts.URL+"/work-items/2/workspaces", "")
		if status != 200 || !strings.Contains(body, `"workItemId":1,`) ||
			!strings.Contains(body, fmt.Sprintf(`"dir":%q,"branch":"work-item/1"`, tree)) {
			t.Fatalf("%s workspaces of sub-item = %d %s, want the parent's workspace at %s", method, status, body, tree)
		}
	}

	writeIn(t, tree, "new.txt", "one\n")
	wantDiff := `[{"path":"1-app/new.txt","status":"added","hunks":[{"header":"@@ -0,0 +1 @@",` +
		`"lines":[{"kind":"add","new":1,"text":"one"}]}]}]`
	if status, body := do(t, http.MethodGet, ts.URL+"/work-items/2/diff", ""); status != 200 || body != wantDiff {
		t.Errorf("diff of sub-item = %d %s, want 200 %s", status, body, wantDiff)
	}

	binDir, args := t.TempDir(), filepath.Join(t.TempDir(), "args.txt")
	script := "#!/bin/sh\necho \"$*\" >>" + args + "\necho '{\"number\":3,\"state\":\"OPEN\"}'\n"
	if err := os.WriteFile(filepath.Join(binDir, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if status, body := do(t, http.MethodGet, ts.URL+"/work-items/2/pr", ""); status != 200 {
		t.Fatalf("pr of sub-item = %d %s, want 200", status, body)
	}
	got, err := os.ReadFile(args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(got), "pr view work-item/1 ") {
		t.Errorf("gh ran %q, want it to view branch work-item/1", got)
	}

	if status, body := do(t, http.MethodDelete, ts.URL+"/work-items/2", ""); status != 204 {
		t.Fatalf("delete sub-item = %d %s, want 204", status, body)
	}
	if _, err := os.Stat(filepath.Join(tree, ".git")); err != nil {
		t.Errorf("deleting the sub-item removed the parent's worktree: %v", err)
	}
}

func TestPollGitHub_SubItemNotPolledOnItsOwn(t *testing.T) {
	ctx := context.Background()
	st, err := openStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	h := New(st, testMemory(t, st), t.TempDir(), "/bin/reef")
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	t.Cleanup(h.Close)
	repo := ghWorkItemSetup(t, ts.URL, store.SoloTeam)
	do(t, http.MethodPost, ts.URL+"/work-items", `{"projectId":1,"parentId":1,"title":"Child"}`)

	binDir := t.TempDir()
	writeFakeGh(t, binDir, repo)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	respFile := filepath.Join(t.TempDir(), "resp.json")
	t.Setenv("GH_FAKE_RESPONSE", respFile)
	setGhResponse(t, respFile, _ghFailedCheck)

	h.s.pollGitHub(ctx)
	msgs, err := st.Messages(ctx, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Errorf("space 1 has %d messages, want 2: %+v", len(msgs), msgs)
	}
}
