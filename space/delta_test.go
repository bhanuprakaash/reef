package space

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/memory"
	"github.com/bhanuprakaash/reef/store"
)

func lastPrompt(e *wakeEnv) string {
	a := e.agents[len(e.agents)-1]
	return a.prompts[len(a.prompts)-1]
}

func wakeWork(e *wakeEnv) memory.Ref {
	return memory.Ref{Kind: memory.ScopeWorkItem, ID: strconv.FormatInt(*e.ch.WorkItemID, 10)}
}

func decide(t *testing.T, e *wakeEnv, author, body string, replaces ...int64) memory.Item {
	t.Helper()
	r, err := e.s.ms.Remember(context.Background(),
		memory.Writer{Author: author, Role: author, Scope: wakeWork(e)},
		memory.Note{Kind: memory.KindDecision, Body: body, Replaces: replaces})
	if err != nil {
		t.Fatal(err)
	}
	return r.Item
}

func TestWakeAs_ChangedSinceLastTurn(t *testing.T) {
	ctx := context.Background()
	e := newWakeEnv(t)
	fid := *e.ch.WorkItemID
	day := time.Now().UTC().Format(_dateLayout)
	say := func(body string) store.Scope {
		return store.Scope{Read: e.say("user", body, nil)}
	}
	check := func(step, want string) {
		t.Helper()
		if time.Now().UTC().Format(_dateLayout) != day {
			t.Skip("test ran across midnight UTC")
		}
		if got := lastPrompt(e); got != want {
			t.Errorf("%s: prompt =\n%q\nwant\n%q", step, got, want)
		}
	}

	e.wake("developer", "one", nil, say("one"))
	if got := lastPrompt(e); !strings.HasPrefix(got, "## You\n") {
		t.Fatalf("first prompt = %q, want the first prompt, with no delta", got)
	}

	d1 := decide(t, e, "architect", "Use SQLite, not Postgres\nit is one file")
	remember(t, e.s.ms, projectScope(store.WorkItem{ProjectID: e.ch.ProjectID}),
		memory.KindFact, "lead", "Builds need cgo off")
	remember(t, e.s.ms, botScope(strconv.FormatInt(*testBotID(t, e.st, "developer"), 10)), memory.KindFinding, "developer", "Name the table turn")
	remember(t, e.s.ms, botScope("tester"), memory.KindFact, "tester", "Tester only")
	if _, err := e.s.ms.Remember(ctx,
		memory.Writer{Author: "lead", Role: "lead", Scope: wakeWork(e)},
		memory.Note{Kind: memory.KindDecision, Body: "Maybe use Redis", NeedsApproval: true}); err != nil {
		t.Fatal(err)
	}
	e.wake("developer", "two", nil, say("two"))
	check("items of every author in the role's scopes, no tester item, no candidate",
		"[Space 1, message 3]\n"+
			"### Changed since your last turn\n"+
			"decision 1: Use SQLite, not Postgres (architect, "+day+")\n"+
			"fact 2: Builds need cgo off (lead, "+day+")\n"+
			"finding 3: Name the table turn (developer, "+day+")\n\n"+
			"two")

	e.wake("developer", "three", nil, say("three"))
	check("nothing changed", "[Space 1, message 5]\nthree")

	decide(t, e, "lead", "Use WAL", d1.ID)
	if _, err := e.st.PutBrief(ctx, store.Brief{WorkItemID: fid, Body: "Ship the store.", Author: "user"}); err != nil {
		t.Fatal(err)
	}
	e.wake("developer", "four", nil, say("four"))
	check("a retirement and a new brief",
		"[Space 1, message 7]\n"+
			"### Changed since your last turn\n"+
			"decision 6: Use WAL (lead, "+day+")\n"+
			"decision 1 is retired, replaced by decision 6.\n\n"+
			"### New brief\nShip the store.\n\n"+
			"four")

	e.wake("developer", "five", nil, say("five"))
	check("the brief is sent once", "[Space 1, message 9]\nfive")

	read := store.Scope{Read: 9}
	decide(t, e, "lead", "Use JSON")
	if got := e.wake("developer", "five again", nil, read); got != "" {
		t.Errorf("a wake with nothing unread started a turn for a change alone: %q", got)
	}

	e.sendErr = errors.New("write prompt: broken pipe")
	if err := e.s.WakeAs(ctx, "developer", "six", store.Scopes{say("six")}, nil, nil, discard); err == nil {
		t.Fatal("wake with a failing send succeeded")
	}
	e.sendErr = nil
	e.wake("developer", "six", nil, say("seven"))
	check("a turn that failed before it sent did not tell the agent",
		"[Space 1, message 12]\n"+
			"### Changed since your last turn\n"+
			"decision 7: Use JSON (lead, "+day+")\n\n"+
			"New in the Space:\nuser: six\n\nseven")
}

