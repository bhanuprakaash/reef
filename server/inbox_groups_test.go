package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/store"
)

func (e *liveEnv) inboxItems(t *testing.T) []store.InboxItem {
	t.Helper()
	_, body := do(t, http.MethodGet, e.url+"/inbox", "")
	var items []store.InboxItem
	if err := json.Unmarshal([]byte(body), &items); err != nil {
		t.Fatalf("GET /inbox = %s, want a list", body)
	}
	return items
}

func groupKinds(items []store.InboxItem) []string {
	out := []string{}
	for _, i := range items {
		out = append(out, i.Group+" "+i.Kind)
	}
	slices.Sort(out)
	return out
}

func TestInbox_EachCauseInItsGroupAndOnlyFYIDismisses(t *testing.T) {
	ctx := context.Background()
	env := newLiveEnv(t)
	for _, name := range []string{"capped", "held"} {
		do(t, http.MethodPost, env.url+"/spaces", fmt.Sprintf(`{"projectId":1,"name":%q}`, name))
	}
	env.failedTurn(t, "lead", "build it", nil)
	if err := env.st.PauseSpace(ctx, 2, _pauseReason); err != nil {
		t.Fatal(err)
	}
	msg, err := env.st.CreateMessage(ctx, store.Message{SpaceID: 3, Author: "lead", Body: "plan it"})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.st.PauseSpace(ctx, 3, "hop cap: 6 agent hops without a user message"); err != nil {
		t.Fatal(err)
	}
	if _, err := env.st.CreateHold(ctx,
		store.Hold{SpaceID: 3, From: "lead", To: "architect", MessageID: msg.ID, Text: "plan it"}); err != nil {
		t.Fatal(err)
	}
	do(t, http.MethodPost, env.url+"/work-items", `{"projectId":1,"title":"Login","teamId":1}`)
	env.exec(t, "UPDATE work_item SET status = 'review' WHERE id = 1")
	do(t, http.MethodPost, env.url+"/work-items", `{"projectId":1,"title":"Logout","teamId":1}`)
	do(t, http.MethodPatch, env.url+"/work-items/2", `{"status":"queued"}`)
	env.waitIdle(t, 4)
	tok, err := env.st.CreateToken(ctx, 4, "lead")
	if err != nil {
		t.Fatal(err)
	}
	call := callTool(t, env.st, tok)
	if got, isErr := call("create_sub_item", map[string]any{"title": "Schema"}); isErr {
		t.Fatalf("create_sub_item = %q", got)
	}
	if got, isErr := call("propose_work_item", map[string]any{
		"type": "bug", "title": "Slow deploy", "description": "It takes ten minutes.",
	}); isErr {
		t.Fatalf("propose_work_item = %q", got)
	}

	want := []string{
		"fyi proposed", "fyi started", "fyi sub_item",
		"needs_you failed", "needs_you held", "needs_you paused", "review review",
	}
	if got := groupKinds(env.inboxItems(t)); !slices.Equal(got, want) {
		t.Fatalf("inbox = %q, want %q", got, want)
	}
	texts := map[string]string{}
	for _, i := range env.inboxItems(t) {
		texts[i.Kind] = i.Text
	}
	for kind, text := range map[string]string{
		"held":     "hop cap: 6 agent hops without a user message (1 wake held)",
		"review":   "Login",
		"started":  "Started from the queue: Logout",
		"sub_item": "lead added sub-item 3 to work item 2: Schema",
		"proposed": "lead proposed work item 4: Slow deploy. It is in the Backlog until you queue it. Suggested team: solo.",
	} {
		if texts[kind] != text {
			t.Errorf("%s text = %q, want %q", kind, texts[kind], text)
		}
	}

	for _, kind := range []string{"paused", "held", "failed", "review"} {
		status, body := do(t, http.MethodPost, env.url+"/inbox/dismiss", fmt.Sprintf(`{"kind":%q,"spaceId":2}`, kind))
		if wantBody := "a " + kind + " item cannot be dismissed"; status != 409 || !strings.Contains(body, wantBody) {
			t.Errorf("dismiss %s = %d %s, want 409 with %q", kind, status, body, wantBody)
		}
	}
	if got := groupKinds(env.inboxItems(t)); !slices.Equal(got, want) {
		t.Errorf("inbox after refused dismissals = %q, want %q", got, want)
	}

	for _, i := range env.inboxItems(t) {
		if i.Kind != "proposed" {
			continue
		}
		raw, _ := json.Marshal(i)
		if status, body := do(t, http.MethodPost, env.url+"/inbox/dismiss", string(raw)); status != 204 {
			t.Fatalf("dismiss proposed = %d %s, want 204", status, body)
		}
		if status, _ := do(t, http.MethodPost, env.url+"/inbox/dismiss", string(raw)); status != 404 {
			t.Errorf("dismiss proposed again = %d, want 404", status)
		}
	}
	want = slices.DeleteFunc(want, func(s string) bool { return s == "fyi proposed" })
	if got := groupKinds(env.inboxItems(t)); !slices.Equal(got, want) {
		t.Errorf("inbox after dismissing = %q, want %q", got, want)
	}
	env.restart(t)
	if got := groupKinds(env.inboxItems(t)); !slices.Equal(got, want) {
		t.Errorf("inbox after a restart = %q, want %q", got, want)
	}
}
