package channel

import (
	"context"
	"strconv"
	"testing"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/store"
)

type wakeAgent struct {
	session string
	fail    string
	prompts []string
	events  chan agent.Event
}

func (a *wakeAgent) Start(context.Context, agent.Config) error { return nil }

func (a *wakeAgent) Resume(_ context.Context, _ agent.Config, id string) error {
	a.session = id
	return nil
}

func (a *wakeAgent) Events() <-chan agent.Event { return a.events }
func (a *wakeAgent) Interrupt() error           { return nil }

func (a *wakeAgent) Send(_ context.Context, prompt string) error {
	a.prompts = append(a.prompts, prompt)
	if a.fail != "" {
		a.events <- agent.Event{Kind: agent.KindError, Text: a.fail}
		return nil
	}
	a.events <- agent.Event{Kind: agent.KindSession, SessionID: a.session}
	a.events <- agent.Event{Kind: agent.KindText, Text: "ok"}
	a.events <- agent.Event{Kind: agent.KindTurnEnd}
	return nil
}

type wakeEnv struct {
	t       *testing.T
	st      *store.Store
	ch      store.Channel
	s       *Session
	started int
	agents  []*wakeAgent
	fail    string
}

func newWakeEnv(t *testing.T) *wakeEnv {
	t.Helper()
	ctx := context.Background()
	t.Setenv("HOME", t.TempDir())
	st, f := openStore(t)
	if _, err := st.ApplyTeam(ctx, store.Team{Name: "core", Roles: []store.Role{
		{Name: "lead", Lead: true, Backend: "claude"},
		{Name: "developer", Backend: "claude"},
	}}); err != nil {
		t.Fatal(err)
	}
	ch, err := st.CreateChannel(ctx, store.Channel{
		ProjectID: f.ProjectID, FeatureID: &f.ID, Name: "chat", Role: "lead", Backend: "claude",
		Team: "core",
	})
	if err != nil {
		t.Fatal(err)
	}
	e := &wakeEnv{t: t, st: st, ch: ch}
	mk := func(string) (agent.Agent, error) {
		e.started++
		a := &wakeAgent{
			session: "s" + strconv.Itoa(e.started), fail: e.fail, events: make(chan agent.Event, 8),
		}
		e.agents = append(e.agents, a)
		return a, nil
	}
	e.s, err = open(ctx, st, t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), mk)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.s.Close() })
	return e
}

func (e *wakeEnv) say(author, body string, parent *int64) int64 {
	e.t.Helper()
	m, err := e.st.CreateMessage(context.Background(), store.Message{
		ChannelID: e.ch.ID, ParentID: parent, Author: author, Body: body,
	})
	if err != nil {
		e.t.Fatal(err)
	}
	return m.ID
}

func (e *wakeEnv) wake(role, routed string, parent *int64) string {
	e.t.Helper()
	ctx := context.Background()
	before, err := e.st.Turns(ctx, store.TurnFilter{ChannelID: &e.ch.ID})
	if err != nil {
		e.t.Fatal(err)
	}
	if err := e.s.WakeAs(ctx, role, routed, parent, discard); err != nil {
		e.t.Fatal(err)
	}
	turns, err := e.st.Turns(ctx, store.TurnFilter{ChannelID: &e.ch.ID})
	if err != nil {
		e.t.Fatal(err)
	}
	if len(turns) == len(before) {
		return ""
	}
	return turns[len(turns)-1].WakeText
}

func TestWakeAs_OnlyMessagesNotYetSent(t *testing.T) {
	e := newWakeEnv(t)
	root := e.say("user", "root", nil)
	e.say("lead", "@developer start", &root)
	first := "[channel 1, message 2, thread 1]\nlead wrote: @developer start"
	want := "[channel 1, message 2, thread 1]\nNew in the thread:\nuser: root\n\n" +
		"lead wrote: @developer start"
	if got := e.wake("developer", first, &root); got != want {
		t.Errorf("first wake =\n%q\nwant\n%q", got, want)
	}

	e.say("lead", "@developer next", &root)
	e.say("user", "and this", &root)
	next := "[channel 1, message 5, thread 1]\nuser: and this"
	want = "[channel 1, message 5, thread 1]\nNew in the thread:\nlead: @developer next\n\nand this"
	if got := e.wake("developer", next, &root); got != want {
		t.Errorf("second wake =\n%q\nwant\n%q: not the root, not the developer's own reply", got, want)
	}
}

