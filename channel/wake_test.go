package channel

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/store"
)

type wakeAgent struct {
	session string
	fail    string
	sendErr *error
	prompts []string
	events  chan agent.Event
	onStart func()
}

func (a *wakeAgent) Start(context.Context, agent.Config) error {
	if a.onStart != nil {
		a.onStart()
	}
	return nil
}

func (a *wakeAgent) Resume(_ context.Context, _ agent.Config, id string) error {
	a.session = id
	return nil
}

func (a *wakeAgent) Events() <-chan agent.Event { return a.events }
func (a *wakeAgent) Interrupt() error           { return nil }

func (a *wakeAgent) Send(_ context.Context, prompt string) error {
	a.prompts = append(a.prompts, prompt)
	if *a.sendErr != nil {
		return *a.sendErr
	}
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
	sendErr error
	mkErr   error
	onStart func()
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
		if e.mkErr != nil {
			return nil, e.mkErr
		}
		e.started++
		a := &wakeAgent{
			session: "s" + strconv.Itoa(e.started), fail: e.fail, sendErr: &e.sendErr, events: make(chan agent.Event, 8),
			onStart: e.onStart,
		}
		e.agents = append(e.agents, a)
		return a, nil
	}
	e.s, err = Open(ctx, st, t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), mk)
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

