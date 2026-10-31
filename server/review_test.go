package server

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/bhanuprakaash/reef/space"
	"github.com/bhanuprakaash/reef/store"
)

// submittedEnv runs item 1 until its lead has submitted it for review from Space 2 and the
// wrap-up proposed one memory item.
func submittedEnv(t *testing.T) *liveEnv {
	t.Helper()
	env := newLiveEnv(t)
	do(t, http.MethodPost, env.url+"/work-items", `{"projectId":1,"title":"Login","ownerBotId":1}`)
	env.fake.onRun = func(_, text string, _ *int64) {
		switch {
		case text == space.WrapCommand:
			_, err := env.st.CreateProposal(context.Background(), store.Proposal{
				SpaceID: 2, Role: "lead", BotID: testServerBot(t, env.st, "lead"), Brief: "login done",
				Items: []store.ProposedItem{{Kind: "decision", Body: "use sessions", Scope: "work_item"}},
			})
			if err != nil {
				t.Error(err)
			}
		case strings.Contains(text, "Work item 1:"):
			if _, err := env.st.SubmitForReview(context.Background(), 1, nil, store.Proof{SpaceID: 2}); err != nil {
				t.Error(err)
			}
		}
	}
	do(t, http.MethodPost, env.url+"/work-items/1/approve", `{"key":"k1"}`)
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := env.st.PendingProposal(context.Background(), 2); err == nil {
			time.Sleep(100 * time.Millisecond)
			return env
		}
		if time.Now().After(deadline) {
			t.Fatal("no wrap-up proposal after the submit")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestDoneApprovesTheWrapUp(t *testing.T) {
	env := submittedEnv(t)

	status, body := do(t, http.MethodPatch, env.url+"/work-items/1", `{"status":"done"}`)
	if status != 200 || !strings.Contains(body, `"status":"done"`) {
		t.Errorf("Done = %d %s, want 200 with status done", status, body)
	}
	if got := countMemory(t, env.st); got != 1 {
		t.Errorf("memory items after Done = %d, want 1", got)
	}
	if _, body := do(t, http.MethodGet, env.url+"/spaces/2", ""); !strings.Contains(body, `"closedAt":"`) {
		t.Errorf("Space 2 after Done = %s, want closed", body)
	}
}

func TestSendBackReachesTheLeadWithTheNote(t *testing.T) {
	env := submittedEnv(t)

	status, body := do(t, http.MethodPatch, env.url+"/work-items/1", `{"status":"queued"}`)
	if status != 400 || !strings.Contains(body, "a note for the lead is required") {
		t.Errorf("send back without a note = %d %s, want 400 asking for a note", status, body)
	}
	status, body = do(t, http.MethodPatch, env.url+"/work-items/1", `{"status":"queued","note":"401 not 403"}`)
	if status != 200 {
		t.Fatalf("send back = %d %s, want 200", status, body)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		env.fake.mu.Lock()
		woken := slices.ContainsFunc(env.fake.ran, func(p string) bool {
			return strings.HasSuffix(p, "Sent back from review by the user:\n401 not 403")
		})
		env.fake.mu.Unlock()
		if woken {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the lead never got the note; prompts = %q", env.fake.ran)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, body := do(t, http.MethodGet, env.url+"/work-items/1", ""); !strings.Contains(body, `"status":"in_progress"`) {
		t.Errorf("item 1 after send back = %s, want in_progress", body)
	}
	if _, err := env.st.PendingProposal(context.Background(), 2); err == nil {
		t.Error("the wrap-up proposal is still pending after send back, want rejected")
	}
}

// submittedUnwrappedEnv has item 1 in review from Space 2 as after a quit: no wrap-up started.
func submittedUnwrappedEnv(t *testing.T) *liveEnv {
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
	if _, err := env.st.SubmitForReview(context.Background(), 1, nil, store.Proof{SpaceID: 2}); err != nil {
		t.Fatal(err)
	}
	return env
}

func TestDeleteSpaceInReviewIsRefused(t *testing.T) {
	env := submittedEnv(t)

	status, body := do(t, http.MethodDelete, env.url+"/spaces/2", "")
	if status != 409 || !strings.Contains(body, "in review") {
		t.Errorf("delete the submitting Space = %d %s, want 409 naming review", status, body)
	}
	if status, body := do(t, http.MethodPatch, env.url+"/work-items/1", `{"status":"done"}`); status != 200 {
		t.Errorf("Done after the refused delete = %d %s, want 200", status, body)
	}
}

func TestProofScreenshotsServeOnlyImagesUnderTheDataFolder(t *testing.T) {
	env := newLiveEnv(t)
	do(t, http.MethodPost, env.url+"/work-items", `{"projectId":1,"title":"Login","ownerBotId":1}`)
	do(t, http.MethodPost, env.url+"/work-items/1/approve", `{"key":"k1"}`)
	env.waitIdle(t, 2)
	inside := filepath.Join(env.dir, "shot.png")
	notes := filepath.Join(env.dir, "notes.txt")
	outside := filepath.Join(t.TempDir(), "secret.png")
	for _, p := range []string{inside, notes, outside} {
		if err := os.WriteFile(p, []byte("data"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	proof := store.Proof{SpaceID: 2, Screenshots: []string{inside, notes, outside}}
	if _, err := env.st.SubmitForReview(context.Background(), 1, nil, proof); err != nil {
		t.Fatal(err)
	}

	for n, want := range []int{200, 404, 404, 404} {
		url := env.url + "/work-items/1/proof/screenshots/" + strconv.Itoa(n)
		if status, body := do(t, http.MethodGet, url, ""); status != want {
			t.Errorf("screenshot %d = %d %s, want %d", n, status, body, want)
		}
	}
}

func TestDoneWithoutWrapUpClosesTheSpace(t *testing.T) {
	env := submittedUnwrappedEnv(t)
	brief, err := env.st.CreateProposal(context.Background(),
		store.Proposal{SpaceID: 2, Kind: store.ProposalBrief, Role: "lead", Brief: "mid-run brief"})
	if err != nil {
		t.Fatal(err)
	}

	if status, body := do(t, http.MethodPatch, env.url+"/work-items/1", `{"status":"done"}`); status != 200 {
		t.Fatalf("Done = %d %s, want 200", status, body)
	}
	if got, err := env.st.Proposal(context.Background(), brief.ID); err != nil || got.Status != store.ProposalPending {
		t.Errorf("brief proposal after Done = %+v, %v, want still pending", got, err)
	}
	if _, body := do(t, http.MethodGet, env.url+"/spaces/2", ""); !strings.Contains(body, `"closedAt":"`) {
		t.Errorf("Space 2 after Done = %s, want closed", body)
	}
}

func TestWrapUpRunsAfterRestart(t *testing.T) {
	env := submittedUnwrappedEnv(t)
	env.fake.onRun = func(_, text string, _ *int64) {
		if text != space.WrapCommand {
			return
		}
		_, err := env.st.CreateProposal(context.Background(), store.Proposal{SpaceID: 2, Role: "lead", BotID: testServerBot(t, env.st, "lead"), Brief: "login done"})
		if err != nil {
			t.Error(err)
		}
	}
	env.restart(t)

	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := env.st.PendingProposal(context.Background(), 2); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no wrap-up proposal after the restart")
		}
		time.Sleep(10 * time.Millisecond)
	}
	env.waitIdle(t, 2)
	if status, body := do(t, http.MethodPatch, env.url+"/work-items/1", `{"status":"done"}`); status != 200 {
		t.Fatalf("Done = %d %s, want 200", status, body)
	}
	if _, body := do(t, http.MethodGet, env.url+"/spaces/2", ""); !strings.Contains(body, `"closedAt":"`) {
		t.Errorf("Space 2 after Done = %s, want closed", body)
	}
}
