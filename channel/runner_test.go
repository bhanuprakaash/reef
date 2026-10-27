package channel

import (
	"context"
	"errors"
	"math"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/store"
)

// fakeAgent answers each prompt with text, usage and a turn end. Like codex, it
// reports its session once.
type fakeAgent struct {
	// The fake emits a whole turn synchronously from Send, before Runner reads Events. The
	// buffer holds that fixed test script; production agents emit asynchronously.
	events  chan agent.Event
	prompts []string
}

type runFunc func(ctx context.Context, text string, parent *int64, onEvent func(agent.Event)) error

func sendAs(
	ctx context.Context, st *store.Store, channelID int64, run runFunc, text string,
	onEvent func(agent.Event),
) error {
	_, err := st.CreateMessage(ctx, store.Message{ChannelID: channelID, Author: _userAuthor, Body: text})
	if err != nil {
		return err
	}
	return run(ctx, text, nil, onEvent)
}

func (f *fakeAgent) Start(context.Context, agent.Config) error          { return nil }
func (f *fakeAgent) Resume(context.Context, agent.Config, string) error { return nil }
func (f *fakeAgent) Events() <-chan agent.Event                         { return f.events }
func (f *fakeAgent) Interrupt() error                                   { return nil }

func (f *fakeAgent) Send(_ context.Context, prompt string) error {
	if len(f.prompts) == 0 {
		f.events <- agent.Event{Kind: agent.KindSession, SessionID: "s1", Model: "m1"}
	}
	f.prompts = append(f.prompts, prompt)
	f.events <- agent.Event{Kind: agent.KindText, Text: "re"}
	f.events <- agent.Event{Kind: agent.KindText, Text: "ply " + prompt[len(prompt)-1:]}
	f.events <- agent.Event{Kind: agent.KindUsage, Usage: &agent.Usage{InputTokens: 3, OutputTokens: 5}}
	f.events <- agent.Event{Kind: agent.KindTurnEnd}
	return nil
}