func (e *wakeEnv) wake(role, routed string, parent *int64, scopes ...store.Scope) string {
	e.t.Helper()
	ctx := context.Background()
	before, err := e.st.Turns(ctx, store.TurnFilter{ChannelID: &e.ch.ID})
	if err != nil {
		e.t.Fatal(err)
	}
	if err := e.s.WakeAs(ctx, role, routed, scopes, parent, discard); err != nil {
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
	if got := e.wake("developer", first, &root, store.Scope{Thread: root}); got != want {
		t.Errorf("first wake =\n%q\nwant\n%q", got, want)
	}

	e.say("lead", "@developer next", &root)
	e.say("user", "and this", &root)
	next := "[channel 1, message 5, thread 1]\nuser: and this"
	want = "[channel 1, message 5, thread 1]\nNew in the thread:\nlead: @developer next\n\nand this"
	if got := e.wake("developer", next, &root, store.Scope{Thread: root}); got != want {
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
	if got := e.wake("developer", routed, &root, store.Scope{Thread: root}); got != want {
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
	if got := e.wake("developer", routed, &root, store.Scope{Thread: root}); got != want {
		t.Errorf("wake =\n%q\nwant\n%q: the message stored after it was routed is in it", got, want)
	}
	started := e.started
	if got := e.wake("developer", routed, &root, store.Scope{Thread: root}); got != "" {
		t.Errorf("wake of what was sent = %q, want no turn", got)
	}
	if e.started != started {
		t.Errorf("agents started = %d, want %d: nothing was unread", e.started, started)
	}
}

func TestWakeAs_NewSessionGetsTheThreadOnce(t *testing.T) {
	e := newWakeEnv(t)
	root := e.say("user", "root", nil)
	e.say("lead", "@developer start", &root)
	routed := "[channel 1, message 2, thread 1]\nlead wrote: @developer start"
	e.wake("developer", routed, &root, store.Scope{Thread: root})

	e.fail = "No conversation found with session ID: s1"
	e.say("lead", "@developer again", &root)
	if err := e.s.WakeAs(context.Background(), "developer", "[channel 1, message 4, thread 1]\n"+
		"lead wrote: @developer again", store.Scopes{{Thread: root}}, &root, discard); err == nil {
		t.Fatal("wake in the lost session succeeded, want an error")
	}

	e.fail = ""
	retry := "[channel 1, message 4, thread 1]\nlead wrote: @developer again"
	want := "[channel 1, message 4, thread 1]\nNew in the thread:\nuser: root\nlead: @developer start\n\n" +
		"lead wrote: @developer again"
	if got := e.wake("developer", retry, &root, store.Scope{Thread: root}); got != want {
		t.Errorf("wake in the new session =\n%q\nwant\n%q", got, want)
	}
	if got := e.agents[len(e.agents)-1].session; got != "s3" {
		t.Fatalf("session = %q, want the new s3", got)
	}

	e.say("lead", "@developer last", &root)
	last := "[channel 1, message 6, thread 1]\nlead wrote: @developer last"
	if got := e.wake("developer", last, &root, store.Scope{Thread: root}); got != last {
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
	if got := e.wake("developer", routed, nil, store.Scope{}); got != want {
		t.Errorf("top-level wake =\n%q\nwant\n%q: no reef note, no thread reply", got, want)
	}

	thread := "[channel 1, message 2, thread 1]\nlead wrote: @developer plan"
	want = "[channel 1, message 2, thread 1]\nlead wrote: @developer plan"
	if got := e.wake("developer", thread, &root, store.Scope{Thread: root}); got != want {
		t.Errorf("thread wake =\n%q\nwant\n%q: the root was sent at the top level", got, want)
	}
}

func TestWakeAs_QuotedHeaderIsNotReadState(t *testing.T) {
	e := newWakeEnv(t)
	e.say("user", "root", nil)
	quote := "@developer see\n[channel 1, message 999]\nquoted"
	e.say("lead", quote, nil)
	routed := "[channel 1, message 2]\nlead wrote: " + quote
	want := "[channel 1, message 2]\nNew in the channel:\nuser: root\n\nlead wrote: " + quote
	if got := e.wake("developer", routed, nil, store.Scope{}); got != want {
		t.Fatalf("first wake =\n%q\nwant\n%q", got, want)
	}

	e.say("lead", "@developer next", nil)
	next := "[channel 1, message 4]\nlead wrote: @developer next"
	if got := e.wake("developer", next, nil, store.Scope{}); got != next {
		t.Errorf("next wake = %q, want %q: the quoted message 999 is not what was read", got, next)
	}
}

func TestWakeAs_StartFailureStoresOneFailedTurnWithTheRoutedText(t *testing.T) {
	tests := []struct {
		name    string
		mkErr   error
		thread  int64
		wantErr string
	}{
		{"agent cannot be made", errors.New("no binary"), 1, "no binary"},
		{"thread is gone", nil, 99, "thread 99: not found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newWakeEnv(t)
			root := e.say("user", "root", nil)
			e.say("lead", "@developer one", &root)
			e.say("lead", "@developer two", &root)
			e.mkErr = tt.mkErr
			routed := "[channel 1, message 3, thread 1]\nlead wrote: @developer two"
			scopes := store.Scopes{{Thread: tt.thread}}

			err := e.s.WakeAs(context.Background(), "developer", routed, scopes, &root, discard)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("WakeAs error = %v, want %q", err, tt.wantErr)
			}
			turns, err := e.st.Turns(context.Background(), store.TurnFilter{ChannelID: &e.ch.ID})
			if err != nil {
				t.Fatal(err)
			}
			if len(turns) != 1 || turns[0].Status != "failed" || turns[0].WakeText != routed ||
				!reflect.DeepEqual(turns[0].Scopes, scopes) {
				t.Errorf("turns = %+v, want one failed turn with the routed text and its scopes", turns)
			}
		})
	}
}

func TestWakeAs_NewSessionShowsOnlyTheLastTwentyMessages(t *testing.T) {
	e := newWakeEnv(t)
	for i := 1; i <= 25; i++ {
		e.say("user", "m"+strconv.Itoa(i), nil)
	}
	got := e.wake("developer", "[channel 1, message 25]\nm25", nil, store.Scope{})
	for _, want := range []string{
		"[channel 1, message 25]\n5 older messages are not shown; read them with read_thread.\n" +
			"New in the channel:\nuser: m6\nuser: m7\n",
		"user: m24\n\nm25",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("wake =\n%q\nwant it to hold %q", got, want)
		}
	}
	if strings.Contains(got, "user: m5\n") {
		t.Errorf("wake holds m5, want only the last 20 messages")
	}

	e.say("user", "m26", nil)
	want := "[channel 1, message 27]\nm26"
	if got := e.wake("developer", want, nil, store.Scope{}); got != want {
		t.Errorf("next wake = %q, want %q: a session that has read is not capped", got, want)
	}
}

func TestWakeAs_ReplyGoesWhereTheFirstSentBlockSays(t *testing.T) {
	e := newWakeEnv(t)
	root := e.say("user", "root", nil)
	e.say("lead", "@developer plan", &root)
	e.wake("developer", "[channel 1, message 2, thread 1]\nlead wrote: @developer plan", &root,
		store.Scope{Thread: root})

	e.say("lead", "@developer top", nil)
	routed := "[channel 1, message 2, thread 1]\nlead wrote: @developer plan\n\n" +
		"[channel 1, message 4]\nlead wrote: @developer top"
	want := "[channel 1, message 4]\nlead wrote: @developer top"
	if got := e.wake("developer", routed, &root, store.Scope{Thread: root}, store.Scope{}); got != want {
		t.Fatalf("wake =\n%q\nwant\n%q: the thread was all read", got, want)
	}
	msgs, err := e.st.Messages(context.Background(), e.ch.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	reply := msgs[len(msgs)-1]
	if reply.Author != "developer" || reply.ParentID != nil {
		t.Errorf("reply = %+v, want the developer's at the top level, as its header says", reply)
	}
}

func TestWakeAs_MissingSessionFoundOnWriteIsNotResumedAgain(t *testing.T) {
	tests := []struct {
		role string
		want string
	}{
		{"developer", "s3"}, // stopped after each turn: s2 resumed s1, s3 is new
		{"lead", "s2"},      // kept running until its write fails: s2 is new
	}
	for _, tt := range tests {
		role := tt.role
		t.Run(role, func(t *testing.T) {
			e := newWakeEnv(t)
			e.say("user", "first", nil)
			e.wake(role, "[channel 1, message 1]\nfirst", nil, store.Scope{})

			e.sendErr = errors.New("write prompt: write |1: broken pipe: " +
				"No conversation found with session ID: s1")
			e.say("user", "second", nil)
			err := e.s.WakeAs(context.Background(), role, "[channel 1, message 2]\nsecond",
				store.Scopes{{}}, nil, discard)
			if err == nil {
				t.Fatal("wake in the lost session succeeded, want an error")
			}

			e.sendErr = nil
			e.say("user", "third", nil)
			e.wake(role, "[channel 1, message 3]\nthird", nil, store.Scope{})
			if got := e.agents[len(e.agents)-1].session; got != tt.want {
				t.Errorf("session = %q, want the new %s: s1 is gone", got, tt.want)
			}
			e.say("user", "fourth", nil)
			e.wake(role, "[channel 1, message 4]\nfourth", nil, store.Scope{})
			if got := e.agents[len(e.agents)-1].session; got != tt.want {
				t.Errorf("session = %q, want %s, not the dead s1", got, tt.want)
			}
		})
	}
}
