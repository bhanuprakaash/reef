package channel

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

func decide(t *testing.T, e *wakeEnv, author, body string, replaces ...int64) store.Decision {
	t.Helper()
	d, err := e.st.CreateDecision(context.Background(), store.Decision{
		FeatureID: *e.ch.FeatureID, Body: body, Author: author, Replaces: replaces,
	})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestWakeAs_ChangedSinceLastTurn(t *testing.T) {
	ctx := context.Background()
	e := newWakeEnv(t)
	fid := *e.ch.FeatureID
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

	d1 := decide(t, e, "lead", "Use SQLite, not Postgres\nit is one file")
	decide(t, e, "developer", "Name the table turn")
	e.wake("developer", "two", nil, say("two"))
	check("a decision of another role, and one of the developer's own name",
		"[channel 1, message 3]\n"+
			"### Changed since your last turn\n"+
			"decision 1: Use SQLite, not Postgres (lead, "+day+")\n"+
			"decision 2: Name the table turn (developer, "+day+")\n\n"+
			"two")

	e.wake("developer", "three", nil, say("three"))
	check("nothing changed", "[channel 1, message 5]\nthree")

	decide(t, e, "lead", "Use WAL", d1.ID)
	if _, err := e.st.PutBrief(ctx, store.Brief{FeatureID: fid, Body: "Ship the store.", Author: "user"}); err != nil {
		t.Fatal(err)
	}
	e.wake("developer", "four", nil, say("four"))
	check("a retirement and a new brief",
		"[channel 1, message 7]\n"+
			"### Changed since your last turn\n"+
			"decision 3: Use WAL (lead, "+day+")\n"+
			"decision 1 is retired, replaced by decision 3.\n\n"+
			"### New brief\nShip the store.\n\n"+
			"four")

	e.wake("developer", "five", nil, say("five"))
	check("the brief is sent once", "[channel 1, message 9]\nfive")

	read := store.Scope{Read: 9}
	decide(t, e, "lead", "Use JSON")
	if got := e.wake("developer", "five again", nil, read); got != "" {
		t.Errorf("a wake with nothing unread started a turn for a change alone: %q", got)
	}

	e.sendErr = errors.New("write prompt: broken pipe")
	if err := e.s.WakeAs(ctx, "developer", "six", store.Scopes{say("six")}, nil, discard); err == nil {
		t.Fatal("wake with a failing send succeeded")
	}
	e.sendErr = nil
	e.wake("developer", "six", nil, say("seven"))
	check("a turn that failed before it sent did not tell the agent",
		"[channel 1, message 12]\n"+
			"### Changed since your last turn\n"+
			"decision 4: Use JSON (lead, "+day+")\n\n"+
			"New in the channel:\nuser: six\n\nseven")
}

func TestWakeAs_DecisionAddedAndRetiredSinceShowsOnlyAsRetired(t *testing.T) {
	e := newWakeEnv(t)
	e.wake("developer", "one", nil, store.Scope{Read: e.say("user", "one", nil)})
	d1 := decide(t, e, "lead", "Use CSV")
	decide(t, e, "lead", "Use JSON", d1.ID)
	day := time.Now().UTC().Format(_dateLayout)

	e.wake("developer", "two", nil, store.Scope{Read: e.say("user", "two", nil)})

	want := "[channel 1, message 3]\n" +
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

func TestWakeAs_DecisionRecordedDuringTheTurnThenReplacedIsNamedAsRetired(t *testing.T) {
	e := newWakeEnv(t)
	var d1 store.Decision
	e.onSend = func() { d1 = decide(t, e, "developer", "Use CSV") }
	e.wake("developer", "one", nil, store.Scope{Read: e.say("user", "one", nil)})
	e.onSend = nil
	decide(t, e, "lead", "Use JSON", d1.ID)
	day := time.Now().UTC().Format(_dateLayout)

	e.wake("developer", "two", nil, store.Scope{Read: e.say("user", "two", nil)})

	want := "[channel 1, message 3]\n" +
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
	if _, err := e.st.ApplyTeam(ctx, store.Team{Name: "core", Roles: []store.Role{
		{Name: "lead", Lead: true, Backend: "claude"},
		{Name: "developer", Backend: "claude", Write: true},
	}}); err != nil {
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
	go func() { done <- e.s.WakeAs(ctx, "developer", "one", scopes, nil, discard) }()

	select {
	case <-started:
	case err := <-done:
		t.Fatalf("wake ended before its agent started: %v", err)
	}
	remember(t, e.s.ms, memory.Ref{Kind: memory.ScopeWorkItem, ID: strconv.FormatInt(*e.ch.FeatureID, 10)},
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

func TestWakeAs_TeamEditedBetweenAFailedFirstSendAndItsRetryIsInTheRetriedPrompt(t *testing.T) {
	ctx := context.Background()
	e := newWakeEnv(t)
	scopes := store.Scopes{{Read: e.say("user", "one", nil)}}
	e.sendErr = errors.New("write prompt: broken pipe")
	if err := e.s.WakeAs(ctx, "lead", "one", scopes, nil, discard); err == nil {
		t.Fatal("wake with a failing send succeeded")
	}
	e.sendErr = nil
	if _, err := e.st.ApplyTeam(ctx, store.Team{Name: "core", Roles: []store.Role{
		{Name: "lead", Lead: true, Backend: "claude"},
		{Name: "developer", Backend: "claude"},
		{Name: "tester", Backend: "claude"},
	}}); err != nil {
		t.Fatal(err)
	}

	e.wake("lead", "one", nil, scopes...)

	want := "Your team: @lead, @developer, @tester. Use post_to_channel"
	if got := lastPrompt(e); !strings.Contains(got, want) {
		t.Errorf("retried first prompt =\n%q\nwant it to contain %q", got, want)
	}
}

func TestRunner_FailedPromptBuildOfDirectTurnLeavesFailedResponse(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch, err := st.CreateChannel(ctx, store.Channel{
		ProjectID: f.ProjectID, FeatureID: &f.ID, Name: "chat",
	})
	if err != nil {
		t.Fatal(err)
	}
	req, err := st.CreateDirectMessage(ctx, store.DirectMessage{
		ChannelID: ch.ID, From: "lead", To: "architect", Body: "plan",
	})
	if err != nil {
		t.Fatal(err)
	}
	ag := &fakeAgent{events: make(chan agent.Event, 8)}
	r := NewRunner(st, ag, ch.ID, "architect", "claude", "")
	r.FirstPrompt = func(context.Context) (string, error) { return "", errors.New("build broke") }

	if err := r.RunDirect(ctx, "plan it", req, func(agent.Event) {}); err == nil {
		t.Fatal("RunDirect succeeded, want the build error")
	}

	direct, err := st.DirectMessages(ctx, ch.ID, 0)
	if err != nil || len(direct) != 2 || direct[1].Status != store.DirectFailed {
		t.Errorf("direct messages = %+v, %v, want a failed response", direct, err)
	}
	turns, err := st.Turns(ctx, store.TurnFilter{ChannelID: &ch.ID})
	if err != nil || len(turns) != 1 || turns[0].Status != "failed" || turns[0].Error != "build broke" ||
		len(ag.prompts) != 0 {
		t.Errorf("turns = %+v, %v, prompts = %q, want one failed turn and nothing sent", turns, err, ag.prompts)
	}
}
