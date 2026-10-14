package store

import (
	"context"
	"errors"
	"testing"
)

func TestSetTurnCheckpoint(t *testing.T) {
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
	if _, err := s.CreateTurn(ctx, Turn{ChannelID: 1, Role: "developer", Backend: "claude"}); err != nil {
		t.Fatal(err)
	}
	if tr, err := s.Turn(ctx, 1); err != nil || tr.Checkpoint != nil {
		t.Fatalf("new turn = %+v, %v, want no checkpoint", tr, err)
	}
	tr, err := s.SetTurnCheckpoint(ctx, 1, "refs/reef/checkpoints/1")
	if err != nil || tr.Checkpoint == nil || *tr.Checkpoint != "refs/reef/checkpoints/1" {
		t.Fatalf("SetTurnCheckpoint = %+v, %v, want refs/reef/checkpoints/1", tr, err)
	}
	if _, err := s.EndTurn(ctx, Turn{ID: 1, Status: "done"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetTurnCheckpoint(ctx, 1, "refs/reef/checkpoints/2"); !errors.Is(err, ErrInvalid) {
		t.Errorf("checkpoint on ended turn: err = %v, want ErrInvalid", err)
	}
	if tr, err := s.Turn(ctx, 1); err != nil || *tr.Checkpoint != "refs/reef/checkpoints/1" {
		t.Errorf("turn after end = %+v, %v, want checkpoint kept", tr, err)
	}
}
