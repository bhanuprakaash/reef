package store

import (
	"context"
	"fmt"
	"testing"
)

func TestPauseInterruptedPausesCutWorkAndKeepsOtherPauses(t *testing.T) {
	ctx := context.Background()
	s, bot := queueFixture(t, 4)
	var spaces []int64
	for id := int64(1); id <= 4; id++ {
		if _, err := s.ApproveWorkItem(ctx, id, &bot, nil, nil, fmt.Sprint("k", id)); err != nil {
			t.Fatal(err)
		}
		sp, err := s.CreateSpaceOf(ctx, Space{ProjectID: 1, WorkItemID: &id, Name: "s", Role: "ann"}, []int64{bot}, bot)
		if err != nil {
			t.Fatal(err)
		}
		spaces = append(spaces, sp.ID)
	}
	// Item 1: a turn was running. Item 2: the user paused it. Item 3: its first message was
	// stored and no turn followed. Item 4: untouched, first turn still to come.
	if _, err := s.CreateTurn(ctx, Turn{SpaceID: spaces[0], Role: "ann", Backend: "claude"}); err != nil {
		t.Fatal(err)
	}
	if err := s.PauseSpace(ctx, spaces[1], "paused by the user"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateMessage(ctx, Message{SpaceID: spaces[2], Author: "user", Body: "start"}); err != nil {
		t.Fatal(err)
	}
	failed, err := s.FailRunningTurns(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.PauseInterrupted(ctx, failed); err != nil {
		t.Fatal(err)
	}

	for i, want := range []string{InterruptedReason, "paused by the user", InterruptedReason, ""} {
		sp, err := s.Space(ctx, spaces[i])
		if err != nil {
			t.Fatal(err)
		}
		if sp.Paused != want {
			t.Errorf("Space %d paused = %q, want %q", spaces[i], sp.Paused, want)
		}
	}
	if got, want := queueState(t, s, 1, 2, 3, 4), "1:needs_you:- 2:needs_you:- 3:needs_you:- 4:in_progress:-"; got != want {
		t.Errorf("items = %q, want %q", got, want)
	}
}