func TestWakeAs_TwoMessagesOfOneThreadGiveOneCopy(t *testing.T) {
	e := newWakeEnv(t)
	root := e.say("user", "root", nil)
	e.say("lead", "@developer one", &root)
	e.say("lead", "@developer two", &root)
	routed := "[channel 1, message 2, thread 1]\nlead wrote: @developer one\n\n" +
		"[channel 1, message 3, thread 1]\nlead wrote: @developer two"
	want := "[channel 1, message 3, thread 1]\nNew in the thread:\nuser: root\nlead: @developer one\n\n" +
		"lead wrote: @developer two"
	if got := e.wake("developer", routed, &root); got != want {
		t.Errorf("wake =\n%q\nwant\n%q", got, want)
	}
}

func TestWakeAs_ReadsWhenTheTurnStarts(t *testing.T) {
	e := newWakeEnv(t)
	root := e.say("user", "root", nil)
	e.say("lead", "@developer start", &root)
	routed := "[channel 1, message 2, thread 1]\nlead wrote: @developer start"
	e.say("user", "one more thing", &root)
	want := "[channel 1, message 3, thread 1]\nNew in the thread:\nuser: root\nlead: @developer start\n\n" +
		"one more thing"
	if got := e.wake("developer", routed, &root); got != want {
		t.Errorf("wake =\n%q\nwant\n%q: the message stored after it was routed is in it", got, want)
	}
	if got := e.wake("developer", routed, &root); got != "" {
		t.Errorf("wake of what was sent = %q, want no turn", got)
	}
}

func TestWakeAs_NewSessionGetsTheThreadOnce(t *testing.T) {
	e := newWakeEnv(t)
	root := e.say("user", "root", nil)
	e.say("lead", "@developer start", &root)
	routed := "[channel 1, message 2, thread 1]\nlead wrote: @developer start"
	e.wake("developer", routed, &root)

	e.fail = "No conversation found with session ID: s1"
	e.say("lead", "@developer again", &root)
	if err := e.s.WakeAs(context.Background(), "developer", "[channel 1, message 4, thread 1]\n"+
		"lead wrote: @developer again", &root, discard); err == nil {
		t.Fatal("wake in the lost session succeeded, want an error")
	}

	e.fail = ""
	retry := "[channel 1, message 4, thread 1]\nlead wrote: @developer again"
	want := "[channel 1, message 4, thread 1]\nNew in the thread:\nuser: root\nlead: @developer start\n\n" +
		"lead wrote: @developer again"
	if got := e.wake("developer", retry, &root); got != want {
		t.Errorf("wake in the new session =\n%q\nwant\n%q", got, want)
	}
	if got := e.agents[len(e.agents)-1].session; got != "s3" {
		t.Fatalf("session = %q, want the new s3", got)
	}

	e.say("lead", "@developer last", &root)
	last := "[channel 1, message 6, thread 1]\nlead wrote: @developer last"
	if got := e.wake("developer", last, &root); got != last {
		t.Errorf("next wake = %q, want %q", got, last)
	}
}

func TestWakeAs_TopLevelHoldsTheChannelsTopLevel(t *testing.T) {
	e := newWakeEnv(t)
	root := e.say("user", "root", nil)
	e.say("lead", "@developer plan", &root)
	e.say("reef", "Rolled back.", nil)
	e.say("lead", "@developer top", nil)
	routed := "[channel 1, message 4]\nlead wrote: @developer top"
	want := "[channel 1, message 4]\nNew in the channel:\nuser: root\n\nlead wrote: @developer top"
	if got := e.wake("developer", routed, nil); got != want {
		t.Errorf("top-level wake =\n%q\nwant\n%q: no reef note, no thread reply", got, want)
	}

	thread := "[channel 1, message 2, thread 1]\nlead wrote: @developer plan"
	want = "[channel 1, message 2, thread 1]\nlead wrote: @developer plan"
	if got := e.wake("developer", thread, &root); got != want {
		t.Errorf("thread wake =\n%q\nwant\n%q: the root was sent at the top level", got, want)
	}
}
