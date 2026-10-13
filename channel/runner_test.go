package channel

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/store"
)

// fakeAgent answers each prompt with text, usage and a turn end. Like codex, it
// reports its session once.
type fakeAgent struct {
	events  chan agent.Event
	prompts []string
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
		err := r.Send(ctx, text, func(ev agent.Event) {
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
