package space

import (
	"context"
	"slices"
	"testing"

	"github.com/bhanuprakaash/reef/store"
)

func reefNote(t *testing.T, st *store.Store, spaceID int64, body string) {
	t.Helper()
	_, err := st.CreateMessage(context.Background(), store.Message{
		SpaceID: spaceID, Author: store.ReefAuthor, Body: body,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRunner_NotesReachTheNextTurnOnce(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch := sessionSpace(t, st, f, false)
	ag := newRecordingAgent()
	r := NewRunner(st, ag, ch.ID, "developer", "claude", "")
	r.Memory = testMemory(t, st)

	if err := r.Run(ctx, "[Space 1, message 1]\nlead wrote: @developer go", nil, discard); err != nil {
		t.Fatal(err)
	}
	reefNote(t, st, ch.ID, "Rolled back to before turn 1.")
	reefNote(t, st, ch.ID, "Restarted.")
	held := "[Space 1, message 2]\nlead wrote: @developer again"
	if err := r.Run(ctx, held, nil, discard); err != nil {
		t.Fatal(err)
	}
	if err := r.Run(ctx, "[Space 1, message 3]\nnext", nil, discard); err != nil {
		t.Fatal(err)
	}

	want := []string{
		"[Space 1, message 1]\nlead wrote: @developer go",
		"[Space 1, message 2]\nreef wrote: Rolled back to before turn 1.\nreef wrote: Restarted.\n\n" +
			"lead wrote: @developer again",
		"[Space 1, message 3]\nnext",
	}
	if !slices.Equal(ag.prompts, want) {
		t.Errorf("prompts = %q, want %q", ag.prompts, want)
	}
	turns, err := st.Turns(ctx, store.TurnFilter{SpaceID: &ch.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 3 || turns[1].WakeText != held {
		t.Errorf("turns = %+v, want turn 2 to keep the wake without the notes", turns)
	}
}

func TestRunner_NextPromptNamesAFailedRequestOnce(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch := sessionSpace(t, st, f, false)
	ag := newRecordingAgent()
	r := NewRunner(st, ag, ch.ID, "lead", "claude", "")
	r.Memory = testMemory(t, st)

	if err := r.Run(ctx, "[Space 1, message 1]\nhello", nil, discard); err != nil {
		t.Fatal(err)
	}
	failed, err := st.CreateDirectMessage(ctx, store.DirectMessage{
		SpaceID: ch.ID, From: "lead", To: "architect", Body: "Review the plan\nand the tests",
	})
	if err != nil {
		t.Fatal(err)
	}
	answered, err := st.CreateDirectMessage(ctx, store.DirectMessage{
		SpaceID: ch.ID, From: "lead", To: "tester", Body: "Run the checks",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateFailedDirectResponse(ctx, failed.ID, "architect", "failed"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateDirectResponse(ctx, answered.ID, "tester", "green"); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"[Space 1, message 2]\nagain", "[Space 1, message 3]\nonce more"} {
		if err := r.Run(ctx, text, nil, discard); err != nil {
			t.Fatal(err)
		}
	}

	want := []string{
		"[Space 1, message 1]\nhello",
		"[Space 1, message 2]\nreef wrote: Your direct request to architect failed: Review the plan\n\nagain",
		"[Space 1, message 3]\nonce more",
	}
	if !slices.Equal(ag.prompts, want) {
		t.Errorf("prompts = %q, want %q", ag.prompts, want)
	}
}

func TestRunner_NotesReachARetry(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch := sessionSpace(t, st, f, false)
	ag := newRecordingAgent()
	r := NewRunner(st, ag, ch.ID, "developer", "claude", "")
	r.Memory = testMemory(t, st)

	if err := r.Run(ctx, "[Space 1, message 1]\nlead wrote: @developer go", nil, discard); err != nil {
		t.Fatal(err)
	}
	reefNote(t, st, ch.ID, "Rolled back to before turn 1.")
	const retryWake = "[Space 1, message 2]\nlead wrote: @developer again"
	failed, err := st.CreateTurn(ctx, store.Turn{
		SpaceID: ch.ID, Role: "developer", Backend: "claude", WakeText: retryWake,
	})
	if err != nil {
		t.Fatal(err)
	}
	failed.Status, failed.Error = "failed", "stopped waiting for the tree"
	if _, err := st.EndTurn(ctx, failed); err != nil {
		t.Fatal(err)
	}

	if err := r.Run(ctx, retryWake, nil, discard); err != nil {
		t.Fatal(err)
	}
	if err := r.Run(ctx, "[Space 1, message 3]\nnext", nil, discard); err != nil {
		t.Fatal(err)
	}

	want := []string{
		"[Space 1, message 1]\nlead wrote: @developer go",
		"[Space 1, message 2]\nreef wrote: Rolled back to before turn 1.\n\n" +
			"lead wrote: @developer again",
		"[Space 1, message 3]\nnext",
	}
	if !slices.Equal(ag.prompts, want) {
		t.Errorf("prompts = %q, want %q: the turn that failed before it sent does not move since", ag.prompts, want)
	}
}

func TestRunner_PreambleKeepsNotes(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch := sessionSpace(t, st, f, false)
	ag := newRecordingAgent()
	// An earlier session of the role already sent a turn.
	prior := NewRunner(st, ag, ch.ID, "developer", "claude", "")
	if err := prior.Run(ctx, "[Space 1, message 1]\nlead wrote: @developer go", nil, discard); err != nil {
		t.Fatal(err)
	}
	reefNote(t, st, ch.ID, "Rolled back to before turn 1.")

	r := NewRunner(st, ag, ch.ID, "developer", "claude", "CONTEXT")
	if err := r.Run(ctx, "[Space 1, message 2]\nlead wrote: @developer again", nil, discard); err != nil {
		t.Fatal(err)
	}

	want := "CONTEXT\n\n[Space 1, message 2]\nreef wrote: Rolled back to before turn 1.\n\n" +
		"lead wrote: @developer again"
	if got := ag.prompts[len(ag.prompts)-1]; got != want {
		t.Errorf("prompt = %q, want %q: the preamble must not drop the notes", got, want)
	}
}

func TestRunner_NotesSkipARoleWithNoTurnAndFitPlainText(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch := sessionSpace(t, st, f, false)
	ag := newRecordingAgent()
	r := NewRunner(st, ag, ch.ID, "developer", "claude", "")
	r.Memory = testMemory(t, st)

	reefNote(t, st, ch.ID, "early")
	if err := r.Run(ctx, "[Space 1, message 2]\nfirst", nil, discard); err != nil {
		t.Fatal(err)
	}
	reefNote(t, st, ch.ID, "later")
	if err := r.Run(ctx, "plain", nil, discard); err != nil {
		t.Fatal(err)
	}

	want := []string{"[Space 1, message 2]\nfirst", "reef wrote: later\n\nplain"}
	if !slices.Equal(ag.prompts, want) {
		t.Errorf("prompts = %q, want %q", ag.prompts, want)
	}
}
