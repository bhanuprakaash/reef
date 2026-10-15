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

	"github.com/bhanuprakaash/reef/channel"
	"github.com/bhanuprakaash/reef/store"
)

// writeFakeGh writes a fake `gh` that answers `pr view` with the content of
// $GH_FAKE_RESPONSE, and fails unless it ran in dir, the project's folder. When $GH_FAKE_ERR
// is set it writes that to stderr and exits nonzero instead, as gh does for a cause such as
// no PR for the branch or not being logged in. `gh run view <id> --log-failed` answers with
// the content of $GH_FAKE_LOG.
func writeFakeGh(t *testing.T, binDir, dir string) {
	t.Helper()
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = run ] && [ \"$2\" = view ]; then cat \"$GH_FAKE_LOG\"; exit 0; fi\n" +
		"if [ \"$1\" != pr ] || [ \"$2\" != view ]; then exit 1; fi\n" +
		"[ \"$PWD\" = \"" + dir + "\" ] || { echo \"wrong dir: $PWD\" >&2; exit 1; }\n" +
		"if [ -n \"$GH_FAKE_ERR\" ]; then echo \"$GH_FAKE_ERR\" >&2; exit 1; fi\n" +
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

func TestPollGitHub_NewCheckRunNotesAgain(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dataDir := t.TempDir()
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
	do(t, http.MethodPost, ts.URL+"/channels", `{"projectId":1,"featureId":1,"name":"main","backend":"claude"}`)

	binDir := t.TempDir()
	writeFakeGh(t, binDir, repo)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	respFile := filepath.Join(t.TempDir(), "resp.json")
	t.Setenv("GH_FAKE_RESPONSE", respFile)

	setGhResponse(t, respFile, `{"number":12,"url":"https://github.com/x/y/pull/12","state":"OPEN",`+
		`"statusCheckRollup":[{"__typename":"CheckRun","name":"build","status":"COMPLETED",`+
		`"conclusion":"SUCCESS","detailsUrl":"https://github.com/x/y/actions/runs/1/job/1"}]}`)
	h.s.pollGitHub(context.Background())
	msgs, err := st.Messages(context.Background(), 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 || msgs[1].Body != "build succeeded" {
		t.Fatalf("messages = %+v, want a PR note and a check note", msgs)
	}

	// A rerun of the same check name, a different run: notes again.
	setGhResponse(t, respFile, `{"number":12,"url":"https://github.com/x/y/pull/12","state":"OPEN",`+
		`"statusCheckRollup":[{"__typename":"CheckRun","name":"build","status":"COMPLETED",`+
		`"conclusion":"SUCCESS","detailsUrl":"https://github.com/x/y/actions/runs/2/job/2"}]}`)
	h.s.pollGitHub(context.Background())
	msgs, _ = st.Messages(context.Background(), 1, 0)
	if len(msgs) != 3 || msgs[2].Body != "build succeeded" {
		t.Fatalf("messages after new run = %+v, want a second check note", msgs)
	}

	h.s.pollGitHub(context.Background()) // same run again: nothing new
	if msgs, _ = st.Messages(context.Background(), 1, 0); len(msgs) != 3 {
		t.Fatalf("messages after repeat poll = %d, want 3", len(msgs))
	}
}

func TestPollGitHub_ErrorLoggedOncePerCause(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	h := New(st, t.TempDir(), "/bin/reef")
	t.Cleanup(h.Close)
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

	binDir := t.TempDir()
	writeFakeGh(t, binDir, repo)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GH_FAKE_RESPONSE", filepath.Join(t.TempDir(), "resp.json"))

	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	t.Setenv("GH_FAKE_ERR", "no pull requests found for branch \"feature/demo\"")
	h.s.pollGitHub(context.Background())
	h.s.pollGitHub(context.Background())
	if got := strings.Count(buf.String(), "no pull requests found"); got != 1 {
		t.Errorf("log lines for first cause = %d, want 1; log:\n%s", got, buf.String())
	}

	t.Setenv("GH_FAKE_ERR", "gh: not logged in to github.com")
	h.s.pollGitHub(context.Background())
	h.s.pollGitHub(context.Background())
	if got := strings.Count(buf.String(), "not logged in"); got != 1 {
		t.Errorf("log lines for second cause = %d, want 1; log:\n%s", got, buf.String())
	}
}

// ghFeatureSetup creates a git repo project, a feature with a worktree, and a channel of
// team on it, via h. It returns the repo path.
func ghFeatureSetup(t *testing.T, url, team string) string {
	t.Helper()
	repo := t.TempDir()
	gitIn(t, repo, "init", "-b", "main")
	writeIn(t, repo, "a.txt", "a")
	gitIn(t, repo, "add", ".")
	gitIn(t, repo, "commit", "-m", "first")
	do(t, http.MethodPost, url+"/projects", `{"name":"app","path":"`+repo+`"}`)
	do(t, http.MethodPost, url+"/features", `{"projectId":1,"slug":"demo","name":"Demo"}`)
	do(t, http.MethodPost, url+"/features/1/worktree", "")
	do(t, http.MethodPost, url+"/channels",
		`{"projectId":1,"featureId":1,"name":"main","team":"`+team+`","backend":"claude"}`)
	return repo
}

const _ghFailedCheck = `{"number":12,"url":"https://github.com/x/y/pull/12","state":"OPEN",` +
	`"statusCheckRollup":[{"__typename":"CheckRun","name":"build","status":"COMPLETED",` +
	`"conclusion":"FAILURE","detailsUrl":"https://github.com/x/y/actions/runs/9/job/9"}]}`

func TestPollGitHub_FailedCheckWakesDeveloper(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.ApplyTeam(ctx, store.Team{Name: "crew", Roles: []store.Role{
		{Name: "lead", Lead: true, Backend: "claude"},
		{Name: "developer", Backend: "claude", Write: true},
	}}); err != nil {
		t.Fatal(err)
	}
	fake := &fakeSession{st: st, started: make(chan struct{}, 1), release: make(chan struct{})}
	h := newHandler(st, dir, channel.NewTrees(dir), func(_ context.Context, id int64) (session, error) {
		fake.channelID.Store(id)
		return fake, nil
	})
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	repo := ghFeatureSetup(t, ts.URL, "crew")

	binDir := t.TempDir()
	writeFakeGh(t, binDir, repo)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	respFile := filepath.Join(t.TempDir(), "resp.json")
	t.Setenv("GH_FAKE_RESPONSE", respFile)
	logFile := filepath.Join(t.TempDir(), "log.txt")
	t.Setenv("GH_FAKE_LOG", logFile)
	if err := os.WriteFile(logFile, []byte("line1\nassertion failed: want 2 got 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	setGhResponse(t, respFile, _ghFailedCheck)
	sub := subscribeTo(t, ts.URL, 1)
	h.s.pollGitHub(ctx)
	sub.want(t, _idleFrame, `turn {"role":"developer","status":"running"}`,
		`agent {"kind":"text","text":"hi"}`, `turn {"role":"developer","status":"done"}`)

	fake.mu.Lock()
	texts := append([]string(nil), fake.ran...)
	fake.mu.Unlock()
	if len(texts) != 1 || !strings.Contains(texts[0], "build") || !strings.Contains(texts[0], "PR #12") ||
		!strings.Contains(texts[0], "assertion failed: want 2 got 1") {
		t.Fatalf("wake text = %q, want the check name, PR and log tail", texts)
	}

	msgs, err := st.Messages(ctx, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 || msgs[1].Body != "build failed" {
		t.Fatalf("messages = %+v, want a PR note and a failed check note", msgs)
	}

	// Same run again: no repeat note and no repeat wake.
	h.s.pollGitHub(ctx)
	fake.mu.Lock()
	again := len(fake.ran)
	fake.mu.Unlock()
	if again != 1 {
		t.Fatalf("turns run after repeat poll = %d, want 1", again)
	}

	// Restart: a fresh store and server on the same data must not wake again either.
	h.Close()
	ts.Close()
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st2, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st2.Close() })
	fake2 := &fakeSession{st: st2, started: make(chan struct{}, 1), release: make(chan struct{})}
	h2 := newHandler(st2, dir, channel.NewTrees(dir), func(_ context.Context, id int64) (session, error) {
		fake2.channelID.Store(id)
		return fake2, nil
	})
	t.Cleanup(h2.Close)
	h2.s.pollGitHub(ctx)
	fake2.mu.Lock()
	afterRestart := len(fake2.ran)
	fake2.mu.Unlock()
	if afterRestart != 0 {
		t.Fatalf("turns run after restart poll = %d, want 0 (no repeat wake)", afterRestart)
	}
}

func TestPollGitHub_SoloTeamFailedCheckNotesOnlyNoWake(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	dataDir := t.TempDir()
	h := New(st, dataDir, "/bin/reef")
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	repo := ghFeatureSetup(t, ts.URL, store.SoloTeam)

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
	if len(msgs) != 2 || msgs[1].Body != "build failed" {
		t.Fatalf("messages = %+v, want a PR note and a failed check note", msgs)
	}
	turns, err := st.Turns(ctx, store.TurnFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 0 {
		t.Fatalf("turns = %+v, want none: a solo team has no developer to wake", turns)
	}
}
