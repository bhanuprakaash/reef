package store

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestCloseWorkItemClosesItsSpacesAndReopenKeepsThemClosed(t *testing.T) {
	ctx := context.Background()
	s := proposalFixture(t)
	if err := s.CloseWorkItem(ctx, 1); err != nil {
		t.Fatal(err)
	}

	w, err := s.WorkItem(ctx, 1)
	if err != nil || w.ClosedAt == nil {
		t.Fatalf("work item 1 closedAt = %v, err = %v, want set", w.ClosedAt, err)
	}
	c, err := s.Space(ctx, 1)
	if err != nil || c.ClosedAt == nil {
		t.Errorf("space 1 closedAt = %v, err = %v, want set", c.ClosedAt, err)
	}
	if q, err := s.Space(ctx, 2); err != nil || q.ClosedAt != nil {
		t.Errorf("quick session closedAt = %v, err = %v, want nil", q.ClosedAt, err)
	}

	if err := s.ReopenWorkItem(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if w, err = s.WorkItem(ctx, 1); err != nil || w.ClosedAt != nil {
		t.Errorf("work item 1 closedAt after reopen = %v, err = %v, want nil", w.ClosedAt, err)
	}
	if c, err = s.Space(ctx, 1); err != nil || c.ClosedAt == nil {
		t.Errorf("space 1 closedAt after reopen = %v, err = %v, want still set", c.ClosedAt, err)
	}
	if err := s.CloseWorkItem(ctx, 99); !errors.Is(err, ErrNotFound) {
		t.Errorf("close work item 99 = %v, want ErrNotFound", err)
	}
}

func TestWorkItemWithLinesAndSubItems(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.CreateProject(ctx, Project{Name: "reef", Path: "/tmp/reef"}); err != nil {
		t.Fatal(err)
	}

	bug, err := s.CreateWorkItem(ctx, WorkItem{
		ProjectID: 1, Type: "bug", Title: "Sign-in fails", Description: "500 on submit",
		AcceptanceLines: []AcceptanceLine{{Text: "no 500"}, {Text: "error shown"}, {Text: "test added"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if bug.Status != "backlog" || bug.TokenBudget != 1_000_000 || bug.ParentID != nil {
		t.Errorf("bug = %+v, want status backlog, budget 1000000, no parent", bug)
	}
	wantLines := []AcceptanceLine{
		{Position: 1, Text: "no 500"}, {Position: 2, Text: "error shown"}, {Position: 3, Text: "test added"},
	}
	if !reflect.DeepEqual(bug.AcceptanceLines, wantLines) {
		t.Errorf("lines = %+v, want %+v", bug.AcceptanceLines, wantLines)
	}

	sub, err := s.CreateWorkItem(ctx, WorkItem{ProjectID: 1, ParentID: &bug.ID, Type: "task", Title: "repro"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.CreateWorkItem(ctx, WorkItem{ProjectID: 1, ParentID: &sub.ID, Title: "nested"})
	if want := "invalid: work item 2 is a sub-item and cannot have sub-items"; err == nil || err.Error() != want {
		t.Errorf("sub-item of a sub-item = %v, want %q", err, want)
	}
	_, err = s.PutBrief(ctx, Brief{WorkItemID: sub.ID, Body: "b", Author: "user"})
	if want := "invalid: work item 2 is a sub-item and has no brief"; err == nil || err.Error() != want {
		t.Errorf("brief on a sub-item = %v, want %q", err, want)
	}
	if _, err := s.PutBrief(ctx, Brief{WorkItemID: bug.ID, Body: "b", Author: "user"}); err != nil {
		t.Errorf("brief on a top-level item = %v, want nil", err)
	}

	kids, err := s.WorkItems(ctx, WorkItemFilter{ParentID: &bug.ID, Type: "task", Status: "backlog"})
	if err != nil || len(kids) != 1 || kids[0].ID != sub.ID {
		t.Errorf("sub-items = %+v, %v, want only item %d", kids, err, sub.ID)
	}
	if _, err := s.WorkItems(ctx, WorkItemFilter{Status: "open"}); !errors.Is(err, ErrInvalid) {
		t.Errorf("status open = %v, want ErrInvalid", err)
	}
}

func TestUpdateWorkItemKeepsLinesWhenOmitted(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.CreateProject(ctx, Project{Name: "reef", Path: "/tmp/reef"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateWorkItem(ctx, WorkItem{
		ProjectID: 1, Title: "A", AcceptanceLines: []AcceptanceLine{{Text: "x"}},
	}); err != nil {
		t.Fatal(err)
	}

	got, err := s.UpdateWorkItem(ctx, WorkItem{ID: 1, Title: "B", Status: "queued"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "B" || got.Status != "queued" || got.Type != "feature" || len(got.AcceptanceLines) != 1 {
		t.Errorf("update without lines = %+v, want title B, queued, feature, one line", got)
	}
	got, err = s.UpdateWorkItem(ctx, WorkItem{ID: 1, Title: "B", AcceptanceLines: []AcceptanceLine{}})
	if err != nil || len(got.AcceptanceLines) != 0 || got.Status != "queued" {
		t.Errorf("update with empty lines = %+v, %v, want no lines, still queued", got, err)
	}
	if _, err := s.UpdateWorkItem(ctx, WorkItem{ID: 1, Title: "B", Type: "epic"}); !errors.Is(err, ErrInvalid) {
		t.Errorf("type epic = %v, want ErrInvalid", err)
	}
}
