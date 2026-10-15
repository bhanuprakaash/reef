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
	"time"

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

// TestGhInterval_ZeroOrNegativeDefaults covers REEF_GH_INTERVAL=0 or a negative duration,
// which would otherwise panic time.NewTicker in the poll goroutine.
func TestGhInterval_ZeroOrNegativeDefaults(t *testing.T) {
	for _, v := range []string{"0", "0s", "-1s"} {
		t.Setenv("REEF_GH_INTERVAL", v)
		if got := ghInterval(); got != 60*time.Second {
			t.Errorf("ghInterval() with REEF_GH_INTERVAL=%q = %v, want 60s", v, got)
		}
	}
}

// TestStartGitHubPoll_PollsOnceAtStart covers a restart: the first poll must run right away,
// not after a full REEF_GH_INTERVAL (60s by default), so a restart does not sit on a stale PR
// note for a minute.
func TestStartGitHubPoll_PollsOnceAtStart(t *testing.T) {
	storeDir, dataDir := t.TempDir(), t.TempDir()
	st, err := store.Open(storeDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	h1 := New(st, dataDir, "/bin/reef")
	ts := httptest.NewServer(h1)

	repo := t.TempDir()
	gitIn(t, repo, "init", "-b", "main")
	writeIn(t, repo, "a.txt", "a")
	gitIn(t, repo, "add", ".")
	gitIn(t, repo, "commit", "-m", "first")
	do(t, http.MethodPost, ts.URL+"/projects", `{"name":"app","path":"`+repo+`"}`)
	do(t, http.MethodPost, ts.URL+"/features", `{"projectId":1,"slug":"demo","name":"Demo"}`)
	do(t, http.MethodPost, ts.URL+"/features/1/worktree", "")
	do(t, http.MethodPost, ts.URL+"/channels", `{"projectId":1,"featureId":1,"name":"main","backend":"claude"}`)
	ts.Close()
	h1.Close()

	binDir := t.TempDir()
	writeFakeGh(t, binDir, repo)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	respFile := filepath.Join(t.TempDir(), "resp.json")
	t.Setenv("GH_FAKE_RESPONSE", respFile)
	setGhResponse(t, respFile, `{"number":12,"url":"https://github.com/x/y/pull/12","state":"OPEN",`+
		`"statusCheckRollup":[]}`)

	// REEF_GH_INTERVAL is left at its 60s default: the note below must appear without the
	// test waiting anywhere near that long.
	h2 := New(st, dataDir, "/bin/reef")
	t.Cleanup(h2.Close)
	msgs, err := st.Messages(context.Background(), 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 || msgs[0].Body != "PR #12 open: https://github.com/x/y/pull/12" {
		t.Fatalf("messages right after New() = %+v, want the PR note already posted", msgs)
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

// TestPollGitHub_NotePublishesToOpenChannel covers a window already subscribed to the
// channel: the poll's note must tell its stream a message arrived, so it refreshes without a
// reload.
func TestPollGitHub_NotePublishesToOpenChannel(t *testing.T) {
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

	sub := subscribeTo(t, ts.URL, 1)
	sub.want(t, _idleFrame)

	setGhResponse(t, respFile, `{"number":12,"url":"https://github.com/x/y/pull/12","state":"OPEN",`+
		`"statusCheckRollup":[]}`)
	h.s.pollGitHub(context.Background())
	sub.want(t, "note {}")
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

// TestPollGitHub_TwoRunsSameNameBothNoted covers a rollup that lists two check runs under the
// same name, such as a matrix job or two workflows: each run is its own row, keyed by
// (feature, name, run key), so both are noted once and neither is re-noted on a repeat poll.
func TestPollGitHub_TwoRunsSameNameBothNoted(t *testing.T) {
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
		`"statusCheckRollup":[{"__typename":"CheckRun","name":"test","status":"COMPLETED",`+
		`"conclusion":"SUCCESS","detailsUrl":"https://github.com/x/y/actions/runs/1/job/1"},`+
		`{"__typename":"CheckRun","name":"test","status":"COMPLETED",`+
		`"conclusion":"SUCCESS","detailsUrl":"https://github.com/x/y/actions/runs/2/job/2"}]}`)

	for i := 0; i < 3; i++ {
		h.s.pollGitHub(context.Background())
	}
	msgs, err := st.Messages(context.Background(), 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 3 || msgs[0].Body != "PR #12 open: https://github.com/x/y/pull/12" ||
		msgs[1].Body != "test succeeded" || msgs[2].Body != "test succeeded" {
		t.Fatalf("messages after 3 identical polls = %+v, want 1 PR note + 2 check notes", msgs)
	}
}

// TestPollGitHub_TimedOutAndStartupFailureCountAsFailed covers two conclusions gh reports for
// a check run that never produced a result: both must be noted and treated as failed, the
// same as FAILURE.
func TestPollGitHub_TimedOutAndStartupFailureCountAsFailed(t *testing.T) {
	for _, conclusion := range []string{"TIMED_OUT", "STARTUP_FAILURE"} {
		t.Run(conclusion, func(t *testing.T) {
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
				`"conclusion":"`+conclusion+`","detailsUrl":"https://github.com/x/y/actions/runs/1/job/1"}]}`)

			h.s.pollGitHub(context.Background())
			msgs, err := st.Messages(context.Background(), 1, 0)
			if err != nil {
				t.Fatal(err)
			}
			if len(msgs) != 2 || msgs[1].Body != "build failed" {
				t.Fatalf("messages for conclusion %s = %+v, want a failed check note", conclusion, msgs)
			}
		})
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
	sub.want(t, _idleFrame, "note {}", "note {}", `turn {"role":"developer","status":"running"}`,
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

// TestPollGitHub_FailedCheckWakeRetriesWhenChannelBusy covers a wake that cannot run because
// the channel has a turn running: the note still posts, the wake is not lost, and is retried
// once the channel goes idle, without re-logging the busy cause on every poll.
func TestPollGitHub_FailedCheckWakeRetriesWhenChannelBusy(t *testing.T) {
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
	setGhResponse(t, respFile, _ghFailedCheck)

	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	sub := subscribeTo(t, ts.URL, 1)
	sub.want(t, _idleFrame)
	if status, body := do(t, http.MethodPost, ts.URL+"/channels/1/send", `{"text":"block"}`); status != 202 {
		t.Fatalf("send block = %d %s, want 202", status, body)
	}
	<-fake.started
	sub.want(t, `turn {"role":"lead","status":"running"}`, `agent {"kind":"text","text":"part1"}`)

	// The channel is busy: the check is still noted, but the wake cannot run.
	h.s.pollGitHub(ctx)
	h.s.pollGitHub(ctx)
	sub.want(t, "note {}", "note {}")
	msgs, err := st.Messages(ctx, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 3 || msgs[2].Body != "build failed" {
		t.Fatalf("messages while busy = %+v, want the block message, a PR note and a failed check note", msgs)
	}
	fake.mu.Lock()
	ran := len(fake.ran)
	fake.mu.Unlock()
	if ran != 1 {
		t.Fatalf("turns run while busy = %d, want 1 (only the blocking turn)", ran)
	}
	if got := strings.Count(buf.String(), "has a turn running"); got != 1 {
		t.Errorf("log lines for the busy channel = %d, want 1; log:\n%s", got, buf.String())
	}

	// The channel goes idle: the next poll wakes the developer, exactly once.
	fake.release <- struct{}{}
	sub.want(t, `agent {"kind":"text","text":"part2"}`, `turn {"role":"lead","status":"done"}`)

	h.s.pollGitHub(ctx)
	sub.want(t, `turn {"role":"developer","status":"running"}`, `agent {"kind":"text","text":"hi"}`,
		`turn {"role":"developer","status":"done"}`)

	fake.mu.Lock()
	ran = len(fake.ran)
	fake.mu.Unlock()
	if ran != 2 {
		t.Fatalf("turns run once idle = %d, want 2 (the blocking turn and the wake)", ran)
	}
	if msgs, _ = st.Messages(ctx, 1, 0); len(msgs) != 3 {
		t.Fatalf("messages after the wake = %d, want 3 (no repeat note)", len(msgs))
	}

	// Once woken, a later poll does not wake it again.
	h.s.pollGitHub(ctx)
	fake.mu.Lock()
	ran = len(fake.ran)
	fake.mu.Unlock()
	if ran != 2 {
		t.Fatalf("turns run after repeat poll = %d, want 2 (no repeat wake)", ran)
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