func TestRunner_Send(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch, err := st.CreateChannel(ctx, store.Channel{
		ProjectID: f.ProjectID, FeatureID: &f.ID, Name: "chat",
	})
	if err != nil {
		t.Fatal(err)
	}
	fa := &fakeAgent{events: make(chan agent.Event, 8)}
	r := NewRunner(st, fa, ch.ID, "architect", "claude", "CONTEXT")
	r.Memory = testMemory(t, st)

	var streamed []string
	for _, text := range []string{"one 1", "two 2"} {
		err := sendAs(ctx, st, ch.ID, r.Run, text, func(ev agent.Event) {
			if ev.Kind == agent.KindText {
				streamed = append(streamed, ev.Text)
			}
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	if want := []string{"CONTEXT\n\none 1", "two 2"}; !reflect.DeepEqual(fa.prompts, want) {
		t.Errorf("prompts = %q, want %q", fa.prompts, want)
	}
	if want := []string{"re", "ply 1", "re", "ply 2"}; !reflect.DeepEqual(streamed, want) {
		t.Errorf("streamed = %q, want %q", streamed, want)
	}
	msgs, err := st.Messages(ctx, ch.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, m := range msgs {
		got = append(got, m.Author+": "+m.Body)
	}
	want := []string{"user: one 1", "architect: reply 1", "user: two 2", "architect: reply 2"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("messages = %q, want %q", got, want)
	}
	if strings.Contains(got[0], "CONTEXT") {
		t.Errorf("stored user message holds the context: %q", got[0])
	}
	turns, err := st.Turns(ctx, store.TurnFilter{ChannelID: &ch.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 2 {
		t.Fatalf("turns = %d, want 2", len(turns))
	}
	for i, tr := range turns {
		if want := []string{"one 1", "two 2"}[i]; tr.WakeText != want {
			t.Errorf("turn %d wake = %q, want %q: the context is not part of it", tr.ID, tr.WakeText, want)
		}
		if tr.Status != "done" || tr.SessionID != "s1" || tr.Role != "architect" || tr.Model != "m1" ||
			tr.Usage != (agent.Usage{InputTokens: 3, OutputTokens: 5}) {
			t.Errorf("turn = %+v, want done s1 architect m1 with usage 3/5", tr)
		}
	}
}

// scriptAgent answers each prompt with its script of events.
type scriptAgent struct {
	events      chan agent.Event
	script      []agent.Event
	closeAfter  bool
	onInterrupt agent.Event // left in events by Interrupt, like codex's usage of a stopped turn
	interrupted int
}

func (s *scriptAgent) Start(context.Context, agent.Config) error          { return nil }
func (s *scriptAgent) Resume(context.Context, agent.Config, string) error { return nil }
func (s *scriptAgent) Events() <-chan agent.Event                         { return s.events }

func (s *scriptAgent) Interrupt() error {
	s.interrupted++
	if s.onInterrupt.Kind != "" {
		s.events <- s.onInterrupt
	}
	return nil
}

func (s *scriptAgent) Send(context.Context, string) error {
	for _, ev := range s.script {
		s.events <- ev
	}
	if s.closeAfter {
		close(s.events)
	}
	return nil
}

func TestRunner_ReplyStaysInThread(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch, err := st.CreateChannel(ctx, store.Channel{
		ProjectID: f.ProjectID, FeatureID: &f.ID, Name: "chat",
	})
	if err != nil {
		t.Fatal(err)
	}
	root, err := st.CreateMessage(ctx, store.Message{ChannelID: ch.ID, Author: "user", Body: "root"})
	if err != nil {
		t.Fatal(err)
	}
	fa := &fakeAgent{events: make(chan agent.Event, 8)}
	r := NewRunner(st, fa, ch.ID, "architect", "claude", "")
	if err := r.Run(ctx, "one 1", &root.ID, func(agent.Event) {}); err != nil {
		t.Fatal(err)
	}
	msgs, err := st.Thread(ctx, ch.ID, root.ID)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, m := range msgs {
		got = append(got, m.Author+": "+m.Body)
	}
	if want := []string{"user: root", "architect: reply 1"}; !reflect.DeepEqual(got, want) {
		t.Errorf("thread = %q, want %q", got, want)
	}
	turn, err := st.Turn(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if turn.WakeText != "one 1" || turn.WakeParent == nil || *turn.WakeParent != root.ID {
		t.Errorf("turn wake = %q in %v, want %q in %d", turn.WakeText, turn.WakeParent, "one 1", root.ID)
	}
}

func TestRunner_BackgroundAnswerIsRoleMessage(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch, err := st.CreateChannel(ctx, store.Channel{
		ProjectID: f.ProjectID, FeatureID: &f.ID, Name: "chat",
	})
	if err != nil {
		t.Fatal(err)
	}
	sa := &scriptAgent{events: make(chan agent.Event, 8), script: []agent.Event{
		{Kind: agent.KindSession, SessionID: "s1"},
		{Kind: agent.KindBackground, Text: "The background command printed bgdone."},
		{Kind: agent.KindText, Text: "42"},
		{Kind: agent.KindTurnEnd},
	}}
	r := NewRunner(st, sa, ch.ID, "architect", "claude", "")
	if err := sendAs(ctx, st, ch.ID, r.Run, "What is 7 times 6?", func(agent.Event) {}); err != nil {
		t.Fatal(err)
	}
	msgs, err := st.Messages(ctx, ch.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, m := range msgs {
		got = append(got, m.Author+": "+m.Body)
	}
	want := []string{
		"user: What is 7 times 6?", "architect: The background command printed bgdone.", "architect: 42",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("messages = %q, want %q", got, want)
	}
}

func TestRunner_PostedTurnKeepsReplyOnTurn(t *testing.T) {
	script := func(tool string, isError bool) []agent.Event {
		return []agent.Event{
			{Kind: agent.KindSession, SessionID: "s1"},
			{Kind: agent.KindToolCall, ToolID: "t1", ToolName: tool},
			{Kind: agent.KindToolResult, ToolID: "t1", IsError: isError},
			{Kind: agent.KindText, Text: "plain reply"},
			{Kind: agent.KindTurnEnd},
		}
	}
	const post = "mcp__reef__post_to_channel"
	for _, tc := range []struct {
		name     string
		script   []agent.Event
		messages []string
		kept     string
	}{
		{"posted", script(post, false), []string{"architect: posted"}, "plain reply"},
		{"post failed", script(post, true), []string{"architect: plain reply"}, ""},
		{"other tool", script("mcp__reef__remember", false), []string{"architect: plain reply"}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			st, f := openStore(t)
			ch, err := st.CreateChannel(ctx, store.Channel{
				ProjectID: f.ProjectID, FeatureID: &f.ID, Name: "chat",
			})
			if err != nil {
				t.Fatal(err)
			}
			if tc.kept != "" {
				_, err := st.CreateMessage(ctx, store.Message{
					ChannelID: ch.ID, Author: "architect", Body: "posted",
				})
				if err != nil {
					t.Fatal(err)
				}
			}
			sa := &scriptAgent{events: make(chan agent.Event, 8), script: tc.script}
			r := NewRunner(st, sa, ch.ID, "architect", "claude", "")
			if err := r.Run(ctx, "go", nil, func(agent.Event) {}); err != nil {
				t.Fatal(err)
			}
			msgs, err := st.Messages(ctx, ch.ID, 0)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, m := range msgs {
				got = append(got, m.Author+": "+m.Body)
			}
			if !reflect.DeepEqual(got, tc.messages) {
				t.Errorf("messages = %q, want %q", got, tc.messages)
			}
			turn, err := st.Turn(ctx, 1)
			if err != nil || turn.Status != "done" || turn.Reply != tc.kept {
				t.Errorf("turn = %+v, %v, want done with reply %q", turn, err, tc.kept)
			}
		})
	}
}

func TestRunner_PostedDirectTurnStillAnswersSender(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch, err := st.CreateChannel(ctx, store.Channel{
		ProjectID: f.ProjectID, FeatureID: &f.ID, Name: "chat",
	})
	if err != nil {
		t.Fatal(err)
	}
	req, err := st.CreateDirectMessage(ctx, store.DirectMessage{
		ChannelID: ch.ID, From: "lead", To: "architect", Body: "plan 1",
	})
	if err != nil {
		t.Fatal(err)
	}
	sa := &scriptAgent{events: make(chan agent.Event, 8), script: []agent.Event{
		{Kind: agent.KindToolCall, ToolID: "t1", ToolName: "mcp__reef__post_to_channel"},
		{Kind: agent.KindToolResult, ToolID: "t1"},
		{Kind: agent.KindText, Text: "private answer"},
		{Kind: agent.KindTurnEnd},
	}}
	r := NewRunner(st, sa, ch.ID, "architect", "claude", "")
	if err := r.RunDirect(ctx, "request 1", req, func(agent.Event) {}); err != nil {
		t.Fatal(err)
	}
	direct, err := st.DirectMessages(ctx, ch.ID, 0)
	if err != nil || len(direct) != 2 || direct[1].Body != "private answer" {
		t.Errorf("direct = %+v, %v, want the private answer as the response", direct, err)
	}
}

func TestRunner_DirectReplyStopsAfterOneResponse(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch, err := st.CreateChannel(ctx, store.Channel{
		ProjectID: f.ProjectID, FeatureID: &f.ID, Name: "chat",
	})
	if err != nil {
		t.Fatal(err)
	}
	req, err := st.CreateDirectMessage(ctx, store.DirectMessage{
		ChannelID: ch.ID, From: "lead", To: "architect", Body: "plan 1",
	})
	if err != nil {
		t.Fatal(err)
	}
	architect := NewRunner(st, &fakeAgent{events: make(chan agent.Event, 8)},
		ch.ID, "architect", "claude", "")
	if err := architect.RunDirect(ctx, "request 1", req, func(agent.Event) {}); err != nil {
		t.Fatal(err)
	}
	direct, err := st.DirectMessages(ctx, ch.ID, 0)
	if err != nil || len(direct) != 2 || direct[1].Body != "reply 1" {
		t.Fatalf("direct after request = %+v, %v, want one response", direct, err)
	}
	lead := NewRunner(st, &fakeAgent{events: make(chan agent.Event, 8)},
		ch.ID, "lead", "claude", "")
	if err := lead.RunDirect(ctx, "response 2", direct[1], func(agent.Event) {}); err != nil {
		t.Fatal(err)
	}
	direct, err = st.DirectMessages(ctx, ch.ID, 0)
	if err != nil || len(direct) != 2 {
		t.Errorf("direct after handling response = %+v, %v, want no automatic third message", direct, err)
	}
	public, err := st.Messages(ctx, ch.ID, 0)
	if err != nil || len(public) != 0 {
		t.Errorf("public messages = %+v, %v, want none", public, err)
	}
}

func TestRunner_FailedDirectTurnKeepsPartialAttempt(t *testing.T) {
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
	// Send emits both scripted events synchronously before Runner reads them.
	ag := &scriptAgent{events: make(chan agent.Event, 8), script: []agent.Event{
		{Kind: agent.KindText, Text: "partial"},
		{Kind: agent.KindError, Text: "boom"},
	}}
	r := NewRunner(st, ag, ch.ID, "architect", "claude", "")
	if err := r.RunDirect(ctx, "direct wake", req, func(agent.Event) {}); err == nil {
		t.Fatal("RunDirect succeeded, want agent error")
	}
	direct, err := st.DirectMessages(ctx, ch.ID, 0)
	if err != nil || len(direct) != 2 || direct[1].Body != "partial" ||
		direct[1].Status != store.DirectFailed {
		t.Errorf("direct messages = %+v, %v, want inspectable failed partial attempt", direct, err)
	}
	public, err := st.Messages(ctx, ch.ID, 0)
	if err != nil || len(public) != 0 {
		t.Errorf("public messages = %+v, %v, want no partial leak", public, err)
	}
}

func TestRunner_StoppedTurnKeepsReplyInThread(t *testing.T) {
	bg := context.Background()
	st, f := openStore(t)
	ch, err := st.CreateChannel(bg, store.Channel{
		ProjectID: f.ProjectID, FeatureID: &f.ID, Name: "chat",
	})
	if err != nil {
		t.Fatal(err)
	}
	root, err := st.CreateMessage(bg, store.Message{ChannelID: ch.ID, Author: "user", Body: "root"})
	if err != nil {
		t.Fatal(err)
	}
	sa := &scriptAgent{
		events: make(chan agent.Event, 8), script: []agent.Event{{Kind: agent.KindText, Text: "par"}},
	}
	r := NewRunner(st, sa, ch.ID, "architect", "claude", "")
	ctx, cancel := context.WithCancel(bg)
	defer cancel()

	err = r.Run(ctx, "one", &root.ID, func(agent.Event) { cancel() })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v, want context canceled", err)
	}
	msgs, err := st.Thread(bg, ch.ID, root.ID)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, m := range msgs {
		got = append(got, m.Author+": "+m.Body)
	}
	if want := []string{"user: root", "architect: par"}; !reflect.DeepEqual(got, want) {
		t.Errorf("thread = %q, want %q", got, want)
	}
}

func TestRunner_SendFailedTurn(t *testing.T) {
	tests := []struct {
		name        string
		script      []agent.Event
		closeAfter  bool
		cancelOn    agent.Kind
		wantErr     string
		wantTurnErr string
		wantSession string
		wantReply   string
		wantStopped int
	}{
		{
			name:        "cancelled mid-turn",
			script:      []agent.Event{{Kind: agent.KindText, Text: "par"}},
			cancelOn:    agent.KindText,
			wantErr:     "context canceled",
			wantTurnErr: "interrupted",
			wantReply:   "par",
			wantStopped: 1,
		},
		{
			name:        "cancelled while the session is stored",
			script:      []agent.Event{{Kind: agent.KindSession, SessionID: "s1"}},
			cancelOn:    agent.KindSession,
			wantErr:     "context canceled",
			wantTurnErr: "interrupted",
			wantSession: "s1",
			wantStopped: 1,
		},
		{
			name: "agent error",
			script: []agent.Event{
				{Kind: agent.KindText, Text: "par"}, {Kind: agent.KindError, Text: "boom"},
			},
			wantErr:     "agent: boom",
			wantTurnErr: "boom",
			wantReply:   "par",
		},
		{
			name:        "events closed",
			script:      []agent.Event{{Kind: agent.KindText, Text: "par"}},
			closeAfter:  true,
			wantErr:     "agent exited",
			wantTurnErr: "agent exited",
			wantReply:   "par",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bg := context.Background()
			st, f := openStore(t)
			ch, err := st.CreateChannel(bg, store.Channel{
				ProjectID: f.ProjectID, FeatureID: &f.ID, Name: "chat",
			})
			if err != nil {
				t.Fatal(err)
			}
			sa := &scriptAgent{
				events: make(chan agent.Event, 8), script: tt.script, closeAfter: tt.closeAfter,
			}
			r := NewRunner(st, sa, ch.ID, "architect", "claude", "")
			ctx, cancel := context.WithCancel(bg)
			defer cancel()

			err = sendAs(ctx, st, ch.ID, r.Run, "hi", func(ev agent.Event) {
				if ev.Kind == tt.cancelOn {
					cancel()
				}
			})
			if err == nil || err.Error() != tt.wantErr {
				t.Errorf("Send error = %v, want %q", err, tt.wantErr)
			}
			if sa.interrupted != tt.wantStopped {
				t.Errorf("agent interrupted %d times, want %d", sa.interrupted, tt.wantStopped)
			}
			turns, err := st.Turns(bg, store.TurnFilter{ChannelID: &ch.ID})
			if err != nil {
				t.Fatal(err)
			}
			if len(turns) != 1 || turns[0].Status != "failed" || turns[0].Error != tt.wantTurnErr ||
				turns[0].SessionID != tt.wantSession {
				t.Errorf("turns = %+v, want one failed turn with error %q and session %q",
					turns, tt.wantTurnErr, tt.wantSession)
			}
			msgs, err := st.Messages(bg, ch.ID, 0)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, m := range msgs {
				got = append(got, m.Author+": "+m.Body)
			}
			want := []string{"user: hi"}
			if tt.wantReply != "" {
				want = append(want, "architect: "+tt.wantReply)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("messages = %q, want %q", got, want)
			}
		})
	}
}

// cancelOnDone reports ctx as cancelled from its nth Done call on. The first three
// calls happen while inserting the user message; the fourth is the runner waiting for
// the agent's events.
type cancelOnDone struct {
	context.Context
	calls atomic.Int32
	once  sync.Once
	done  chan struct{}
}

func (c *cancelOnDone) Done() <-chan struct{} {
	if c.calls.Add(1) == 4 {
		c.cancel()
	}
	return c.done
}

func (c *cancelOnDone) cancel() { c.once.Do(func() { close(c.done) }) }

func (c *cancelOnDone) Err() error {
	select {
	case <-c.done:
		return context.Canceled
	default:
		return nil
	}
}

func TestRunner_SendCancelledAfterMessageStoresTurn(t *testing.T) {
	bg := context.Background()
	st, f := openStore(t)
	ch, err := st.CreateChannel(bg, store.Channel{
		ProjectID: f.ProjectID, FeatureID: &f.ID, Name: "chat",
	})
	if err != nil {
		t.Fatal(err)
	}
	sa := &scriptAgent{events: make(chan agent.Event, 8)}
	r := NewRunner(st, sa, ch.ID, "architect", "claude", "")
	ctx := &cancelOnDone{Context: bg, done: make(chan struct{})}
	defer time.AfterFunc(5*time.Second, ctx.cancel).Stop()

	if err := sendAs(ctx, st, ch.ID, r.Run, "hi", func(agent.Event) {}); err == nil || err.Error() != "context canceled" {
		t.Fatalf("Send error = %v, want context canceled", err)
	}
	msgs, err := st.Messages(bg, ch.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	turns, err := st.Turns(bg, store.TurnFilter{ChannelID: &ch.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 || msgs[0].Body != "hi" || len(turns) != 1 ||
		turns[0].Status != "failed" || turns[0].Error != "interrupted" {
		t.Errorf("messages = %+v, turns = %+v, want the user message and one failed interrupted turn",
			msgs, turns)
	}
}

func TestRunner_RunCancelledBeforeSend(t *testing.T) {
	bg := context.Background()
	st, f := openStore(t)
	ch, err := st.CreateChannel(bg, store.Channel{
		ProjectID: f.ProjectID, FeatureID: &f.ID, Name: "chat",
	})
	if err != nil {
		t.Fatal(err)
	}
	sa := &scriptAgent{
		events: make(chan agent.Event, 8), script: []agent.Event{{Kind: agent.KindTurnEnd}},
	}
	r := NewRunner(st, sa, ch.ID, "architect", "claude", "")
	ctx, cancel := context.WithCancel(bg)
	cancel()

	if err := r.Run(ctx, "hi", nil, func(agent.Event) {}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v, want context canceled", err)
	}
	if len(sa.events) != 0 || sa.interrupted != 0 {
		t.Errorf("events left = %d, interrupts = %d, want the agent untouched",
			len(sa.events), sa.interrupted)
	}
	turns, err := st.Turns(bg, store.TurnFilter{ChannelID: &ch.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 1 || turns[0].Status != "failed" || turns[0].Error != "interrupted" {
		t.Errorf("turns = %+v, want one failed interrupted turn", turns)
	}
}

func TestRunner_SendCancelledKeepsStoppedUsage(t *testing.T) {
	bg := context.Background()
	st, f := openStore(t)
	ch, err := st.CreateChannel(bg, store.Channel{
		ProjectID: f.ProjectID, FeatureID: &f.ID, Name: "chat",
	})
	if err != nil {
		t.Fatal(err)
	}
	sa := &scriptAgent{
		events:      make(chan agent.Event, 8),
		script:      []agent.Event{{Kind: agent.KindText, Text: "par"}},
		onInterrupt: agent.Event{Kind: agent.KindUsage, Usage: &agent.Usage{InputTokens: 9, OutputTokens: 2}},
	}
	r := NewRunner(st, sa, ch.ID, "architect", "codex", "")
	ctx, cancel := context.WithCancel(bg)
	defer cancel()

	_ = sendAs(ctx, st, ch.ID, r.Run, "hi", func(agent.Event) { cancel() })
	turns, err := st.Turns(bg, store.TurnFilter{ChannelID: &ch.ID})
	if err != nil {
		t.Fatal(err)
	}
	want := agent.Usage{InputTokens: 9, OutputTokens: 2}
	if len(turns) != 1 || turns[0].Status != "failed" || turns[0].Usage != want {
		t.Errorf("turns = %+v, want one failed turn with usage %+v", turns, want)
	}
}

func TestRunner_RecordsModelCostPromptAndToolCalls(t *testing.T) {
	bg := context.Background()
	st, f := openStore(t)
	ch, err := st.CreateChannel(bg, store.Channel{
		ProjectID: f.ProjectID, FeatureID: &f.ID, Name: "chat",
	})
	if err != nil {
		t.Fatal(err)
	}
	cost := 0.0123
	sa := &scriptAgent{
		events: make(chan agent.Event, 8),
		script: []agent.Event{
			{Kind: agent.KindSession, SessionID: "s1", Model: "claude-opus-5-5"},
			{Kind: agent.KindToolCall, ToolID: "t1", ToolName: "Bash", Input: []byte(`{"command":"ls"}`)},
			{Kind: agent.KindToolResult, ToolID: "t1", Output: "a.txt\n"},
			{Kind: agent.KindTurnEnd, CostUSD: &cost},
		},
	}
	r := NewRunner(st, sa, ch.ID, "architect", "claude", "")
	if err := sendAs(bg, st, ch.ID, r.Run, "do it", func(agent.Event) {}); err != nil {
		t.Fatal(err)
	}

	turn, err := st.Turn(bg, 1)
	if err != nil {
		t.Fatal(err)
	}
	if turn.Model != "claude-opus-5-5" || turn.CostUSD == nil || *turn.CostUSD != cost ||
		!strings.Contains(turn.Prompt, "do it") {
		t.Errorf("turn = %+v, want model claude-opus-5-5, cost %v, prompt containing %q",
			turn, cost, "do it")
	}
	calls, err := st.ToolCalls(bg, turn.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := []store.ToolCall{{ID: 1, TurnID: 1, Name: "Bash", Input: `{"command":"ls"}`, Result: "a.txt\n"}}
	if !reflect.DeepEqual(calls, want) {
		t.Errorf("tool calls = %+v, want %+v", calls, want)
	}
}

func TestRunner_RecordsSubagentToolCallParent(t *testing.T) {
	bg := context.Background()
	st, f := openStore(t)
	ch, err := st.CreateChannel(bg, store.Channel{
		ProjectID: f.ProjectID, FeatureID: &f.ID, Name: "chat",
	})
	if err != nil {
		t.Fatal(err)
	}
	sa := &scriptAgent{
		events: make(chan agent.Event, 8),
		script: []agent.Event{
			{Kind: agent.KindToolCall, ToolID: "a1", ToolName: "Agent", Input: []byte(`{}`)},
			{Kind: agent.KindToolCall, ToolID: "b1", ParentToolID: "a1", ToolName: "Read", Input: []byte(`{}`)},
			{Kind: agent.KindToolCall, ToolID: "c1", ParentToolID: "gone", ToolName: "Bash", Input: []byte(`{}`)},
			{Kind: agent.KindTurnEnd},
		},
	}
	r := NewRunner(st, sa, ch.ID, "developer", "claude", "")
	if err := sendAs(bg, st, ch.ID, r.Run, "do it", func(agent.Event) {}); err != nil {
		t.Fatal(err)
	}

	calls, err := st.ToolCalls(bg, 1)
	if err != nil {
		t.Fatal(err)
	}
	var parents []int64
	for _, c := range calls {
		parents = append(parents, c.ParentID)
	}
	if want := []int64{0, 1, 0}; !reflect.DeepEqual(parents, want) {
		t.Errorf("parent ids = %v, want %v", parents, want)
	}
}

func TestRunner_ResumedSessionStoresTurnOwnCost(t *testing.T) {
	bg := context.Background()
	st, f := openStore(t)
	ch, err := st.CreateChannel(bg, store.Channel{
		ProjectID: f.ProjectID, FeatureID: &f.ID, Name: "chat",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, total := range []float64{1.00, 1.10} {
		sa := &scriptAgent{
			events: make(chan agent.Event, 8),
			script: []agent.Event{
				{Kind: agent.KindSession, SessionID: "s1"},
				{Kind: agent.KindTurnEnd, CostUSD: &total},
			},
		}
		r := NewRunner(st, sa, ch.ID, "architect", "claude", "")
		if err := sendAs(bg, st, ch.ID, r.Run, "do it", func(agent.Event) {}); err != nil {
			t.Fatal(err)
		}
	}

	turns, err := st.Turns(bg, store.TurnFilter{})
	if err != nil {
		t.Fatal(err)
	}
	var got []float64
	for _, tn := range turns {
		got = append(got, *tn.CostUSD)
	}
	if want := []float64{1.00, 0.10}; len(got) != 2 || got[0] != want[0] || math.Abs(got[1]-want[1]) > 1e-9 {
		t.Errorf("turn costs = %v, want %v", got, want)
	}
}

// Each turn of a session reports the whole session's usage. A resumed turn that ran a subagent
// stores the subagent's tokens too, and a stopped turn between two turns is not counted twice.
func TestRunner_ResumedSessionStoresTurnOwnUsage(t *testing.T) {
	bg := context.Background()
	st, f := openStore(t)
	ch, err := st.CreateChannel(bg, store.Channel{
		ProjectID: f.ProjectID, FeatureID: &f.ID, Name: "chat",
	})
	if err != nil {
		t.Fatal(err)
	}
	first := agent.Usage{InputTokens: 1176, OutputTokens: 23, CacheReadTokens: 38208, CacheWriteTokens: 7551}
	second := agent.Usage{InputTokens: 8, OutputTokens: 511, CacheReadTokens: 53174, CacheWriteTokens: 24389}
	stopped := agent.Usage{InputTokens: 2, OutputTokens: 40, CacheReadTokens: 100}
	streamed := agent.Usage{InputTokens: 2, OutputTokens: 72, CacheReadTokens: 18205, CacheWriteTokens: 102}
	session := func(u agent.Usage) *agent.Usage { return &u }
	sum := func(a, b agent.Usage) agent.Usage { a.Add(b); return a }

	scripts := [][]agent.Event{
		{{Kind: agent.KindTurnEnd, SessionUsage: session(first)}},
		{{Kind: agent.KindUsage, Usage: &streamed}, {Kind: agent.KindTurnEnd, SessionUsage: session(sum(first, second))}},
		{{Kind: agent.KindUsage, Usage: &stopped}},
		{{Kind: agent.KindTurnEnd, SessionUsage: session(sum(sum(first, second), sum(stopped, stopped)))}},
	}
	for i, script := range scripts {
		sa := &scriptAgent{
			events: make(chan agent.Event, 8),
			script: append([]agent.Event{{Kind: agent.KindSession, SessionID: "s1"}}, script...),
		}
		r := NewRunner(st, sa, ch.ID, "developer", "claude", "")
		if i == 2 {
			sa.closeAfter = true
		}
		_ = sendAs(bg, st, ch.ID, r.Run, "do it", func(agent.Event) {})
	}

	turns, err := st.Turns(bg, store.TurnFilter{})
	if err != nil {
		t.Fatal(err)
	}
	var got []agent.Usage
	for _, tn := range turns {
		got = append(got, tn.Usage)
	}
	want := []agent.Usage{first, second, stopped, stopped}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("turn usage = %+v, want %+v", got, want)
	}
}

// A session whose turns were stored with running totals as their cost still stores the cost of
// its next turn.
func TestRunner_LegacySessionStoresTurnOwnCost(t *testing.T) {
	bg := context.Background()
	st, f := openStore(t)
	ch, err := st.CreateChannel(bg, store.Channel{
		ProjectID: f.ProjectID, FeatureID: &f.ID, Name: "chat",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, total := range []float64{1.00, 1.10, 1.30} {
		old, err := st.CreateTurn(bg, store.Turn{
			ChannelID: ch.ID, Role: "architect", Backend: "claude", SessionID: "s1",
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := st.EndTurn(bg, store.Turn{ID: old.ID, Status: "done", CostUSD: &total}); err != nil {
			t.Fatal(err)
		}
	}
	total := 1.45
	sa := &scriptAgent{
		events: make(chan agent.Event, 8),
		script: []agent.Event{
			{Kind: agent.KindSession, SessionID: "s1"},
			{Kind: agent.KindTurnEnd, CostUSD: &total},
		},
	}
	r := NewRunner(st, sa, ch.ID, "architect", "claude", "")
	if err := sendAs(bg, st, ch.ID, r.Run, "do it", func(agent.Event) {}); err != nil {
		t.Fatal(err)
	}
	turns := mustTurns(t, st)
	got := *turns[len(turns)-1].CostUSD
	if math.Abs(got-0.15) > 1e-9 {
		t.Errorf("cost of the turn after the old ones = %v, want 0.15", got)
	}
}

func mustTurns(t *testing.T, st *store.Store) []store.Turn {
	t.Helper()
	turns, err := st.Turns(context.Background(), store.TurnFilter{})
	if err != nil {
		t.Fatal(err)
	}
	return turns
}

func TestRunner_FailedTurnStoresCost(t *testing.T) {
	bg := context.Background()
	st, f := openStore(t)
	ch, err := st.CreateChannel(bg, store.Channel{
		ProjectID: f.ProjectID, FeatureID: &f.ID, Name: "chat",
	})
	if err != nil {
		t.Fatal(err)
	}
	cost := 0.0073
	sa := &scriptAgent{
		events: make(chan agent.Event, 8),
		script: []agent.Event{
			{Kind: agent.KindSession, SessionID: "s1", Model: "claude-opus-5-5"},
			{Kind: agent.KindError, Text: "boom", CostUSD: &cost},
		},
	}
	r := NewRunner(st, sa, ch.ID, "architect", "claude", "")
	if err := sendAs(bg, st, ch.ID, r.Run, "do it", func(agent.Event) {}); err == nil {
		t.Fatal("want error")
	}

	turn, err := st.Turn(bg, 1)
	if err != nil {
		t.Fatal(err)
	}
	if turn.Status != "failed" || turn.CostUSD == nil || *turn.CostUSD != cost {
		t.Errorf("turn = %+v, want failed with cost %v", turn, cost)
	}
}

func TestRunner_CodexTurnHasModelNoCost(t *testing.T) {
	bg := context.Background()
	st, f := openStore(t)
	ch, err := st.CreateChannel(bg, store.Channel{
		ProjectID: f.ProjectID, FeatureID: &f.ID, Name: "chat",
	})
	if err != nil {
		t.Fatal(err)
	}
	sa := &scriptAgent{
		events: make(chan agent.Event, 8),
		script: []agent.Event{
			{Kind: agent.KindSession, SessionID: "t1", Model: "gpt-6-codex"},
			{Kind: agent.KindUsage, Usage: &agent.Usage{InputTokens: 10, OutputTokens: 3}},
			{Kind: agent.KindTurnEnd},
		},
	}
	r := NewRunner(st, sa, ch.ID, "architect", "codex", "")
	if err := sendAs(bg, st, ch.ID, r.Run, "go", func(agent.Event) {}); err != nil {
		t.Fatal(err)
	}

	turn, err := st.Turn(bg, 1)
	if err != nil {
		t.Fatal(err)
	}
	if turn.Model != "gpt-6-codex" || turn.CostUSD != nil ||
		turn.Usage != (agent.Usage{InputTokens: 10, OutputTokens: 3}) {
		t.Errorf("turn = %+v, want model gpt-6-codex, no cost, usage 10/3", turn)
	}
}

func TestRunner_SendRunsAnyNumberOfToolCalls(t *testing.T) {
	bg := context.Background()
	st, f := openStore(t)
	ch, err := st.CreateChannel(bg, store.Channel{
		ProjectID: f.ProjectID, FeatureID: &f.ID, Name: "chat",
	})
	if err != nil {
		t.Fatal(err)
	}
	var script []agent.Event
	for range 26 {
		script = append(script, agent.Event{Kind: agent.KindToolCall, ToolName: "Read"})
	}
	sa := &scriptAgent{
		events: make(chan agent.Event, 32),
		script: append(script, agent.Event{Kind: agent.KindText, Text: "done"}, agent.Event{Kind: agent.KindTurnEnd}),
	}
	r := NewRunner(st, sa, ch.ID, "architect", "claude", "")

	if err := sendAs(bg, st, ch.ID, r.Run, "hi", func(agent.Event) {}); err != nil {
		t.Fatalf("Send error = %v, want nil", err)
	}
	got, err := st.Channel(bg, ch.ID)
	if err != nil || got.Paused != "" {
		t.Errorf("channel = %+v, %v, want not paused", got, err)
	}
	turns, err := st.Turns(bg, store.TurnFilter{ChannelID: &ch.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 1 || turns[0].Status != "done" {
		t.Errorf("turns = %+v, want one done turn", turns)
	}
}
