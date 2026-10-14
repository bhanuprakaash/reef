package channel

import (
	"context"
	"slices"
	"testing"

	"github.com/bhanuprakaash/reef/store"
)

func reefNote(t *testing.T, st *store.Store, channelID int64, body string) {
	t.Helper()
	_, err := st.CreateMessage(context.Background(), store.Message{
		ChannelID: channelID, Author: _reefAuthor, Body: body,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRunner_NotesReachTheNextTurnOnce(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch := sessionChannel(t, st, f, false)
	ag := newRecordingAgent()
	r := NewRunner(st, ag, ch.ID, "developer", "claude", "")

	if err := r.Run(ctx, "[channel 1, message 1]\nlead wrote: @developer go", nil, discard); err != nil {
		t.Fatal(err)
	}
	reefNote(t, st, ch.ID, "Rolled back to before turn 1.")
	reefNote(t, st, ch.ID, "Restarted.")
	held := "[channel 1, message 2]\nlead wrote: @developer again"
	if err := r.Run(ctx, held, nil, discard); err != nil {
		t.Fatal(err)
	}
	if err := r.Run(ctx, "[channel 1, message 3]\nnext", nil, discard); err != nil {
		t.Fatal(err)
	}

	want := []string{
		"[channel 1, message 1]\nlead wrote: @developer go",
		"[channel 1, message 2]\nreef wrote: Rolled back to before turn 1.\nreef wrote: Restarted.\n\n" +
			"lead wrote: @developer again",
		"[channel 1, message 3]\nnext",
	}
	if !slices.Equal(ag.prompts, want) {
		t.Errorf("prompts = %q, want %q", ag.prompts, want)
	}
	turns, err := st.Turns(ctx, store.TurnFilter{ChannelID: &ch.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 3 || turns[1].WakeText != held {
		t.Errorf("turns = %+v, want turn 2 to keep the wake without the notes", turns)
	}
}

func TestRunner_NotesReachARetry(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch := sessionChannel(t, st, f, false)
	ag := newRecordingAgent()
	r := NewRunner(st, ag, ch.ID, "developer", "claude", "")
	const wake = "[channel 1, message 1]\nlead wrote: @developer go"
	failed, err := st.CreateTurn(ctx, store.Turn{
		ChannelID: ch.ID, Role: "developer", Backend: "claude", WakeText: wake,
	})
	if err != nil {
		t.Fatal(err)
	}
	failed.Status, failed.Error = "failed", "server restarted"
	if _, err := st.EndTurn(ctx, failed); err != nil {
		t.Fatal(err)
	}
	reefNote(t, st, ch.ID, "Rolled back to before turn 1.")

	if err := r.Run(ctx, wake, nil, discard); err != nil {
		t.Fatal(err)
	}
	if err := r.Run(ctx, "[channel 1, message 3]\nnext", nil, discard); err != nil {
		t.Fatal(err)
	}

	want := []string{
		"[channel 1, message 1]\nreef wrote: Rolled back to before turn 1.\n\nlead wrote: @developer go",
		"[channel 1, message 3]\nnext",
	}
	if !slices.Equal(ag.prompts, want) {
		t.Errorf("prompts = %q, want %q", ag.prompts, want)
	}
}

func TestRunner_NotesSkipARoleWithNoTurnAndFitPlainText(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch := sessionChannel(t, st, f, false)
	ag := newRecordingAgent()
	r := NewRunner(st, ag, ch.ID, "developer", "claude", "")

	reefNote(t, st, ch.ID, "early")
	if err := r.Run(ctx, "[channel 1, message 2]\nfirst", nil, discard); err != nil {
		t.Fatal(err)
	}
	reefNote(t, st, ch.ID, "later")
	if err := r.Run(ctx, "plain", nil, discard); err != nil {
		t.Fatal(err)
	}

	want := []string{"[channel 1, message 2]\nfirst", "reef wrote: later\n\nplain"}
	if !slices.Equal(ag.prompts, want) {
		t.Errorf("prompts = %q, want %q", ag.prompts, want)
	}
}
