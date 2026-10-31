package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/store"
)

// TestDirectQueueOrderAndSingleConcurrency runs two direct wakes created during one lead
// turn, in stored order, and proves a direct turn never overlaps another (max concurrency 1)
// using deterministic Spaces instead of sleeps.
func TestDirectQueueOrderAndSingleConcurrency(t *testing.T) {
	ctx := context.Background()
	env := newLiveEnv(t)
	id, _ := env.teamSpace(t)
	env.fake.directRelease = make(chan struct{})
	var directsCreated atomic.Bool
	var started sync.Mutex
	startedCount := 0
	env.fake.onRun = func(role, text string, parent *int64) {
		if role == "lead" && directsCreated.CompareAndSwap(false, true) {
			if _, err := env.st.CreateDirectMessage(ctx, store.DirectMessage{
				SpaceID: int64(id), From: "lead", To: "architect", FromBotID: testServerBot(t, env.st, "lead"), ToBotID: testServerBot(t, env.st, "architect"), Body: "first",
			}); err != nil {
				t.Error(err)
			}
			if _, err := env.st.CreateDirectMessage(ctx, store.DirectMessage{
				SpaceID: int64(id), From: "lead", To: "architect", FromBotID: testServerBot(t, env.st, "lead"), ToBotID: testServerBot(t, env.st, "architect"), Body: "second",
			}); err != nil {
				t.Error(err)
			}
		}
	}
	sub := subscribeTo(t, env.url, id)
	env.sendTo(t, id, "go")

	// The first direct wake starts and blocks.
	select {
	case <-env.fake.directStart:
	case <-time.After(5 * time.Second):
		t.Fatal("first direct turn did not start")
	}
	started.Lock()
	startedCount++
	started.Unlock()

	// While the first direct turn is blocked, the second must not start.
	startedCh := make(chan struct{})
	go func() {
		select {
		case <-env.fake.directStart:
			close(startedCh)
		case <-time.After(250 * time.Millisecond):
		}
	}()
	select {
	case <-startedCh:
		t.Fatal("second direct turn started while the first was still running")
	case <-time.After(350 * time.Millisecond):
	}

	// Release the first, then the second must start.
	env.fake.directRelease <- struct{}{}
	select {
	case <-env.fake.directStart:
	case <-time.After(5 * time.Second):
		t.Fatal("second direct turn did not start after the first released")
	}
	started.Lock()
	startedCount++
	started.Unlock()
	env.fake.directRelease <- struct{}{}

	// Both requests must have a done response, in stored order.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		direct, _ := env.st.DirectMessages(ctx, int64(id), 0)
		if len(direct) == 4 && direct[2].Status == store.DirectDone && direct[3].Status == store.DirectDone {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	direct, err := env.st.DirectMessages(ctx, int64(id), 0)
	if err != nil || len(direct) != 4 {
		t.Fatalf("direct = %+v, %v", direct, err)
	}
	if direct[0].Status != store.DirectSent || direct[0].From != "lead" || direct[0].To != "architect" || direct[0].Body != "first" {
		t.Fatalf("first request = %+v, want sent lead->architect 'first'", direct[0])
	}
	if direct[1].Status != store.DirectSent || direct[1].From != "lead" || direct[1].To != "architect" || direct[1].Body != "second" {
		t.Fatalf("second request = %+v, want sent lead->architect 'second'", direct[1])
	}
	if direct[2].Status != store.DirectDone || direct[2].From != "architect" || direct[2].To != "lead" ||
		direct[2].ReplyToID == nil || *direct[2].ReplyToID != direct[0].ID {
		t.Fatalf("first response = %+v, want done architect->lead replying to %d", direct[2], direct[0].ID)
	}
	if direct[3].Status != store.DirectDone || direct[3].From != "architect" || direct[3].To != "lead" ||
		direct[3].ReplyToID == nil || *direct[3].ReplyToID != direct[1].ID {
		t.Fatalf("second response = %+v, want done architect->lead replying to %d", direct[3], direct[1].ID)
	}
	if got := directRanOrder(env, "first", "second"); !got {
		t.Fatalf("direct turns did not run in stored order: %q", env.ranTexts())
	}
	env.fake.directMu.Lock()
	if got := env.fake.directMax; got != 1 {
		t.Errorf("max direct concurrency = %d, want 1", got)
	}
	env.fake.directMu.Unlock()
	if startedCount != 2 {
		t.Fatalf("direct starts = %d, want 2", startedCount)
	}
	sub.want(t, _idleFrame)
}

// directRanOrder reports whether the texts containing a and b ran with a before b.
func directRanOrder(env *liveEnv, a, b string) bool {
	texts := env.ranTexts()
	ia, ib := -1, -1
	for i, r := range texts {
		if strings.Contains(r, a) && ia < 0 {
			ia = i
		}
		if strings.Contains(r, b) && ib < 0 {
			ib = i
		}
	}
	return ia >= 0 && ib >= 0 && ia < ib
}

// TestDirectStopWhileQueuedRecipientStopped stops a Space while a direct wake is queued:
// the recipient is interrupted, never completes, and the sender sees a failed response while
// the request stays sent (terminal).
func TestDirectStopWhileQueuedRecipientStopped(t *testing.T) {
	ctx := context.Background()
	env := newLiveEnv(t)
	id, post := env.teamSpace(t)
	env.fake.directRelease = make(chan struct{})
	env.fake.onRun = func(role, text string, parent *int64) {
		if role == "lead" {
			post("lead", "@architect plan", nil)
			if _, err := env.st.CreateDirectMessage(ctx, store.DirectMessage{
				SpaceID: int64(id), From: "lead", To: "architect", FromBotID: testServerBot(t, env.st, "lead"), ToBotID: testServerBot(t, env.st, "architect"), Body: "queued direct",
			}); err != nil {
				t.Error(err)
			}
		}
	}
	sub := subscribeTo(t, env.url, id)
	env.sendTo(t, id, "go")
	// Wait for the architect direct turn to start (queued and blocked).
	select {
	case <-env.fake.directStart:
	case <-time.After(5 * time.Second):
		t.Fatal("direct wake did not start")
	}
	do(t, http.MethodPost, fmt.Sprintf("%s/spaces/%d/stop", env.url, id), "")
	// The interrupted direct turn leaves a failed response; the request stays sent.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		direct, _ := env.st.DirectMessages(ctx, int64(id), 0)
		if len(direct) == 2 && direct[1].Status == store.DirectFailed {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	direct, err := env.st.DirectMessages(ctx, int64(id), 0)
	if err != nil || len(direct) != 2 {
		t.Fatalf("direct = %+v, %v", direct, err)
	}
	if direct[0].Status != store.DirectSent {
		t.Fatalf("request status = %s, want sent", direct[0].Status)
	}
	if direct[1].Status != store.DirectFailed || direct[1].From != "architect" || direct[1].To != "lead" ||
		direct[1].ReplyToID == nil || *direct[1].ReplyToID != direct[0].ID {
		t.Fatalf("response = %+v, want failed architect->lead replying to %d", direct[1], direct[0].ID)
	}
	sub.want(t, _idleFrame)
}

// waitHolds is the Space's hold count once it has any: a cap pause is stored just before
// its holds, both under the chain's lock.
func waitHolds(t *testing.T, dir string, spaceID int64) int {
	t.Helper()
	n := 0
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if n = countHolds(t, dir, spaceID); n != 0 {
			break
		}
	}
	return n
}

// TestDirectHopCapHoldThenContinue drives direct exchanges until the shared hop cap pauses
// the Space with exactly one hold, then Continue runs the pending direct wake and preserves
// private identity (a direct response, not a public message).
func TestDirectHopCapHoldThenContinue(t *testing.T) {
	ctx := context.Background()
	env := newLiveEnv(t)
	id, _ := env.teamSpace(t)
	env.fake.onRun = func(role, text string, parent *int64) {
		if role == "lead" {
			if _, err := env.st.CreateDirectMessage(ctx, store.DirectMessage{
				SpaceID: int64(id), From: "lead", To: "architect", FromBotID: testServerBot(t, env.st, "lead"), ToBotID: testServerBot(t, env.st, "architect"), Body: "ping",
			}); err != nil {
				t.Error(err)
			}
		}
	}
	sub := subscribeTo(t, env.url, id)
	env.sendTo(t, id, "go")
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		ch, _ := env.st.Space(ctx, int64(id))
		if ch.Paused != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	ch, err := env.st.Space(ctx, int64(id))
	if err != nil || !strings.HasPrefix(ch.Paused, _hopReasonPrefix) {
		t.Fatalf("paused = %q, %v, want hop cap", ch.Paused, err)
	}
	if nHold := waitHolds(t, env.dir, int64(id)); nHold != 1 {
		t.Fatalf("holds = %d, want exactly one", nHold)
	}
	// Block the held direct wake so we can prove Continue started it.
	env.fake.directRelease = make(chan struct{})
	if status, _ := do(t, http.MethodPost, fmt.Sprintf("%s/spaces/%d/continue", env.url, id), ""); status != 204 {
		t.Fatalf("continue = %d, want 204", status)
	}
	select {
	case <-env.fake.directStart:
	case <-time.After(5 * time.Second):
		t.Fatal("continue did not run the held direct wake")
	}
	env.fake.directRelease <- struct{}{}
	// The held wake completes as a direct response: private identity preserved.
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		direct, _ := env.st.DirectMessages(ctx, int64(id), 0)
		if len(direct) >= 2 && direct[len(direct)-1].Status == store.DirectDone &&
			direct[len(direct)-1].From == "architect" && direct[len(direct)-1].To == "lead" &&
			direct[len(direct)-1].ReplyToID != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	direct, err := env.st.DirectMessages(ctx, int64(id), 0)
	if err != nil {
		t.Fatal(err)
	}
	var doneResponse bool
	for _, m := range direct {
		if m.Status == store.DirectDone && m.From == "architect" && m.To == "lead" && m.ReplyToID != nil {
			doneResponse = true
		}
	}
	if !doneResponse {
		t.Fatalf("direct = %+v, want a done architect->lead response", direct)
	}
	msgs, _ := env.st.Messages(ctx, int64(id), 0)
	for _, m := range msgs {
		if m.Author == "architect" {
			t.Fatalf("architect posted publicly: %s, direct must stay private", m.Body)
		}
	}
	sub.want(t, _idleFrame)
}

func TestDirectUserMessageFailsHeldWake(t *testing.T) {
	ctx := context.Background()
	env := newLiveEnv(t)
	id, _ := env.teamSpace(t)
	var ping atomic.Bool
	ping.Store(true)
	env.fake.onRun = func(role, _ string, _ *int64) {
		if role == "lead" && ping.Load() {
			if _, err := env.st.CreateDirectMessage(ctx, store.DirectMessage{
				SpaceID: int64(id), From: "lead", To: "architect", FromBotID: testServerBot(t, env.st, "lead"), ToBotID: testServerBot(t, env.st, "architect"), Body: "ping",
			}); err != nil {
				t.Error(err)
			}
		}
	}
	env.sendTo(t, id, "go")
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if ch, _ := env.st.Space(ctx, int64(id)); ch.Paused != "" {
			break
		}
	}
	env.waitIdle(t, id)
	before, err := env.st.DirectMessages(ctx, int64(id), 0)
	if err != nil {
		t.Fatal(err)
	}
	held := before[len(before)-1]
	if held.Body != "ping" || held.Status != store.DirectSent || held.ReplyToID != nil ||
		countHolds(t, env.dir, int64(id)) != 1 {
		t.Fatalf("last direct message = %+v, holds = %d, want the unanswered ping held once",
			held, countHolds(t, env.dir, int64(id)))
	}

	ping.Store(false)
	if status, body := env.sendTo(t, id, "more"); status != 202 || body != "" {
		t.Fatalf("send while hop paused = %d %q, want 202", status, body)
	}
	after, err := env.st.DirectMessages(ctx, int64(id), 0)
	if err != nil || len(after) != len(before)+1 {
		t.Fatalf("direct messages = %d, %v, want %d: one response for the held request",
			len(after), err, len(before)+1)
	}
	got := after[len(after)-1]
	if got.Status != store.DirectFailed || got.Body != "dropped" || got.From != "architect" ||
		got.To != "lead" || got.ReplyToID == nil || *got.ReplyToID != held.ID {
		t.Errorf("response = %+v, want failed architect->lead \"dropped\" replying to %d", got, held.ID)
	}
	env.waitIdle(t, id)
	if ran := env.ranTexts(); len(ran) != 8 || !strings.HasSuffix(ran[7], "\nmore") {
		t.Errorf("ran = %q, want 8 turns, the last the user's \"more\"", ran)
	}
}

