package store

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestAddTurnCheckpoint(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	for _, name := range []string{"app", "infra"} {
		if _, err := s.CreateProject(ctx, Project{Name: name, Path: "/tmp/" + name}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.CreateSpace(ctx, Space{ProjectID: 1, Name: "a"}); err != nil {
		t.Fatal(err)
	}
	item, err := s.CreateWorkItem(ctx, WorkItem{ProjectID: 1, Title: "x"})
	if err != nil {
		t.Fatal(err)
	}
	topID := item.ID
	for i, name := range []string{"app", "infra"} {
		if _, err := s.EnsureWorkspace(ctx, Workspace{
			ProjectID: int64(i + 1), WorkItemID: &topID, Dir: "/tmp/w/" + name, Branch: "work-item/1",
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.CreateTurn(ctx, Turn{SpaceID: 1, Role: "developer", Backend: "claude"}); err != nil {
		t.Fatal(err)
	}
	if tr, err := s.Turn(ctx, 1); err != nil || len(tr.Checkpoints) != 0 || tr.Checkpoints == nil {
		t.Fatalf("new turn = %+v, %v, want empty checkpoints", tr, err)
	}
	for _, c := range []struct {
		ws  int64
		ref string
	}{{2, "refs/reef/checkpoints/b"}, {1, "refs/reef/checkpoints/a"}} {
		if err := s.AddTurnCheckpoint(ctx, 1, c.ws, c.ref); err != nil {
			t.Fatal(err)
		}
	}
	want := []TurnCheckpoint{{1, 1, "refs/reef/checkpoints/a"}, {2, 2, "refs/reef/checkpoints/b"}}
	if tr, err := s.Turn(ctx, 1); err != nil || !reflect.DeepEqual(tr.Checkpoints, want) {
		t.Fatalf("checkpoints = %+v, %v, want %+v", tr.Checkpoints, err, want)
	}
	if _, err := s.EndTurn(ctx, Turn{ID: 1, Status: "done"}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddTurnCheckpoint(ctx, 1, 1, "refs/reef/checkpoints/c"); !errors.Is(err, ErrInvalid) {
		t.Errorf("checkpoint on ended turn: err = %v, want ErrInvalid", err)
	}
	if tr, err := s.Turn(ctx, 1); err != nil || !reflect.DeepEqual(tr.Checkpoints, want) {
		t.Errorf("turn after end = %+v, %v, want checkpoints kept", tr.Checkpoints, err)
	}
}