func TestWakeAs_ItemAddedAndRetiredSinceShowsOnlyAsRetired(t *testing.T) {
	e := newWakeEnv(t)
	e.wake("developer", "one", nil, store.Scope{Read: e.say("user", "one", nil)})
	d1 := decide(t, e, "lead", "Use CSV")
	decide(t, e, "lead", "Use JSON", d1.ID)
	day := time.Now().UTC().Format(_dateLayout)

	e.wake("developer", "two", nil, store.Scope{Read: e.say("user", "two", nil)})

	want := "[Space 1, message 3]\n" +
		"### Changed since your last turn\n" +
		"decision 2: Use JSON (lead, " + day + ")\n" +
		"decision 1 is retired, replaced by decision 2.\n\n" +
		"two"
	if time.Now().UTC().Format(_dateLayout) != day {
		t.Skip("test ran across midnight UTC")
	}
	if got := lastPrompt(e); got != want {
		t.Errorf("prompt =\n%q\nwant\n%q", got, want)
	}
}

func TestWakeAs_CandidateAcceptedAfterTheTurnIsInTheNextDeltaWithWhatItReplaced(t *testing.T) {
	ctx := context.Background()
	e := newWakeEnv(t)
	d1 := decide(t, e, "lead", "Use CSV")
	cand, err := e.s.ms.Remember(ctx,
		memory.Writer{Author: "developer", Role: "developer", Scope: wakeWork(e)},
		memory.Note{Kind: memory.KindConvention, Body: "Use JSON", Replaces: []int64{d1.ID}})
	if err != nil || cand.Item.Status != memory.StatusCandidate {
		t.Fatalf("convention from a developer = %+v, %v, want a candidate", cand.Item, err)
	}
	e.wake("developer", "one", nil, store.Scope{Read: e.say("user", "one", nil)})
	if err := e.s.ms.Accept(ctx, cand.Item.ID); err != nil {
		t.Fatal(err)
	}
	day := time.Now().UTC().Format(_dateLayout)

	e.wake("developer", "two", nil, store.Scope{Read: e.say("user", "two", nil)})

	want := "[Space 1, message 3]\n" +
		"### Changed since your last turn\n" +
		"convention 2: Use JSON (developer, " + day + ")\n" +
		"decision 1 is retired, replaced by convention 2.\n\n" +
		"two"
	if time.Now().UTC().Format(_dateLayout) != day {
		t.Skip("test ran across midnight UTC")
	}
	if got := lastPrompt(e); got != want {
		t.Errorf("prompt =\n%q\nwant\n%q", got, want)
	}
}

func TestWakeAs_ItemRetiredWithNothingReplacingItIsNamedAlone(t *testing.T) {
	e := newWakeEnv(t)
	d1 := decide(t, e, "lead", "Use CSV")
	e.wake("developer", "one", nil, store.Scope{Read: e.say("user", "one", nil)})
	if err := e.s.ms.Retire(context.Background(), d1.ID); err != nil {
		t.Fatal(err)
	}

	e.wake("developer", "two", nil, store.Scope{Read: e.say("user", "two", nil)})

	want := "[Space 1, message 3]\n" +
		"### Changed since your last turn\n" +
		"decision 1 is retired.\n\n" +
		"two"
	if got := lastPrompt(e); got != want {
		t.Errorf("prompt =\n%q\nwant\n%q", got, want)
	}
}