// TestDirectTokenCapHoldThenContinue exhausts the token budget mid-chain so the pending
// direct wake is held, then Continue extends the budget and runs it.
func TestDirectTokenCapHoldThenContinue(t *testing.T) {
	ctx := context.Background()
	env := newLiveEnv(t)
	id, _ := env.teamSpace(t)
	env.fake.directRelease = make(chan struct{})
	exhausted := false
	env.fake.onRun = func(role, text string, parent *int64) {
		if role == "lead" && !exhausted {
			exhausted = true
			if _, err := env.st.CreateDirectMessage(ctx, store.DirectMessage{
				SpaceID: int64(id), From: "lead", To: "architect", FromBotID: testServerBot(t, env.st, "lead"), ToBotID: testServerBot(t, env.st, "architect"), Body: "ping",
			}); err != nil {
				t.Error(err)
			}
			turn, err := env.st.CreateTurn(ctx, store.Turn{
				SpaceID: int64(id), Role: "lead", BotID: testServerBot(t, env.st, "lead"), Backend: "claude",
			})
			if err != nil {
				t.Error(err)
				return
			}
			turn.Status = "done"
			turn.Usage = agent.Usage{InputTokens: store.TokenBudget}
			if _, err := env.st.EndTurn(ctx, turn); err != nil {
				t.Error(err)
			}
		}
	}
	sub := subscribeTo(t, env.url, id)
	env.sendTo(t, id, "go")
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		ch, _ := env.st.Space(ctx, int64(id))
		if ch.Paused != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	ch, err := env.st.Space(ctx, int64(id))
	if err != nil || !strings.HasPrefix(ch.Paused, "token budget") {
		t.Fatalf("paused = %q, %v, want token budget pause", ch.Paused, err)
	}
	if nHold := waitHolds(t, env.dir, int64(id)); nHold != 1 {
		t.Fatalf("holds = %d, want exactly one", nHold)
	}
	if status, _ := do(t, http.MethodPost, fmt.Sprintf("%s/spaces/%d/continue", env.url, id), ""); status != 204 {
		t.Fatalf("continue = %d, want 204", status)
	}
	select {
	case <-env.fake.directStart:
	case <-time.After(5 * time.Second):
		t.Fatal("continue did not run the held direct wake after budget extension")
	}
	env.fake.directRelease <- struct{}{}
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		direct, _ := env.st.DirectMessages(ctx, int64(id), 0)
		if len(direct) >= 2 && direct[len(direct)-1].Status == store.DirectDone &&
			direct[len(direct)-1].From == "architect" && direct[len(direct)-1].To == "lead" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	direct, _ := env.st.DirectMessages(ctx, int64(id), 0)
	var doneResponse bool
	for _, m := range direct {
		if m.Status == store.DirectDone && m.From == "architect" && m.To == "lead" {
			doneResponse = true
		}
	}
	if !doneResponse {
		t.Fatalf("direct = %+v, want a done architect->lead response", direct)
	}
	sub.want(t, _idleFrame)
}

