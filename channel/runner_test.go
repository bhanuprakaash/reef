package channel

import (
	"context"
	"errors"
	"fmt"
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
	events  chan agent.Event
	prompts []string
}

type sayRunner interface {
	Say(ctx context.Context, body string) error
	Run(ctx context.Context, text string, onEvent func(agent.Event)) error
}

func sendAs(ctx context.Context, r sayRunner, text string, onEvent func(agent.Event)) error {
	if err := r.Say(ctx, text); err != nil {
		return err
	}
	return r.Run(ctx, text, onEvent)
}

func (f *fakeAgent) Start(context.Context, agent.Config) error          { return nil }
func (f *fakeAgent) Resume(context.Context, agent.Config, string) error { return nil }
func (f *fakeAgent) Events() <-chan agent.Event                         { return f.events }
func (f *fakeAgent) Interrupt() error                                   { return nil }

func (f *fakeAgent) Send(_ context.Context, prompt string) error {
	if len(f.prompts) == 0 {
		f.events <- agent.Event{Kind: agent.KindSession, SessionID: "s1"}
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

	var streamed []string
	for _, text := range []string{"one 1", "two 2"} {
		err := sendAs(ctx, r, text, func(ev agent.Event) {
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
	for _, tr := range turns {
		if tr.Status != "done" || tr.SessionID != "s1" || tr.Role != "architect" ||
			tr.Usage != (agent.Usage{InputTokens: 3, OutputTokens: 5}) {
			t.Errorf("turn = %+v, want done s1 architect with usage 3/5", tr)
		}
	}
}

// scriptAgent answers each prompt with its script of events.
type scriptAgent struct {
	events      chan agent.Event
	script      []agent.Event
	closeAfter  bool
	interrupted int
}

func (s *scriptAgent) Start(context.Context, agent.Config) error          { return nil }
func (s *scriptAgent) Resume(context.Context, agent.Config, string) error { return nil }
func (s *scriptAgent) Events() <-chan agent.Event                         { return s.events }

func (s *scriptAgent) Interrupt() error {
	s.interrupted++
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

func TestRunner_SendFailedTurn(t *testing.T) {
	tests := []struct {
		name        string
		script      []agent.Event
		closeAfter  bool
		cancelOn    agent.Kind
		wantErr     string
		wantTurnErr string
		wantSession string
		wantStopped int
	}{
		{
			name:        "cancelled mid-turn",
			script:      []agent.Event{{Kind: agent.KindText, Text: "par"}},
			cancelOn:    agent.KindText,
			wantErr:     "context canceled",
			wantTurnErr: "interrupted",
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
		},
		{
			name:        "events closed",
			script:      []agent.Event{{Kind: agent.KindText, Text: "par"}},
			closeAfter:  true,
			wantErr:     "agent exited",
			wantTurnErr: "agent exited",
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

			err = sendAs(ctx, r, "hi", func(ev agent.Event) {
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
			if len(msgs) != 1 || msgs[0].Author != "user" {
				t.Errorf("messages = %+v, want only the user message", msgs)
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

	if err := sendAs(ctx, r, "hi", func(agent.Event) {}); err == nil || err.Error() != "context canceled" {
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

func TestRunner_SendToolCapIs25(t *testing.T) {
	tests := []struct {
		name    string
		give    int
		wantErr string
	}{
		{"at the cap", 25, ""},
		{"over the cap", 26, "capped: more than 25 tool calls"},
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
			var script []agent.Event
			for range tt.give {
				script = append(script, agent.Event{Kind: agent.KindToolCall})
			}
			sa := &scriptAgent{
				events: make(chan agent.Event, 32),
				script: append(script, agent.Event{Kind: agent.KindTurnEnd}),
			}
			r := NewRunner(st, sa, ch.ID, "architect", "claude", "")

			err = sendAs(bg, r, "hi", func(agent.Event) {})
			if got := fmt.Sprint(err); (tt.wantErr == "" && err != nil) ||
				(tt.wantErr != "" && got != tt.wantErr) {
				t.Errorf("Send error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestRunner_SendCapsToolCalls(t *testing.T) {
	bg := context.Background()
	st, f := openStore(t)
	ch, err := st.CreateChannel(bg, store.Channel{
		ProjectID: f.ProjectID, FeatureID: &f.ID, Name: "chat",
	})
	if err != nil {
		t.Fatal(err)
	}
	call := agent.Event{Kind: agent.KindToolCall, ToolName: "Read"}
	sa := &scriptAgent{
		events: make(chan agent.Event, 8),
		script: []agent.Event{call, call, call, {Kind: agent.KindTurnEnd}},
	}
	r := NewRunner(st, sa, ch.ID, "architect", "claude", "")
	r.toolCap = 2

	err = sendAs(bg, r, "hi", func(agent.Event) {})
	if !errors.Is(err, ErrCapped) || err.Error() != "capped: more than 2 tool calls" {
		t.Errorf("Send error = %v, want ErrCapped %q", err, "capped: more than 2 tool calls")
	}
	if sa.interrupted != 1 {
		t.Errorf("agent interrupted %d times, want 1", sa.interrupted)
	}
	turns, err := st.Turns(bg, store.TurnFilter{ChannelID: &ch.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 1 || turns[0].Status != "failed" ||
		turns[0].Error != "capped: more than 2 tool calls" {
		t.Errorf("turns = %+v, want one failed turn capped at 2 tool calls", turns)
	}
	got, err := st.Channel(bg, ch.ID)
	if err != nil || got.Paused != "capped: more than 2 tool calls" {
		t.Errorf("channel = %+v, %v, want paused with the reason", got, err)
	}
}
