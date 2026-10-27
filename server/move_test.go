package server

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/bhanuprakaash/reef/store"
)

// conversation adds the Teammates sam (2) and kit (3) and a direct conversation with sam
// (Space 2). Sam answers a request for a summary with a message.
func (e *liveEnv) conversation(t *testing.T) {
	t.Helper()
	for _, name := range []string{"sam", "kit"} {
		if status, body := do(t, http.MethodPost, e.url+"/teammates",
			`{"name":"`+name+`","backend":"claude"}`); status != 201 {
			t.Fatalf("create %s = %d %s", name, status, body)
		}
	}
	if status, body := do(t, http.MethodPost, e.url+"/spaces",
		`{"projectId":1,"name":"talk","teammateId":2,"kind":"conversation"}`); status != 201 {
		t.Fatalf("create conversation = %d %s", status, body)
	}
	e.fake.onRun = func(role, text string, _ *int64) {
		if strings.HasPrefix(text, "The user is adding @kit") {
			_, err := e.st.CreateMessage(context.Background(), store.Message{
				SpaceID: 2, Author: role, Body: "Ada wants a login page; the design is open."})
			if err != nil {
				t.Error(err)
			}
		}
	}
}

func (e *liveEnv) waitRan(t *testing.T, n int) []string {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		if ran := e.ranTexts(); len(ran) >= n {
			return ran
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("turns run = %q, want %d", e.ranTexts(), n)
	return nil
}

func TestSend_AtTeammateMovesConversation(t *testing.T) {
	env := newLiveEnv(t)
	env.conversation(t)

	if status, body := env.sendTo(t, 2, "plum-31 is the codeword.\\n@kit take over the login page"); status != 202 {
		t.Fatalf("send = %d %s, want 202", status, body)
	}
	ran := env.waitRan(t, 2)
	if want := "[Space 3, message 3]\nsam wrote: @kit Ada wants a login page; the design is open."; ran[1] != want {
		t.Errorf("kit's wake = %q, want %q", ran[1], want)
	}
	if strings.Contains(ran[1], "plum-31") || strings.Contains(ran[1], "take over") {
		t.Errorf("kit's wake = %q, want no line of the conversation", ran[1])
	}
	env.waitIdle(t, 2)

	_, body := do(t, http.MethodGet, env.url+"/spaces/2", "")
	if !strings.Contains(body, `"movedTo":3`) {
		t.Errorf("conversation = %s, want movedTo 3", body)
	}
	_, body = do(t, http.MethodGet, env.url+"/spaces/3/members", "")
	if !strings.Contains(body, `"name":"sam","lead":true`) || !strings.Contains(body, `"name":"kit","lead":false`) {
		t.Errorf("members of Space 3 = %s, want sam leading and kit", body)
	}
	if status, body := env.sendTo(t, 2, "more"); status != 409 ||
		body != `{"error":"conversation 2 moved to Space 3: conflict"}` {
		t.Errorf("send to the moved conversation = %d %s, want 409 moved to Space 3", status, body)
	}
}

func TestSend_AtTeammateInsideLineMovesNothing(t *testing.T) {
	env := newLiveEnv(t)
	env.conversation(t)

	if status, body := env.sendTo(t, 2, "ask @kit about the login page"); status != 202 {
		t.Fatalf("send = %d %s, want 202", status, body)
	}
	ran := env.waitRan(t, 1)
	env.waitIdle(t, 2)
	if len(ran) != 1 || ran[0] != "[Space 2, message 1]\nask @kit about the login page" {
		t.Errorf("turns run = %q, want one ordinary turn of sam", ran)
	}
	_, body := do(t, http.MethodGet, env.url+"/spaces", "")
	if strings.Contains(body, `"movedTo":2`) || strings.Count(body, `"kind":`) != 2 {
		t.Errorf("spaces = %s, want two and none moved", body)
	}
}

func TestAddSpaceMember_MovesConversation(t *testing.T) {
	env := newLiveEnv(t)
	env.conversation(t)

	if status, body := do(t, http.MethodPost, env.url+"/spaces/2/members", `{"teammateId":2}`); status != 400 {
		t.Errorf("add the lead = %d %s, want 400", status, body)
	}
	if status, body := do(t, http.MethodPost, env.url+"/spaces/2/members", `{"teammateId":3}`); status != 202 {
		t.Fatalf("add kit = %d %s, want 202", status, body)
	}
	env.waitRan(t, 2)
	env.waitIdle(t, 2)
	if ch, err := env.st.Space(context.Background(), 2); err != nil || ch.MovedTo == nil || *ch.MovedTo != 3 {
		t.Errorf("conversation = %+v, %v, want moved to Space 3", ch, err)
	}
}