// countHolds opens a separate read connection to the live store's database file and counts
// the Space's pending holds.
func countHolds(t *testing.T, dir string, spaceID int64) int {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+dir+"/reef.db?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var n int
	if err := db.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM approval WHERE space_id = ? AND kind = ?",
		spaceID, "hold").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// TestDirectRecipientStartFailure makes the recipient's direct turn fail to start and checks
// the failed response is sender-visible while the request stays sent.
func TestDirectRecipientStartFailure(t *testing.T) {
	ctx := context.Background()
	env := newLiveEnv(t)
	id, post := env.teamSpace(t)
	env.fake.directFail = errors.New("recipient failed to start")
	env.fake.onRun = func(role, text string, parent *int64) {
		if role == "lead" {
			post("lead", "@architect plan", nil)
			if _, err := env.st.CreateDirectMessage(ctx, store.DirectMessage{
				SpaceID: int64(id), From: "lead", To: "architect", FromBotID: testServerBot(t, env.st, "lead"), ToBotID: testServerBot(t, env.st, "architect"), Body: "do it",
			}); err != nil {
				t.Error(err)
			}
		}
	}
	sub := subscribeTo(t, env.url, id)
	env.sendTo(t, id, "go")
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		direct, _ := env.st.DirectMessages(ctx, int64(id), 0)
		if len(direct) == 2 && direct[1].Status == store.DirectFailed {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	direct, err := env.st.DirectMessages(ctx, int64(id), 0)
	if err != nil || len(direct) != 2 {
		t.Fatalf("direct = %+v, %v", direct, err)
	}
	if direct[0].Status != store.DirectSent || direct[0].From != "lead" || direct[0].To != "architect" {
		t.Fatalf("request = %+v, want sent lead->architect", direct[0])
	}
	if direct[1].Status != store.DirectFailed || direct[1].From != "architect" || direct[1].To != "lead" ||
		direct[1].ReplyToID == nil || *direct[1].ReplyToID != direct[0].ID {
		t.Fatalf("response = %+v, want failed architect->lead replying to %d", direct[1], direct[0].ID)
	}
	sub.want(t, _idleFrame)
}

// TestDirectTurnFailureMarksQueuedWakesFailed runs two direct wakes in one discovery where
// the recipient's turn fails: each request ends with a failed response naming it, so no
// direct message the chain never ran is left silently sent.
func TestDirectTurnFailureMarksQueuedWakesFailed(t *testing.T) {
	ctx := context.Background()
	env := newLiveEnv(t)
	id, _ := env.teamSpace(t)
	env.fake.directFail = errors.New("recipient failed")
	var directsCreated atomic.Bool
	env.fake.onRun = func(role, text string, parent *int64) {
		if role == "lead" && directsCreated.CompareAndSwap(false, true) {
			for _, body := range []string{"first", "second"} {
				if _, err := env.st.CreateDirectMessage(ctx, store.DirectMessage{
					SpaceID: int64(id), From: "lead", To: "architect", FromBotID: testServerBot(t, env.st, "lead"), ToBotID: testServerBot(t, env.st, "architect"), Body: body,
				}); err != nil {
					t.Error(err)
				}
			}
		}
	}
	sub := subscribeTo(t, env.url, id)
	env.sendTo(t, id, "go")
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		direct, _ := env.st.DirectMessages(ctx, int64(id), 0)
		if len(direct) == 4 && direct[1].Status == store.DirectFailed &&
			direct[3].Status == store.DirectFailed {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	direct, err := env.st.DirectMessages(ctx, int64(id), 0)
	if err != nil || len(direct) != 4 {
		t.Fatalf("direct = %+v, %v, want two requests and two failed responses", direct, err)
	}
	if direct[0].Status != store.DirectSent || direct[0].Body != "first" ||
		direct[1].Status != store.DirectSent || direct[1].Body != "second" {
		t.Fatalf("requests = %+v, want both sent", direct)
	}
	for i, want := range []int64{direct[0].ID, direct[1].ID} {
		if f := direct[2+i]; f.Status != store.DirectFailed || f.From != "architect" ||
			f.To != "lead" || f.ReplyToID == nil || *f.ReplyToID != want {
			t.Fatalf("response %d = %+v, want failed architect->lead replying to %d", i, f, want)
		}
	}
	sub.want(t, _idleFrame)
}

// TestDirectChainEndClearsActiveTurn checks the active-turn hint the server stores: it is
// the running turn while the exchange runs and empty again when the chain ends on its own.
func TestDirectChainEndClearsActiveTurn(t *testing.T) {
	ctx := context.Background()
	env := newLiveEnv(t)
	id, _ := env.teamSpace(t)
	env.fake.directRelease = make(chan struct{})
	var directsCreated atomic.Bool
	env.fake.onRun = func(role, text string, parent *int64) {
		if role == "lead" && directsCreated.CompareAndSwap(false, true) {
			if _, err := env.st.CreateDirectMessage(ctx, store.DirectMessage{
				SpaceID: int64(id), From: "lead", To: "architect", FromBotID: testServerBot(t, env.st, "lead"), ToBotID: testServerBot(t, env.st, "architect"), Body: "design it",
			}); err != nil {
				t.Error(err)
			}
		}
	}
	sub := subscribeTo(t, env.url, id)
	env.sendTo(t, id, "go")
	select {
	case <-env.fake.directStart:
	case <-time.After(5 * time.Second):
		t.Fatal("direct turn did not start")
	}
	if ch, err := env.st.Space(ctx, int64(id)); err != nil || ch.ActiveTurn != store.ActiveTurnMessage {
		t.Fatalf("active turn while the chain runs = %q, %v, want message", ch.ActiveTurn, err)
	}
	close(env.fake.directRelease)
	sub.want(t, _idleFrame)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		ch, _ := env.st.Space(ctx, int64(id))
		if ch.ActiveTurn == "" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	ch, err := env.st.Space(ctx, int64(id))
	t.Fatalf("active turn after the chain ends = %q, %v, want empty", ch.ActiveTurn, err)
}

// TestDirectRestartRecovery simulates a crash with a running direct turn: on restart it
// becomes a sender-visible failed response, active_turn clears, and a missing original direct
// message does not block startup.
func TestDirectRestartRecovery(t *testing.T) {
	ctx := context.Background()
	env := newLiveEnv(t)
	id, _ := env.teamSpace(t)
	req, err := env.st.CreateDirectMessage(ctx, store.DirectMessage{
		SpaceID: int64(id), From: "lead", To: "architect", FromBotID: testServerBot(t, env.st, "lead"), ToBotID: testServerBot(t, env.st, "architect"), Body: "do",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.st.CreateTurn(ctx, store.Turn{
		SpaceID: int64(id), Role: "architect", BotID: testServerBot(t, env.st, "architect"), Backend: "claude",
		WakeText: "do", DirectMessageID: req.IDRef(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := env.st.SetActiveTurn(ctx, int64(id), store.ActiveTurnMessage); err != nil {
		t.Fatal(err)
	}
	// Also leave a turn referencing a direct message that no longer exists: recovery must
	// not block on it.
	db, err := sql.Open("sqlite", "file:"+env.dir+"/reef.db?_pragma=foreign_keys(OFF)&_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.ExecContext(ctx,
		"INSERT INTO turn (space_id, role, backend, status, started_at, wake_text, direct_message_id) "+
			"VALUES (?, ?, ?, ?, ?, ?, ?)",
		int64(id), "architect", "claude", "running", now, "gone", 999999); err != nil {
		t.Fatal(err)
	}
	if _, err := env.st.FailRunningTurns(ctx, nil); err != nil {
		t.Fatalf("recovery blocked: %v", err)
	}
	tr, err := env.st.Turn(ctx, 1)
	if err != nil || tr.Status != "failed" || tr.Error != "server restarted" {
		t.Fatalf("direct turn = %+v, %v, want failed server restarted", tr, err)
	}
	ch, err := env.st.Space(ctx, int64(id))
	if err != nil || ch.ActiveTurn != "" {
		t.Fatalf("active_turn = %q, want cleared", ch.ActiveTurn)
	}
	direct, err := env.st.DirectMessages(ctx, int64(id), 0)
	if err != nil || len(direct) != 2 {
		t.Fatalf("direct = %+v, %v", direct, err)
	}
	if direct[1].Status != store.DirectFailed || direct[1].From != "architect" || direct[1].To != "lead" {
		t.Fatalf("response = %+v, want failed architect->lead", direct[1])
	}
}

// TestDirectRejectSendDuringStarting rejects send_to_role while a chain is starting.
func TestDirectRejectSendDuringStarting(t *testing.T) {
	st, tok := mcpFixture(t)
	if err := st.SetActiveTurn(context.Background(), 1, store.ActiveTurnStarting); err != nil {
		t.Fatal(err)
	}
	got, isErr := callTool(t, st, tok)("send_to_role", map[string]any{"to": "developer", "body": "hi"})
	if !isErr || !strings.Contains(got, "starting") {
		t.Fatalf("send during starting = %q (error %v), want starting error", got, isErr)
	}
}

// TestDirectRejectSendDuringWrap rejects send_to_role while wrapping up.
func TestDirectRejectSendDuringWrap(t *testing.T) {
	st, tok := mcpFixture(t)
	if err := st.SetActiveTurn(context.Background(), 1, store.ActiveTurnWrap); err != nil {
		t.Fatal(err)
	}
	got, isErr := callTool(t, st, tok)("send_to_role", map[string]any{"to": "developer", "body": "hi"})
	if !isErr || !strings.Contains(got, "wrapping") {
		t.Fatalf("send during wrap = %q (error %v), want wrapping error", got, isErr)
	}
}

// TestDirectCloseRejectsSend rejects send_to_role on a closed Space.
func TestDirectCloseRejectsSend(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	if _, err := st.CreateProject(ctx, store.Project{Name: "reef", Path: "/tmp/reef"}); err != nil {
		t.Fatal(err)
	}
	one := int64(1)
	if _, err := st.CreateWorkItem(ctx, store.WorkItem{ProjectID: 1, Title: "Auth"}); err != nil {
		t.Fatal(err)
	}
	if _, err := testServerTeam(t, st, ctx, store.Team{Name: "core", Roles: []store.Role{
		{Name: "lead", Lead: true, Backend: "claude"},
		{Name: "developer", Backend: "claude"},
	}}); err != nil {
		t.Fatal(err)
	}
	closeSpace(t, st, 1)
	ch, err := st.CreateSpace(ctx, store.Space{ProjectID: 1, WorkItemID: &one, Name: "team", Team: "core", Role: "lead"})
	if err != nil {
		t.Fatal(err)
	}
	tok, err := testToken(t, st, ctx, ch.ID, "lead")
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+dir+"/reef.db?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.ExecContext(ctx, "UPDATE space SET closed_at = ? WHERE id = ?",
		time.Now().UTC().Format(time.RFC3339Nano), ch.ID); err != nil {
		t.Fatal(err)
	}
	got, isErr := callTool(t, st, tok)("send_to_role", map[string]any{"to": "developer", "body": "hi"})
	if !isErr || !strings.Contains(got, "closed") {
		t.Fatalf("send on closed = %q (error %v), want closed error", got, isErr)
	}
}

// TestDirectSelfLoopPrevented rejects sending a direct message to oneself, exactly once.
func TestDirectSelfLoopPrevented(t *testing.T) {
	st, tok := mcpFixture(t)
	if _, err := st.CreateTurn(context.Background(), store.Turn{SpaceID: tok.SpaceID, Role: "architect", BotID: tok.BotID, Backend: "claude", WakeText: "go"}); err != nil {
		t.Fatal(err)
	}
	got, isErr := callTool(t, st, tok)("send_to_role", map[string]any{"to": "architect", "body": "hi"})
	if !isErr || !strings.Contains(got, "yourself") {
		t.Fatalf("self send = %q (error %v), want self error", got, isErr)
	}
}

// requestStates maps the id of each direct request in the Space to its state.
func requestStates(t *testing.T, env *liveEnv, id int) map[int64]string {
	t.Helper()
	direct, err := env.st.DirectMessages(context.Background(), int64(id), 0)
	if err != nil {
		t.Fatal(err)
	}
	states := map[int64]string{}
	for _, m := range direct {
		if m.ReplyToID == nil {
			states[m.ID] = m.State
		}
	}
	return states
}

// TestDirectResponseWakeDroppedFailsRequest holds the response wake of an answered request at
// the hop cap. The request reads answered while it is held and failed once a user message
// drops the wake.
func TestDirectResponseWakeDroppedFailsRequest(t *testing.T) {
	ctx := context.Background()
	env := newLiveEnv(t)
	id, post := env.teamSpace(t)
	var led atomic.Bool
	env.fake.onRun = func(role, _ string, _ *int64) {
		switch {
		case role == "lead" && !led.Swap(true):
			post("lead", "@architect go", nil)
		case role == "architect":
			if _, err := env.st.CreateDirectMessage(ctx, store.DirectMessage{
				SpaceID: int64(id), From: "architect", To: "lead", FromBotID: testServerBot(t, env.st, "architect"), ToBotID: testServerBot(t, env.st, "lead"), Body: "ping",
			}); err != nil {
				t.Error(err)
			}
		}
	}
	env.sendTo(t, id, "go")
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if ch, _ := env.st.Space(ctx, int64(id)); ch.Paused != "" {
			break
		}
	}
	env.waitIdle(t, id)
	if n := countHolds(t, env.dir, int64(id)); n != 1 {
		t.Fatalf("holds = %d, want the one response wake", n)
	}
	direct, err := env.st.DirectMessages(ctx, int64(id), 0)
	if err != nil {
		t.Fatal(err)
	}
	held := direct[len(direct)-1]
	if held.ReplyToID == nil || held.Status != store.DirectDone || held.To != "architect" {
		t.Fatalf("last direct message = %+v, want the done response held for the architect", held)
	}
	if got := requestStates(t, env, id)[*held.ReplyToID]; got != store.DirectAnswered {
		t.Errorf("request state while its response wake is held = %q, want answered", got)
	}

	if status, _ := env.sendTo(t, id, "more"); status != 202 {
		t.Fatalf("send while hop paused = %d, want 202", status)
	}
	env.waitIdle(t, id)
	if got := requestStates(t, env, id)[*held.ReplyToID]; got != store.DirectFailed {
		t.Errorf("request state after its response wake was dropped = %q, want failed", got)
	}
}

// TestDirectResponseWakeFailureFailsRequest fails the turn the response of a request wakes.
func TestDirectResponseWakeFailureFailsRequest(t *testing.T) {
	ctx := context.Background()
	env := newLiveEnv(t)
	id, _ := env.teamSpace(t)
	var runs atomic.Int32
	env.fake.onRun = func(role, _ string, _ *int64) {
		if role != "lead" {
			return
		}
		if runs.Add(1) == 1 {
			if _, err := env.st.CreateDirectMessage(ctx, store.DirectMessage{
				SpaceID: int64(id), From: "lead", To: "architect", FromBotID: testServerBot(t, env.st, "lead"), ToBotID: testServerBot(t, env.st, "architect"), Body: "ping",
			}); err != nil {
				t.Error(err)
			}
			return
		}
		env.fake.directFail = errors.New("boom")
	}
	env.sendTo(t, id, "go")
	env.waitIdle(t, id)
	if got := requestStates(t, env, id); len(got) != 1 || got[1] != store.DirectFailed {
		t.Errorf("request states = %v, want the one request failed", got)
	}
}
