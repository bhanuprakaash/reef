package server

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bhanuprakaash/reef/store"
)

const _work = 4

// sendEnv is consultEnv with a work Space (4) that Ben is a member of, referred to by Ada's chat.
func newSendEnv(t *testing.T) *consultEnv {
	t.Helper()
	e := newConsultEnv(t)
	if status, body := do(t, http.MethodPost, e.url+"/spaces", `{"projectId":1,"name":"work"}`); status != 201 {
		t.Fatalf("create Space = %d %s", status, body)
	}
	if status, body := do(t, http.MethodPost, e.url+"/spaces/4/members", fmt.Sprintf(`{"botId":%d}`, e.ben)); status >= 300 {
		t.Fatalf("add Ben = %d %s", status, body)
	}
	if status, body := do(t, http.MethodPost, e.url+"/bots", `{"name":"cy","backend":"claude"}`); status != 201 {
		t.Fatalf("create cy = %d %s", status, body)
	}
	msg, err := e.st.CreateMessage(context.Background(), store.Message{
		SpaceID: _adaChat, Author: "user", Body: "see #[work](space:4)"})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.st.SaveMessageRefs(context.Background(), msg.ID, store.ParseSpaceRefs(msg.Body)); err != nil {
		t.Fatal(err)
	}
	return e
}

// send makes Ada call send_to_space with each of calls and returns the tool results.
func (e *consultEnv) send(t *testing.T, n int, calls string) []string {
	t.Helper()
	e.mu.Lock()
	e.results = nil
	e.mu.Unlock()
	e.say(t, "send:"+calls)
	for {
		e.mu.Lock()
		got := slices.Clone(e.results)
		e.mu.Unlock()
		if len(got) >= n {
			for id := range int64(8) {
				e.waitIdle(t, int(id))
			}
			return got
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (e *consultEnv) sessions(t *testing.T, spaceID int64) int {
	t.Helper()
	var n int
	if err := e.st.DB().QueryRow("SELECT COUNT(*) FROM bot_session WHERE space_id = ?", spaceID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestSendToSpaceQuestionAnswersOnceInExistingSession(t *testing.T) {
	e := newSendEnv(t)
	if _, err := e.st.AdoptBotSession(context.Background(), _work, e.ben); err != nil {
		t.Fatal(err)
	}

	got := e.send(t, 2, `[{"spaceId":4,"bot":"ben","kind":"question","body":"why?"},{"spaceId":4,"bot":"ben","kind":"question","body":"why?"}]`)
	if want := slices.Repeat([]string{`{"exchange":1,"state":"waiting"}`}, 2); !slices.Equal(got, want) {
		t.Fatalf("results = %q, want one exchange %q", got, want)
	}
	for !slices.Contains(e.lines(t, _adaChat), "ben: hi (exchange 1)") {
		time.Sleep(5 * time.Millisecond)
	}
	e.waitIdle(t, _work)
	e.waitIdle(t, _adaChat)

	var chat []string
	for _, l := range e.lines(t, _adaChat) {
		if !strings.HasPrefix(l, "user:") {
			chat = append(chat, l)
		}
	}
	if want := []string{"reef: Sent to ben in work: why? (exchange 1)", "ben: hi (exchange 1)"}; !slices.Equal(chat, want) {
		t.Errorf("chat = %q, want %q", chat, want)
	}
	if wake := "[Space 2, question to Space 4, exchange 1]\nben replied: hi"; !slices.Contains(e.ranTexts(), wake) {
		t.Errorf("ran = %q, want the wake of Ada %q", e.ranTexts(), wake)
	}
	if turns, sessions := e.turnCount(t, _work), e.sessions(t, _work); turns != 1 || sessions != 1 {
		t.Errorf("Space 4 has %d turns and %d sessions, want 1 and 1", turns, sessions)
	}
}

func TestSendToSpaceInstructionWaitsForPausedSpaceAndFailsVisibly(t *testing.T) {
	ctx := context.Background()
	e := newSendEnv(t)
	if err := e.st.PauseSpace(ctx, _work, "hold"); err != nil {
		t.Fatal(err)
	}

	got := e.send(t, 1, `[{"spaceId":4,"bot":"ben","kind":"instruction","body":"use sessions"}]`)
	if want := []string{`{"exchange":1,"state":"waiting"}`}; !slices.Equal(got, want) {
		t.Fatalf("results = %q, want %q", got, want)
	}
	if ex := e.exchange(t, 1); ex.Request.State != store.DirectWaiting || e.turnCount(t, _work) != 0 {
		t.Errorf("paused: state = %s, turns = %d, want waiting, 0", ex.Request.State, e.turnCount(t, _work))
	}
	if c, err := e.st.Space(ctx, _work); err != nil || c.Paused != "hold" {
		t.Errorf("Space 4 = %+v, %v, want still paused", c, err)
	}

	if status, body := do(t, http.MethodPost, e.url+"/spaces/4/continue", ""); status != 204 {
		t.Fatalf("continue = %d %s", status, body)
	}
	e.waitState(t, 1, store.DirectAnswered)
	e.waitIdle(t, _work)
	if n := e.turnCount(t, _work); n != 1 {
		t.Errorf("turns after continue = %d, want 1", n)
	}
	wantRan := "[Space 4, direct message 1, chain 1, source: direct request]\nada wrote privately: " +
		"Instruction from the user, passed on by ada from Space 2: use sessions"
	if ran := e.ranTexts(); !slices.Contains(ran, wantRan) {
		t.Errorf("ran = %q, want %q", ran, wantRan)
	}
	if ran := e.ranTexts(); slices.ContainsFunc(ran, func(s string) bool { return strings.Contains(s, "replied") }) {
		t.Errorf("ran = %q, an instruction must not wake its sender", ran)
	}

	got = e.send(t, 2, `[{"spaceId":4,"bot":"cy","kind":"instruction","body":"x"},{"spaceId":2,"bot":"ben","kind":"question","body":"x"}]`)
	if got[0] != `{"exchange":3,"state":"waiting"}` || got[1] != "space 2: not found" {
		t.Fatalf("results = %q", got)
	}
	e.waitState(t, 3, store.DirectFailed)
	if err := e.st.CloseSpace(ctx, _work); err != nil {
		t.Fatal(err)
	}
	e.send(t, 1, `[{"spaceId":4,"bot":"ben","kind":"question","body":"late"}]`)
	ex := e.waitState(t, 5, store.DirectFailed)
	if ex.Response == nil || ex.Response.Body != "space 4 is closed: conflict" {
		t.Errorf("closed destination = %+v, want failed, closed", ex)
	}
	lines := e.lines(t, _adaChat)
	for _, want := range []string{
		"cy: No reply: failed, cy is not a member of Space 4 (exchange 3)",
		"ben: No reply: failed, space 4 is closed: conflict (exchange 5)",
	} {
		if !slices.Contains(lines, want) {
			t.Errorf("chat = %q, missing %q", lines, want)
		}
	}
}
