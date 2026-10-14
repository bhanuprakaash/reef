package server

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/store"
)

// writeFakeGh writes a fake `gh` that answers `pr view` with the content of
// $GH_FAKE_RESPONSE, and fails unless it ran in dir, the project's folder.
func writeFakeGh(t *testing.T, binDir, dir string) {
	t.Helper()
	script := "#!/bin/sh\n" +
		"if [ \"$1\" != pr ] || [ \"$2\" != view ]; then exit 1; fi\n" +
		"[ \"$PWD\" = \"" + dir + "\" ] || { echo \"wrong dir: $PWD\" >&2; exit 1; }\n" +
		"cat \"$GH_FAKE_RESPONSE\"\n"
	if err := os.WriteFile(filepath.Join(binDir, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

func setGhResponse(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPollGitHub_PRAndCheckNotesDedupeAndSurviveRestart(t *testing.T) {
	storeDir, dataDir := t.TempDir(), t.TempDir()
	st, err := store.Open(storeDir)
	if err != nil {
		t.Fatal(err)
	}
	h := New(st, dataDir, "/bin/reef")
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)

	repo := t.TempDir()
	gitIn(t, repo, "init", "-b", "main")
	writeIn(t, repo, "a.txt", "a")
	gitIn(t, repo, "add", ".")
	gitIn(t, repo, "commit", "-m", "first")
	do(t, http.MethodPost, ts.URL+"/projects", `{"name":"app","path":"`+repo+`"}`)
	do(t, http.MethodPost, ts.URL+"/features", `{"projectId":1,"slug":"demo","name":"Demo"}`)
	do(t, http.MethodPost, ts.URL+"/features/1/worktree", "")
	if status, body := do(t, http.MethodPost, ts.URL+"/channels",
		`{"projectId":1,"featureId":1,"name":"main","backend":"claude"}`); status != 201 {
		t.Fatalf("create channel = %d %s", status, body)
	}

	binDir := t.TempDir()
	writeFakeGh(t, binDir, repo)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	respFile := filepath.Join(t.TempDir(), "resp.json")
	t.Setenv("GH_FAKE_RESPONSE", respFile) // missing file: gh exits nonzero, as for "no PR"
	h.s.pollGitHub(context.Background())
	if msgs, _ := st.Messages(context.Background(), 1, 0); len(msgs) != 0 {
		t.Fatalf("messages with no PR = %+v, want none", msgs)
	}

	setGhResponse(t, respFile, `{"number":12,"url":"https://github.com/x/y/pull/12","state":"OPEN",`+
		`"statusCheckRollup":[]}`)
	h.s.pollGitHub(context.Background())
	msgs, err := st.Messages(context.Background(), 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 || msgs[0].Body != "PR #12 open: https://github.com/x/y/pull/12" {
		t.Fatalf("messages = %+v, want one PR note", msgs)
	}

	h.s.pollGitHub(context.Background()) // same answer: nothing new
	if msgs, _ = st.Messages(context.Background(), 1, 0); len(msgs) != 1 {
		t.Fatalf("messages after repeat poll = %d, want 1", len(msgs))
	}

	setGhResponse(t, respFile, `{"number":12,"url":"https://github.com/x/y/pull/12","state":"OPEN",`+
		`"statusCheckRollup":[{"__typename":"CheckRun","name":"build","status":"COMPLETED","conclusion":"SUCCESS"}]}`)
	h.s.pollGitHub(context.Background())
	msgs, _ = st.Messages(context.Background(), 1, 0)
	if len(msgs) != 2 || msgs[1].Body != "build succeeded" {
		t.Fatalf("messages = %+v, want a check note naming build", msgs)
	}

	h.s.pollGitHub(context.Background()) // same finished check: nothing new
	if msgs, _ = st.Messages(context.Background(), 1, 0); len(msgs) != 2 {
		t.Fatalf("messages after repeat check poll = %d, want 2", len(msgs))
	}

	// Restart: a fresh store and server on the same data must not repeat either note.
	h.Close()
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st2, err := store.Open(storeDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st2.Close() })
	h2 := New(st2, dataDir, "/bin/reef")
	t.Cleanup(h2.Close)
	h2.s.pollGitHub(context.Background())
	if msgs, _ = st2.Messages(context.Background(), 1, 0); len(msgs) != 2 {
		t.Fatalf("messages after restart poll = %d, want 2 (no repeat)", len(msgs))
	}
}

func TestPollGitHub_NoGhOnPathLogsOnce(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	h := New(st, t.TempDir(), "/bin/reef")
	t.Cleanup(h.Close)
	t.Setenv("PATH", t.TempDir()) // no gh anywhere on it

	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	h.s.pollGitHub(context.Background())
	h.s.pollGitHub(context.Background())

	if got := strings.Count(buf.String(), "gh is not on PATH"); got != 1 {
		t.Errorf("log lines about missing gh = %d, want 1; log:\n%s", got, buf.String())
	}
}
