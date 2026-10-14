package store

import (
	"context"
	"errors"
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
		rows = append(rows, turnRow{ID: tr.ID, Status: tr.Status, Error: tr.Error})
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

	failed, err := s.FailRunningTurns(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(failed) != 3 {
		t.Errorf("failed %d turns, want 3", len(failed))
	}

	wantTurns := []turnRow{
		{ID: 1, Status: "failed", Error: "server restarted"},
		{ID: 2, Status: "failed", Error: "server restarted"},
		{ID: 3, Status: "failed", Error: "server restarted"},
		{ID: 4, Status: "done"},
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

	failed, err = s.FailRunningTurns(ctx, nil)
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

func TestSetTurnSession(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	if _, err := s.CreateProject(ctx, Project{Name: "reef", Path: "/tmp/reef"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateChannel(ctx, Channel{ProjectID: 1, Name: "a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateTurn(ctx, Turn{ChannelID: 1, Role: "architect", Backend: "claude"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetTurnSession(ctx, 1, "abc"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FailRunningTurns(ctx, nil); err != nil {
		t.Fatal(err)
	}
	tr, err := s.Turn(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if tr.Status != "failed" || tr.SessionID != "abc" {
		t.Errorf("turn = %q %q, want failed abc", tr.Status, tr.SessionID)
	}
	if _, err := s.SetTurnSession(ctx, 1, "def"); !errors.Is(err, ErrInvalid) {
		t.Errorf("session on ended turn: err = %v, want ErrInvalid", err)
	}
}

func TestSetTurnRetried(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.CreateProject(ctx, Project{Name: "reef", Path: "/tmp/reef"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateChannel(ctx, Channel{ProjectID: 1, Name: "a"}); err != nil {
		t.Fatal(err)
	}
	msg, err := s.CreateMessage(ctx, Message{ChannelID: 1, Author: "user", Body: "go"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateTurn(ctx, Turn{
		ChannelID: 1, Role: "developer", Backend: "claude", WakeText: "build it", WakeParent: &msg.ID,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateTurn(ctx, Turn{ChannelID: 1, Role: "tester", Backend: "claude"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.EndTurn(ctx, Turn{ID: 1, Status: "failed", Error: "boom"}); err != nil {
		t.Fatal(err)
	}

	if err := s.SetTurnRetried(ctx, 2, true); !errors.Is(err, ErrConflict) {
		t.Errorf("retry a running turn = %v, want ErrConflict", err)
	}
	if err := s.SetTurnRetried(ctx, 9, true); !errors.Is(err, ErrNotFound) {
		t.Errorf("retry an unknown turn = %v, want ErrNotFound", err)
	}
	if err := s.SetTurnRetried(ctx, 1, true); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTurnRetried(ctx, 1, true); !errors.Is(err, ErrConflict) {
		t.Errorf("retry twice = %v, want ErrConflict", err)
	}
	got, err := s.Turn(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Retried || got.WakeText != "build it" || got.WakeParent == nil || *got.WakeParent != 1 {
		t.Errorf("turn = %+v, want retried with wake %q in thread 1", got, "build it")
	}
	if err := s.SetTurnRetried(ctx, 1, false); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Turn(ctx, 1); got.Retried {
		t.Errorf("turn = %+v, want not retried", got)
	}
}
