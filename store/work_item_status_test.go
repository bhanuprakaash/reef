package store

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestCheckMove(t *testing.T) {
	allowed := map[string][]string{
		"user top": {
			"backlog>queued", "queued>backlog", "review>queued", "review>done",
			"backlog>dropped", "queued>dropped", "in_progress>dropped", "needs_you>dropped",
			"review>dropped", "done>dropped",
		},
		"user sub": {
			"backlog>queued", "queued>backlog", "review>queued",
			"backlog>dropped", "queued>dropped", "in_progress>dropped", "needs_you>dropped",
			"review>dropped", "done>dropped",
		},
		"queue top":  {"queued>in_progress"},
		"queue sub":  {"queued>in_progress"},
		"space top":  {"in_progress>needs_you", "needs_you>in_progress"},
		"space sub":  {"in_progress>needs_you", "needs_you>in_progress"},
		"submit top": {"in_progress>review"},
		"submit sub": {"in_progress>review"},
		"lead top":   {},
		"lead sub": {
			"backlog>queued", "backlog>in_progress", "backlog>needs_you", "backlog>review", "backlog>done", "backlog>dropped",
			"queued>backlog", "queued>in_progress", "queued>needs_you", "queued>review", "queued>done", "queued>dropped",
			"in_progress>backlog", "in_progress>queued", "in_progress>needs_you", "in_progress>review", "in_progress>done", "in_progress>dropped",
			"needs_you>backlog", "needs_you>queued", "needs_you>in_progress", "needs_you>review", "needs_you>done", "needs_you>dropped",
			"review>backlog", "review>queued", "review>in_progress", "review>needs_you", "review>done", "review>dropped",
		},
	}
	for _, by := range _movers {
		for _, kind := range []string{"top", "sub"} {
			want := allowed[by+" "+kind]
			for _, from := range _statuses {
				for _, to := range _statuses {
					move := from + ">" + to
					err := checkMove(from, to, by, kind == "sub")
					if got := err == nil; got != slices.Contains(want, move) {
						t.Errorf("%s %s %s: err = %v, want allowed = %v", by, kind, move, err, !got)
					}
				}
			}
		}
	}
}

func TestMoveWorkItem(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.CreateProject(ctx, Project{Name: "reef", Path: "/tmp/reef"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateWorkItem(ctx, WorkItem{ProjectID: 1, Title: "Login", Status: _inProgress}); err != nil {
		t.Fatal(err)
	}
	parent := int64(1)
	if _, err := s.CreateWorkItem(ctx, WorkItem{ProjectID: 1, ParentID: &parent, Title: "Form", Status: _inProgress}); err != nil {
		t.Fatal(err)
	}

	_, err = s.MoveWorkItem(ctx, 1, _review, BySubmit)
	if !errors.Is(err, ErrInvalid) ||
		err.Error() != `invalid: work item 1 cannot go to review while sub-item 2 "Form" is in_progress` {
		t.Errorf("review with an open sub-item = %v, want ErrInvalid naming sub-item 2", err)
	}
	if got, err := s.MoveWorkItem(ctx, 2, _done, ByLead); err != nil || got.Status != _done {
		t.Fatalf("lead done on the sub-item = %+v, %v, want done", got, err)
	}
	if got, err := s.MoveWorkItem(ctx, 1, _review, BySubmit); err != nil || got.Status != _review {
		t.Errorf("review once the sub-item is done = %+v, %v, want review", got, err)
	}
	_, err = s.MoveWorkItem(ctx, 1, _done, ByLead)
	if err == nil || !strings.HasPrefix(err.Error(), "invalid: work item 1: the lead moves sub-items only") {
		t.Errorf("lead done on a top-level item = %v, want the rule named", err)
	}
	if got, err := s.MoveWorkItem(ctx, 1, _done, ByUser); err != nil || got.Status != _done {
		t.Errorf("user done = %+v, %v, want done", got, err)
	}
	if _, err := s.MoveWorkItem(ctx, 9, _done, ByUser); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown item = %v, want ErrNotFound", err)
	}
}
