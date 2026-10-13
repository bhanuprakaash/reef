package main

import (
	"context"
	"testing"

	"github.com/bhanuprakaash/reef/store"
)

func resumeFixture(t *testing.T) *store.Store {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.CreateProject(ctx, store.Project{Name: "shop", Path: "/tmp/shop"}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"one", "two"} {
		if _, err := st.CreateChannel(ctx, store.Channel{ProjectID: 1, Name: name}); err != nil {
			t.Fatal(err)
		}
	}
	return st
}

// addTurn adds a turn that ends as done, or stays running when done is false.
func addTurn(t *testing.T, st *store.Store, channelID int64, sessionID string, done bool) {
	t.Helper()
	ctx := context.Background()
	turn, err := st.CreateTurn(ctx, store.Turn{
		ChannelID: channelID, Role: "architect", Backend: "claude", SessionID: sessionID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !done {
		return
	}
	turn.Status = "done"
	if _, err := st.EndTurn(ctx, turn); err != nil {
		t.Fatal(err)
	}
}

func TestResumeTurn(t *testing.T) {
	ctx := context.Background()
	st := resumeFixture(t)
	addTurn(t, st, 1, "sess-old", true)
	addTurn(t, st, 1, "sess-new", true)
	addTurn(t, st, 1, "", false)
	addTurn(t, st, 2, "sess-other", true)

	got, err := resumeTurn(ctx, st, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != 2 || got.SessionID != "sess-new" {
		t.Errorf("resumeTurn = turn %d with session %q, want turn 2 with session %q",
			got.ID, got.SessionID, "sess-new")
	}
	stale, err := st.Turn(ctx, 3)
	if err != nil {
		t.Fatal(err)
	}
	if stale.Status != "failed" || stale.Error != "chat exited" {
		t.Errorf("stale turn = %q %q, want %q %q", stale.Status, stale.Error, "failed", "chat exited")
	}
	other, err := st.Turn(ctx, 4)
	if err != nil {
		t.Fatal(err)
	}
	if other.Status != "done" {
		t.Errorf("turn of another channel = %q, want %q", other.Status, "done")
	}
}

func TestResumeTurn_NoSession(t *testing.T) {
	st := resumeFixture(t)
	addTurn(t, st, 1, "", true)
	addTurn(t, st, 2, "sess-other", true)

	_, err := resumeTurn(context.Background(), st, 1)
	if err == nil || err.Error() != "channel 1 has no session to resume" {
		t.Errorf("resumeTurn error = %v, want %q", err, "channel 1 has no session to resume")
	}
}