func TestWakeAs_ItemRecordedDuringTheTurnThenReplacedIsNamedAsRetired(t *testing.T) {
	e := newWakeEnv(t)
	var d1 memory.Item
	e.onSend = func() { d1 = decide(t, e, "developer", "Use CSV") }
	e.wake("developer", "one", nil, store.Scope{Read: e.say("user", "one", nil)})
	e.onSend = nil
	decide(t, e, "lead", "Use JSON", d1.ID)
	day := time.Now().UTC().Format(_dateLayout)

	e.wake("developer", "two", nil, store.Scope{Read: e.say("user", "two", nil)})

	want := "[Space 1, message 3]\n" +
		"### Changed since your last turn\n" +
		"decision 2: Use JSON (lead, " + day + ")\n" +
		"decision 1 is retired, replaced by decision 2.\n\n" +
		"two"
	if time.Now().UTC().Format(_dateLayout) != day {
		t.Skip("test ran across midnight UTC")
	}
	if got := lastPrompt(e); got != want {
		t.Errorf("prompt =\n%q\nwant\n%q", got, want)
	}
}

func TestWakeAs_DecisionWrittenWhileAWriterHoldsTheTreeIsInTheFirstPrompt(t *testing.T) {
	ctx := context.Background()
	e := newWakeEnv(t)
	bot, err := e.st.Bot(ctx, *testBotID(t, e.st, "developer"))
	if err != nil {
		t.Fatal(err)
	}
	bot.Write = true
	if _, err = e.st.UpdateBot(ctx, bot); err != nil {
		t.Fatal(err)
	}
	dir, _, err := WorkDir(ctx, e.st, e.s.dataDir, e.ch.ID)
	if err != nil {
		t.Fatal(err)
	}
	unlock, err := e.s.trees.Lock(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	started := make(chan struct{})
	e.onStart = func() { close(started) }
	scopes := store.Scopes{{Read: e.say("user", "one", nil)}}
	done := make(chan error, 1)
	go func() { done <- e.s.WakeAs(ctx, "developer", "one", scopes, nil, nil, discard) }()

	select {
	case <-started:
	case err := <-done:
		t.Fatalf("wake ended before its agent started: %v", err)
	}
	remember(t, e.s.ms, memory.Ref{Kind: memory.ScopeWorkItem, ID: strconv.FormatInt(*e.ch.WorkItemID, 10)},
		memory.KindDecision, "lead", "Use WAL")
	day := time.Now().UTC().Format(_dateLayout)
	unlock()
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	want := "### Decisions\ndecision 1: Use WAL (lead, " + day + ")\n"
	if time.Now().UTC().Format(_dateLayout) != day {
		t.Skip("test ran across midnight UTC")
	}
	if got := lastPrompt(e); !strings.Contains(got, want) {
		t.Errorf("first prompt =\n%q\nwant it to contain %q", got, want)
	}
}

func TestWakeAs_MemberAddedBetweenAFailedFirstSendAndItsRetryIsInTheRetriedPrompt(t *testing.T) {
	ctx := context.Background()
	e := newWakeEnv(t)
	scopes := store.Scopes{{Read: e.say("user", "one", nil)}}
	e.sendErr = errors.New("write prompt: broken pipe")
	if err := e.s.WakeAs(ctx, "lead", "one", scopes, nil, nil, discard); err == nil {
		t.Fatal("wake with a failing send succeeded")
	}
	e.sendErr = nil
	tester, err := e.st.CreateBot(ctx, store.Bot{Name: "tester", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.st.AddSpaceMember(ctx, e.ch.ID, tester.ID); err != nil {
		t.Fatal(err)
	}

	e.wake("lead", "one", nil, scopes...)

	want := "Your team: @lead, @developer, @tester. Use post_to_space"
	if got := lastPrompt(e); !strings.Contains(got, want) {
		t.Errorf("retried first prompt =\n%q\nwant it to contain %q", got, want)
	}
}

func TestRunner_FailedPromptBuildOfDirectTurnLeavesFailedResponse(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch, err := st.CreateSpace(ctx, store.Space{
		ProjectID: f.ProjectID, WorkItemID: &f.ID, Name: "chat",
	})
	if err != nil {
		t.Fatal(err)
	}
	req, err := st.CreateDirectMessage(ctx, store.DirectMessage{
		SpaceID: ch.ID, From: "lead", To: "architect", FromBotID: testBotID(t, st, "lead"), ToBotID: testBotID(t, st, "architect"), Body: "plan",
	})
	if err != nil {
		t.Fatal(err)
	}
	ag := &fakeAgent{events: make(chan agent.Event, 8)}
	r := newTestRunner(t, st, ag, ch.ID, "architect", "claude", "")
	r.FirstPrompt = func(context.Context) (string, error) { return "", errors.New("build broke") }

	if err := r.RunDirect(ctx, "plan it", req, func(agent.Event) {}); err == nil {
		t.Fatal("RunDirect succeeded, want the build error")
	}

	direct, err := st.DirectMessages(ctx, ch.ID, 0)
	if err != nil || len(direct) != 2 || direct[1].Status != store.DirectFailed {
		t.Errorf("direct messages = %+v, %v, want a failed response", direct, err)
	}
	turns, err := st.Turns(ctx, store.TurnFilter{SpaceID: &ch.ID})
	if err != nil || len(turns) != 1 || turns[0].Status != "failed" || turns[0].Error != "build broke" ||
		len(ag.prompts) != 0 {
		t.Errorf("turns = %+v, %v, prompts = %q, want one failed turn and nothing sent", turns, err, ag.prompts)
	}
}

// The lead keeps its agent between turns; a specialist's is closed after each turn and resumed.
func TestWakeAs_CompactedSessionGetsTheFirstPromptAgain(t *testing.T) {
	for _, role := range []string{"lead", "developer"} {
		t.Run(role, func(t *testing.T) {
			ctx := context.Background()
			e := newWakeEnv(t)
			say := func(body string) store.Scope {
				return store.Scope{Read: e.say("user", body, nil)}
			}
			work := memory.Ref{Kind: memory.ScopeWorkItem, ID: strconv.FormatInt(*e.ch.WorkItemID, 10)}
			remember(t, e.s.ms, work, memory.KindDecision, "lead", "Use SQLite")
			compact := true
			e.onSend = func() {
				if compact {
					e.agents[0].events <- agent.Event{Kind: agent.KindCompacted}
				}
			}
			e.wake(role, "one", nil, say("one"))
			compact = false
			e.wake(role, "two", nil, say("two"))
			e.wake(role, "three", nil, say("three"))

			turns, err := e.st.Turns(ctx, store.TurnFilter{SpaceID: &e.ch.ID})
			if err != nil || len(turns) != 3 {
				t.Fatalf("turns = %d, %v, want 3", len(turns), err)
			}
			for _, section := range []string{"## You\n", "## How to work\n", "## What we know\n"} {
				if !strings.Contains(turns[1].Prompt, section) {
					t.Errorf("turn 2 prompt = %q, want %q: the session compacted in turn 1", turns[1].Prompt, section)
				}
			}
			if got := turns[2].Prompt; !strings.HasPrefix(got, "[Space 1, message 5]\n") ||
				strings.Contains(got, "## You\n") || strings.Contains(got, "## What we know\n") {
				t.Errorf("turn 3 prompt = %q, want a wake and at most a delta", got)
			}
		})
	}
}

func TestWakeAs_CompactionSeenBetweenTurnsIsKeptForARebuiltMember(t *testing.T) {
	ctx := context.Background()
	e := newWakeEnv(t)
	e.wake("lead", "one", nil, store.Scope{Read: e.say("user", "one", nil)})

	e.agents[0].events <- agent.Event{Kind: agent.KindCompacted}
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		compacted, err := e.st.SessionCompacted(ctx, e.ch.ID, e.s.leadID, "s1")
		if err != nil {
			t.Fatal(err)
		}
		if compacted {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the compaction the watch saw was not stored on the lead's newest turn")
		}
	}
	if err := e.s.closeMembers(); err != nil {
		t.Fatal(err)
	}
	e.wake("lead", "two", nil, store.Scope{Read: e.say("user", "two", nil)})

	if got := lastPrompt(e); !strings.HasPrefix(got, "## You\n") {
		t.Errorf("prompt of the rebuilt member = %q, want the first prompt", got)
	}
}
