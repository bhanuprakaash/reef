package space

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/store"
)

// askAgent parks a turn whose prompt holds PARK on a permission prompt for a Bash call, and
// finishes it when answered, with the reason of a denial in its reply.
type askAgent struct {
	events  chan agent.Event
	answers []agent.Answer
}

func newAskAgent() *askAgent { return &askAgent{events: make(chan agent.Event, 8)} }

func (a *askAgent) Start(context.Context, agent.Config) error          { return nil }
func (a *askAgent) Resume(context.Context, agent.Config, string) error { return nil }
func (a *askAgent) Events() <-chan agent.Event                         { return a.events }
func (a *askAgent) Interrupt() error                                   { return nil }
func (a *askAgent) factory(string) (agent.Agent, error)                { return a, nil }

func (a *askAgent) Send(_ context.Context, prompt string) error {
	a.events <- agent.Event{Kind: agent.KindText, Text: "looking"}
	if strings.Contains(prompt, "PARK") {
		a.events <- agent.Event{
			Kind: agent.KindPermission, RequestID: "req-1", ToolID: "t1", ToolName: "Bash",
			Input: []byte(`{"command":"gzip -l /x"}`), Rule: "Bash(gzip -l /x)",
		}
		return nil
	}
	a.events <- agent.Event{Kind: agent.KindTurnEnd}
	return nil
}

func (a *askAgent) Answer(_ string, ans agent.Answer) error {
	a.answers = append(a.answers, ans)
	a.events <- agent.Event{Kind: agent.KindText, Text: " " + ans.Reason + "ran"}
	a.events <- agent.Event{Kind: agent.KindTurnEnd}
	return nil
}

func TestRunner_PermissionParksTurnUntilAnswered(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch, err := st.CreateSpace(ctx, store.Space{ProjectID: f.ProjectID, WorkItemID: &f.ID, Name: "chat"})
	if err != nil {
		t.Fatal(err)
	}
	ag := newAskAgent()
	r := newTestRunner(t, st, ag, ch.ID, "architect", "claude", "")
	var seen []string
	r.OnPermission = func(p store.PermissionPrompt) { seen = append(seen, p.Status) }

	if err := r.Run(ctx, "PARK 1", nil, discard); !errors.Is(err, ErrWaiting) {
		t.Fatalf("Run = %v, want ErrWaiting", err)
	}
	prompts, err := st.SpacePermissionPrompts(ctx, ch.ID)
	if err != nil || len(prompts) != 1 || prompts[0].Status != store.PromptWaiting ||
		prompts[0].Tool != "Bash" || prompts[0].Rule != "Bash(gzip -l /x)" ||
		string(prompts[0].Input) != `{"command":"gzip -l /x"}` {
		t.Fatalf("prompts = %+v, %v, want one waiting Bash prompt", prompts, err)
	}
	if turn, err := st.Turn(ctx, 1); err != nil || turn.Status != "running" {
		t.Fatalf("turn = %+v, %v, want running", turn.Status, err)
	}

	if err := r.Answer(ctx, prompts[0].ID, agent.Answer{Allow: true}, false, discard); err != nil {
		t.Fatalf("Answer allow = %v, want the turn to end", err)
	}
	if turn, err := st.Turn(ctx, 1); err != nil || turn.Status != "done" || turn.Reply != "" {
		t.Errorf("turn = %+v, %v, want done", turn, err)
	}
	msgs, err := st.Messages(ctx, ch.ID, 0)
	if err != nil || len(msgs) != 1 || msgs[0].Body != "looking ran" {
		t.Errorf("messages = %+v, %v, want the one reply %q", msgs, err, "looking ran")
	}

	if err := r.Run(ctx, "PARK 2", nil, discard); !errors.Is(err, ErrWaiting) {
		t.Fatalf("second Run = %v, want ErrWaiting", err)
	}
	next, err := st.SpacePermissionPrompts(ctx, ch.ID)
	if err != nil || len(next) != 2 {
		t.Fatalf("prompts = %+v, %v, want two", next, err)
	}
	deny := agent.Answer{Reason: "no answer from the user"}
	if err := r.Answer(ctx, next[1].ID, deny, true, discard); err != nil {
		t.Fatal(err)
	}
	if got := ag.answers; len(got) != 2 || got[0].Allow != true || got[1] != deny {
		t.Errorf("answers = %+v, want an allow then %+v", got, deny)
	}
	if msgs, err := st.Messages(ctx, ch.ID, 0); err != nil || msgs[len(msgs)-1].Body != "looking no answer from the userran" {
		t.Errorf("messages = %+v, %v, want the denial in the agent's reply", msgs, err)
	}
	if len(seen) != 2 || seen[0] != store.PromptWaiting {
		t.Errorf("OnPermission saw %q, want two waiting prompts", seen)
	}
}

func TestSession_NewMessageDeniesWaitingPrompt(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch := sessionSpace(t, st, f, false)
	ag := newAskAgent()
	s, err := Open(ctx, st, testMemory(t, st), t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), ag.factory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	lead := *testBotID(t, st, "architect")

	if err := s.RunBot(ctx, lead, "PARK", nil, nil, discard); !errors.Is(err, ErrWaiting) {
		t.Fatalf("RunBot = %v, want ErrWaiting", err)
	}
	if err := s.RunBot(ctx, lead, "next", nil, nil, discard); err != nil {
		t.Fatalf("second RunBot = %v, want the new turn to run", err)
	}

	if want := _newMessageReason; len(ag.answers) != 1 || ag.answers[0].Allow || ag.answers[0].Reason != want {
		t.Errorf("answers = %+v, want one denial with %q", ag.answers, want)
	}
	if p, err := st.PermissionPrompt(ctx, 1); err != nil || p.Status != store.PromptDenied || p.Reason != _newMessageReason {
		t.Errorf("prompt = %+v, %v, want denied with %q", p, err, _newMessageReason)
	}
	turns, err := st.Turns(ctx, store.TurnFilter{SpaceID: &ch.ID})
	if err != nil || len(turns) != 2 || turns[0].Status != "done" || turns[1].Status != "done" {
		t.Errorf("turns = %+v, %v, want two done turns", turns, err)
	}
}

func TestSession_CloseDeniesWaitingPromptAsStopped(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch := sessionSpace(t, st, f, false)
	ag := newAskAgent()
	s, err := Open(ctx, st, testMemory(t, st), t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), ag.factory)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RunBot(ctx, *testBotID(t, st, "architect"), "PARK", nil, nil, discard); !errors.Is(err, ErrWaiting) {
		t.Fatalf("RunBot = %v, want ErrWaiting", err)
	}

	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	if p, err := st.PermissionPrompt(ctx, 1); err != nil || p.Status != store.PromptDenied || p.Reason != "stopped" {
		t.Errorf("prompt = %+v, %v, want denied with reason stopped", p, err)
	}
	if turn, err := st.Turn(ctx, 1); err != nil || turn.Status != "failed" || turn.Error != "interrupted" {
		t.Errorf("turn = %+v, %v, want failed, interrupted", turn, err)
	}
}
