package server

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bhanuprakaash/reef/space"
	"github.com/bhanuprakaash/reef/store"
)

func TestSubmitStartsTheNextItemAndTheWrapUp(t *testing.T) {
	env := newLiveEnv(t)
	do(t, http.MethodPost, env.url+"/work-items",
		`{"projectId":1,"title":"Login","teamId":1,"acceptanceLines":[{"text":"wrong password is refused"}]}`)
	do(t, http.MethodPost, env.url+"/work-items", `{"projectId":1,"title":"Logout","teamId":1}`)
	queued := make(chan struct{})
	var wraps atomic.Int32
	env.fake.onRun = func(_, text string, _ *int64) {
		switch {
		case text == space.WrapCommand:
			wraps.Add(1)
		case strings.Contains(text, "Work item 1:"):
			<-queued
			_, err := env.st.SubmitForReview(context.Background(), 1,
				[]store.LineTick{{Position: 1, Checked: true, Note: "401"}}, store.Proof{SpaceID: 2})
			if err != nil {
				t.Error(err)
			}
		}
	}

	do(t, http.MethodPatch, env.url+"/work-items/1", `{"status":"queued"}`)
	do(t, http.MethodPatch, env.url+"/work-items/2", `{"status":"queued"}`)
	close(queued)

	deadline := time.Now().Add(5 * time.Second)
	for {
		env.fake.mu.Lock()
		started := slices.ContainsFunc(env.fake.ran, func(p string) bool {
			return strings.HasSuffix(p, "Work item 2: Logout\nDescription: none\nBrief: none")
		})
		env.fake.mu.Unlock()
		if started && wraps.Load() == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("after the submit item 2 started = %v, wrap-ups = %d, want true and 1", started, wraps.Load())
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, body := do(t, http.MethodGet, env.url+"/work-items/1", ""); !strings.Contains(body, `"status":"review"`) {
		t.Errorf("item 1 = %s, want review", body)
	}
	if _, body := do(t, http.MethodGet, env.url+"/work-items/2", ""); !strings.Contains(body, `"status":"in_progress"`) {
		t.Errorf("item 2 = %s, want in_progress", body)
	}
	status, body := do(t, http.MethodGet, env.url+"/work-items/1/proof", "")
	want := `"acceptanceLines":[{"position":1,"text":"wrong password is refused","checked":true,"note":"401"}],"commands":[],"screenshots":[]`
	if status != 200 || !strings.Contains(body, want) {
		t.Errorf("proof of item 1 = %d %s, want 200 with %s", status, body, want)
	}
	if status, body := do(t, http.MethodGet, env.url+"/work-items/2/proof", ""); status != 404 {
		t.Errorf("proof of item 2 = %d %s, want 404", status, body)
	}
}
