package channel

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/store"
)

func TestWakeAs_ChangedSinceLastTurn(t *testing.T) {
	ctx := context.Background()
	e := newWakeEnv(t)
	fid := *e.ch.FeatureID
	prompt := func() string {
		a := e.agents[len(e.agents)-1]
		return a.prompts[len(a.prompts)-1]
	}
	say := func(body string) store.Scope {
		return store.Scope{Read: e.say("user", body, nil)}
	}
	decide := func(author, body string, replaces ...int64) store.Decision {
		t.Helper()
		d, err := e.st.CreateDecision(ctx, store.Decision{
			FeatureID: fid, Body: body, Author: author, Replaces: replaces,
		})
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	check := func(step, want string) {
		t.Helper()
		if got := prompt(); got != want {
			t.Errorf("%s: prompt =\n%q\nwant\n%q", step, got, want)
		}
	}

	e.wake("developer", "one", nil, say("one"))
	if got := prompt(); !strings.HasPrefix(got, "## You\n") {
		t.Fatalf("first prompt = %q, want the first prompt, with no delta", got)
	}

	d1 := decide("lead", "Use SQLite, not Postgres\nit is one file")
	decide("developer", "Name the table turn")
	e.wake("developer", "two", nil, say("two"))
	check("a decision of another role, not the developer's own",
		"### Changed since your last turn\n"+
			"decision 1: Use SQLite, not Postgres (lead, "+d1.CreatedAt.Format("2006-01-02")+")\n\n"+
			"[channel 1, message 3]\ntwo")

	e.wake("developer", "three", nil, say("three"))
	check("nothing changed", "[channel 1, message 5]\nthree")

	d3 := decide("lead", "Use WAL", d1.ID)
	if _, err := e.st.PutBrief(ctx, store.Brief{FeatureID: fid, Body: "Ship the store.", Author: "user"}); err != nil {
		t.Fatal(err)
	}
	e.wake("developer", "four", nil, say("four"))
	check("a retirement and a new brief",
		"### Changed since your last turn\n"+
			"decision 3: Use WAL (lead, "+d3.CreatedAt.Format("2006-01-02")+")\n"+
			"decision 1 is retired, replaced by decision 3.\n"+
			"### New brief\nShip the store.\n\n"+
			"[channel 1, message 7]\nfour")

	e.wake("developer", "five", nil, say("five"))
	check("the brief is sent once", "[channel 1, message 9]\nfive")

	read := store.Scope{Read: 9}
	decide("lead", "Use JSON")
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
		"### Changed since your last turn\n"+
			"decision 4: Use JSON (lead, "+d3.CreatedAt.Format("2006-01-02")+")\n\n"+
			"[channel 1, message 12]\nNew in the channel:\nuser: six\n\nseven")
}
