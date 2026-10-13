package store

import (
	"context"
	"reflect"
	"testing"
)

type turnRow struct {
	ID     int64
	Status string
	Error  string
}

type messageRow struct {
	Author string
	Body   string
}

func turnRows(t *testing.T, s *Store) []turnRow {
	t.Helper()
	ts, err := s.Turns(context.Background(), TurnFilter{})
	if err != nil {
		t.Fatal(err)
	}
	var rows []turnRow
	for _, tr := range ts {
		rows = append(rows, turnRow{tr.ID, tr.Status, tr.Error})
	}
	return rows
}

func messageRows(t *testing.T, s *Store, channelID int64) []messageRow {
	t.Helper()
	ms, err := s.Messages(context.Background(), channelID, 0)
	if err != nil {
		t.Fatal(err)
	}
	var rows []messageRow
	for _, m := range ms {
		rows = append(rows, messageRow{m.Author, m.Body})
	}
	return rows
}

func TestFailRunningTurns(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	if _, err := s.CreateProject(ctx, Project{Name: "reef", Path: "/tmp/reef"}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a", "b", "c"} {
		if _, err := s.CreateChannel(ctx, Channel{ProjectID: 1, Name: name}); err != nil {
			t.Fatal(err)
		}
	}
	for _, tr := range []Turn{
		{ChannelID: 1, Role: "architect", Backend: "claude"},
		{ChannelID: 2, Role: "developer", Backend: "codex"},
		{ChannelID: 1, Role: "developer", Backend: "claude"},
		{ChannelID: 3, Role: "tester", Backend: "claude"},
	} {
		if _, err := s.CreateTurn(ctx, tr); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.EndTurn(ctx, Turn{ID: 4, Status: "done"}); err != nil {
		t.Fatal(err)
	}

	failed, err := s.FailRunningTurns(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(failed) != 3 {
		t.Errorf("failed %d turns, want 3", len(failed))
	}

	wantTurns := []turnRow{
		{1, "failed", "server restarted"},
		{2, "failed", "server restarted"},
		{3, "failed", "server restarted"},
		{4, "done", ""},
	}
	if got := turnRows(t, s); !reflect.DeepEqual(got, wantTurns) {
		t.Errorf("turns = %v, want %v", got, wantTurns)
	}
	tr, err := s.Turn(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if tr.EndedAt == nil {
		t.Error("turn 1 has no endedAt")
	}

	wantMessages := map[int64][]messageRow{
		1: {{"reef", "Turn 1 by architect, turn 3 by developer failed: server restarted. The worktree may be dirty; retry or roll back."}},
		2: {{"reef", "Turn 2 by developer failed: server restarted. The worktree may be dirty; retry or roll back."}},
		3: nil,
	}
	for ch, want := range wantMessages {
		if got := messageRows(t, s, ch); !reflect.DeepEqual(got, want) {
			t.Errorf("channel %d messages = %v, want %v", ch, got, want)
		}
	}

	failed, err = s.FailRunningTurns(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(failed) != 0 {
		t.Errorf("second run failed %d turns, want 0", len(failed))
	}
	for ch, want := range wantMessages {
		if got := messageRows(t, s, ch); !reflect.DeepEqual(got, want) {
			t.Errorf("channel %d messages after second run = %v, want %v", ch, got, want)
		}
	}
}
