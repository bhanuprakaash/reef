package server

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/bhanuprakaash/reef/store"
)

// fakeCLIs puts a gh and a glab first on PATH that print what answer last set for them, or
// fail with the stderr text when it is set after a body of "".
func fakeCLIs(t *testing.T) (answer func(bin, body, stderr string)) {
	t.Helper()
	bin, dir := t.TempDir(), t.TempDir()
	for _, name := range []string{"gh", "glab"} {
		script := "#!/bin/sh\nif [ -s " + dir + "/" + name + ".err ]; then cat " + dir + "/" + name +
			".err >&2; exit 1; fi\ncat " + dir + "/" + name + "\n"
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return func(name, body, stderr string) {
		for suffix, content := range map[string]string{"": body, ".err": stderr} {
			if err := os.WriteFile(filepath.Join(dir, name+suffix), []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func ghAnswer(state, review, conclusion, run string) string {
	return `{"state":"` + state + `","reviewDecision":"` + review + `","statusCheckRollup":[` +
		`{"__typename":"CheckRun","name":"build","status":"COMPLETED","conclusion":"` + conclusion +
		`","detailsUrl":"https://github.com/x/y/actions/runs/` + run + `/job/1"}]}`
}

// trackedRequest records an open GitHub pull request #12 of Work item 1, or of Space 1 when
// workItem is false.
func trackedRequest(t *testing.T, env *liveEnv, workItem bool) store.RepoRequest {
	t.Helper()
	ctx := context.Background()
	one, w := int64(1), store.Workspace{ProjectID: 1, Name: "fix", Dir: t.TempDir(), Branch: "fix"}
	r := store.RepoRequest{Provider: store.RepoGitHub, Host: "github.com", Path: "x/y", Number: 12,
		URL: "https://github.com/x/y/pull/12", State: store.RequestOpened, Head: "fix", Base: "main"}
	if workItem {
		w.WorkItemID, r.WorkItemID = &one, &one
	} else {
		w.SpaceID, r.SpaceID = &one, &one
	}
	w, err := env.st.EnsureWorkspace(ctx, w)
	if err != nil {
		t.Fatal(err)
	}
	r.WorkspaceID = w.ID
	if r, err = env.st.PutRepoRequest(ctx, r); err != nil {
		t.Fatal(err)
	}
	return r
}

func (e *liveEnv) request(t *testing.T, id int64) store.RepoRequest {
	t.Helper()
	rows, err := e.st.TrackedRequests(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.ID == id {
			return r
		}
	}
	t.Fatalf("request %d is not tracked", id)
	return store.RepoRequest{}
}

func (e *liveEnv) wakesAbout(text string) int {
	n := 0
	for _, p := range e.ranTexts() {
		if strings.Contains(p, text) {
			n++
		}
	}
	return n
}

// startedEnv has item 1 in progress with its lead idle in Space 2.
func startedEnv(t *testing.T) *liveEnv {
	t.Helper()
	env := newLiveEnv(t)
	do(t, http.MethodPost, env.url+"/work-items", `{"projectId":1,"title":"Login","ownerBotId":1}`)
	do(t, http.MethodPost, env.url+"/work-items/1/approve", `{"key":"k1"}`)
	deadline := time.Now().Add(5 * time.Second)
	for env.h.s.lookup(2) == nil {
		if time.Now().After(deadline) {
			t.Fatal("item 1 was not opened")
		}
		time.Sleep(10 * time.Millisecond)
	}
	env.waitIdle(t, 2)
	return env
}

func TestTrackReadsBothProvidersAndRecoversFromAnOutage(t *testing.T) {
	ctx := context.Background()
	env := newLiveEnv(t)
	answer := fakeCLIs(t)
	gh := trackedRequest(t, env, false)
	lab, err := env.st.PutRepoRequest(ctx, store.RepoRequest{Provider: store.RepoGitLab, Host: "gitlab.com", Path: "g/s/r",
		Number: 5, URL: "https://gitlab.com/g/s/r/-/merge_requests/5", State: store.RequestOpened, Head: "fix2",
		Base: "main", WorkspaceID: gh.WorkspaceID, SpaceID: gh.SpaceID})
	if err != nil {
		t.Fatal(err)
	}
	answer("gh", ghAnswer("OPEN", "APPROVED", "SUCCESS", "1"), "")
	answer("glab", `{"state":"opened","detailed_merge_status":"not_approved","head_pipeline":{"id":77,"status":"running"}}`, "")
	env.h.s.trackRepoRequests(ctx)
	got := env.request(t, gh.ID)
	if got.State != "opened" || got.Review != "approved" || len(got.Checks) != 1 || got.Checks[0].State != "pass" || got.CheckedAt == nil {
		t.Errorf("GitHub request = %+v", got)
	}
	if got = env.request(t, lab.ID); got.State != "opened" || got.Review != "review_required" || got.Checks[0].State != "pending" || got.Checks[0].Key != "77" {
		t.Errorf("GitLab request = %+v", got)
	}

	answer("gh", ghAnswer("CLOSED", "", "SUCCESS", "1"), "")
	answer("glab", `{"state":"merged","head_pipeline":{"id":77,"status":"success"}}`, "")
	env.h.s.trackRepoRequests(ctx)
	if got = env.request(t, gh.ID); got.State != "closed" {
		t.Errorf("closed GitHub request state = %q, want closed", got.State)
	}
	if rows, _ := env.st.TrackedRequests(ctx); len(rows) != 1 {
		t.Errorf("tracked after the merge = %d requests, want only the closed one", len(rows))
	}

	answer("gh", "", "HTTP 401: Bad credentials (token ghp_secret)")
	env.h.s.trackRepoRequests(ctx)
	if got = env.request(t, gh.ID); got.CheckError != "login rejected: the token is expired or revoked" || got.State != "closed" {
		t.Errorf("after an outage = %+v, want the fixed reason and the last state kept", got)
	}
	answer("gh", ghAnswer("OPEN", "", "SUCCESS", "1"), "")
	env.h.s.trackRepoRequests(ctx)
	if got = env.request(t, gh.ID); got.CheckError != "" || got.State != "opened" {
		t.Errorf("after recovery = %+v, want no error and opened again", got)
	}
}

func TestTrackWakesTheLeadOnceForANewFailure(t *testing.T) {
	ctx := context.Background()
	env := startedEnv(t)
	answer := fakeCLIs(t)
	r := trackedRequest(t, env, true)
	answer("gh", ghAnswer("OPEN", "", "FAILURE", "9"), "")
	const text = "build failed on pull request #12 in x/y"
	for range 3 {
		env.h.s.trackRepoRequests(ctx)
		env.waitIdle(t, 2)
	}
	if n := env.wakesAbout(text); n != 1 {
		t.Fatalf("wakes about the failure after 3 reads = %d, want 1; prompts = %q", n, env.ranTexts())
	}
	if got := env.request(t, r.ID); got.Failure == "" || got.Failure != got.FailureHandled {
		t.Errorf("failure %q handled %q, want handled", got.Failure, got.FailureHandled)
	}

	answer("gh", ghAnswer("OPEN", "", "FAILURE", "10"), "")
	if err := env.st.PauseSpace(ctx, 2, "stopped by the user"); err != nil {
		t.Fatal(err)
	}
	env.h.s.trackRepoRequests(ctx)
	if sp, _ := env.st.Space(ctx, 2); sp.Paused != "stopped by the user" || env.wakesAbout(text) != 1 {
		t.Errorf("paused Space = %q with %d wakes, want the pause kept and no new wake", sp.Paused, env.wakesAbout(text))
	}
}

func TestTrackSendsAReviewedItemBackOnce(t *testing.T) {
	ctx := context.Background()
	env := submittedEnv(t)
	answer := fakeCLIs(t)
	r := trackedRequest(t, env, true)
	answer("gh", ghAnswer("OPEN", "", "FAILURE", "9"), "")
	for range 2 {
		env.h.s.trackRepoRequests(ctx)
		env.waitIdle(t, 2)
	}
	want := _reefSendBack + "build failed on pull request #12 in x/y: https://github.com/x/y/pull/12"
	if n := env.wakesAbout(want); n != 1 {
		t.Fatalf("send-back prompts = %d, want 1; prompts = %q", n, env.ranTexts())
	}
	if _, body := do(t, http.MethodGet, env.url+"/work-items/1", ""); !strings.Contains(body, `"status":"in_progress"`) {
		t.Errorf("item 1 = %s, want back in progress", body)
	}

	if got := env.request(t, r.ID); got.Failure != got.FailureHandled {
		t.Errorf("failure %q handled %q, want handled", got.Failure, got.FailureHandled)
	}
}

func TestTrackNotifiesInsteadOfRestartingDoneWork(t *testing.T) {
	ctx := context.Background()
	env := submittedEnv(t)
	if status, body := do(t, http.MethodPatch, env.url+"/work-items/1", `{"status":"done"}`); status != 200 {
		t.Fatalf("done = %d %s", status, body)
	}
	answer := fakeCLIs(t)
	trackedRequest(t, env, true)
	answer("gh", ghAnswer("OPEN", "", "FAILURE", "9"), "")
	ran := len(env.ranTexts())
	for range 2 {
		env.h.s.trackRepoRequests(ctx)
	}
	items, err := env.st.Inbox(ctx)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, i := range items {
		if i.Kind == store.KindDelivery && strings.Contains(i.Text, "is Done and was not restarted") {
			n++
		}
	}
	if n != 1 || len(env.ranTexts()) != ran {
		t.Errorf("delivery notices = %d, turns started = %d, want 1 and 0", n, len(env.ranTexts())-ran)
	}
	if _, body := do(t, http.MethodGet, env.url+"/work-items/1", ""); !strings.Contains(body, `"status":"done"`) {
		t.Errorf("item 1 = %s, want still done", body)
	}
}

func itoa(id int64) string { return strconv.FormatInt(id, 10) }
