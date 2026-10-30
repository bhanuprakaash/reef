package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func queueState(t *testing.T, s *Store, ids ...int64) string {
	t.Helper()
	var parts []string
	for _, id := range ids {
		w, err := s.WorkItem(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		pos := "-"
		if w.QueuePosition != nil {
			pos = fmt.Sprint(*w.QueuePosition)
		}
		parts = append(parts, fmt.Sprintf("%d:%s:%s", id, w.Status, pos))
	}
	return strings.Join(parts, " ")
}

func TestTeamQueue(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.CreateProject(ctx, Project{Name: "reef", Path: "/tmp/reef"}); err != nil {
		t.Fatal(err)
	}
	if _, err := testApplyTeam(t, s, ctx, Team{Name: "core", Roles: []Role{
		{Name: "lead", Lead: true, Backend: "claude"}}}); err != nil {
		t.Fatal(err)
	}
	solo, core := int64(1), int64(2)
	for i, team := range []int64{solo, solo, solo, core} {
		if _, err := s.CreateWorkItem(ctx, WorkItem{
			ProjectID: 1, Title: fmt.Sprint("item ", i+1), TeamID: &team}); err != nil {
			t.Fatal(err)
		}
	}
	move := func(id int64, to, by string) {
		t.Helper()
		if _, err := s.MoveWorkItem(ctx, id, to, by); err != nil {
			t.Fatalf("move %d to %s by %s: %v", id, to, by, err)
		}
	}
	check := func(want string) {
		t.Helper()
		if got := queueState(t, s, 1, 2, 3, 4); got != want {
			t.Errorf("state = %q, want %q", got, want)
		}
	}

	move(1, _queued, ByUser)
	move(2, _queued, ByUser)
	move(3, _queued, ByUser)
	move(4, _queued, ByUser)
	check("1:in_progress:- 2:queued:1 3:queued:2 4:in_progress:-")

	move(1, _needsYou, BySpace)
	check("1:needs_you:- 2:queued:1 3:queued:2 4:in_progress:-")
	move(1, _inProgress, BySpace)
	move(1, _review, BySubmit)
	check("1:review:- 2:in_progress:- 3:queued:1 4:in_progress:-")

	move(1, _queued, ByUser)
	check("1:queued:1 2:in_progress:- 3:queued:2 4:in_progress:-")
	move(2, _dropped, ByUser)
	check("1:in_progress:- 2:dropped:- 3:queued:1 4:in_progress:-")
}

func TestMoveInQueue(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.CreateProject(ctx, Project{Name: "reef", Path: "/tmp/reef"}); err != nil {
		t.Fatal(err)
	}
	if _, err := testApplyTeam(t, s, ctx, Team{Name: "core", Roles: []Role{
		{Name: "lead", Lead: true, Backend: "claude"}}}); err != nil {
		t.Fatal(err)
	}
	solo, core := int64(1), int64(2)
	for i, team := range []int64{solo, solo, solo, solo, core, core} {
		if _, err := s.CreateWorkItem(ctx, WorkItem{
			ProjectID: 1, Title: fmt.Sprint("item ", i+1), TeamID: &team}); err != nil {
			t.Fatal(err)
		}
	}
	for id := int64(1); id <= 6; id++ {
		if _, err := s.MoveWorkItem(ctx, id, _queued, ByUser); err != nil {
			t.Fatal(err)
		}
	}
	if got, want := queueState(t, s, 1, 2, 3, 4, 5, 6), "1:in_progress:- 2:queued:1 3:queued:2 4:queued:3 5:in_progress:- 6:queued:1"; got != want {
		t.Fatalf("state = %q, want %q", got, want)
	}

	for _, step := range []struct {
		id, position int64
		want         string
	}{
		{4, 1, "2:queued:2 3:queued:3 4:queued:1 6:queued:1"},
		{4, 3, "2:queued:1 3:queued:2 4:queued:3 6:queued:1"},
		{2, 2, "2:queued:2 3:queued:1 4:queued:3 6:queued:1"},
		{2, 2, "2:queued:2 3:queued:1 4:queued:3 6:queued:1"},
	} {
		w, err := s.MoveInQueue(ctx, step.id, step.position)
		if err != nil || *w.QueuePosition != step.position {
			t.Fatalf("move %d to %d: %v, %+v", step.id, step.position, err, w)
		}
		if got := queueState(t, s, 2, 3, 4, 6); got != step.want {
			t.Errorf("after moving %d to %d: %q, want %q", step.id, step.position, got, step.want)
		}
	}

	for _, bad := range []struct{ id, position int64 }{{4, 0}, {4, 4}, {1, 1}} {
		if _, err := s.MoveInQueue(ctx, bad.id, bad.position); !errors.Is(err, ErrInvalid) {
			t.Errorf("move %d to %d: err = %v, want ErrInvalid", bad.id, bad.position, err)
		}
	}
}

func TestDeleteAndUpdateKeepTheQueueRight(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.CreateProject(ctx, Project{Name: "reef", Path: "/tmp/reef"}); err != nil {
		t.Fatal(err)
	}
	if _, err := testApplyTeam(t, s, ctx, Team{Name: "core", Roles: []Role{
		{Name: "lead", Lead: true, Backend: "claude"}}}); err != nil {
		t.Fatal(err)
	}
	solo, core := int64(1), int64(2)
	for i := 1; i <= 4; i++ {
		if _, err := s.CreateWorkItem(ctx, WorkItem{ProjectID: 1, Title: fmt.Sprint("item ", i), TeamID: &solo}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.MoveWorkItem(ctx, int64(i), _queued, ByUser); err != nil {
			t.Fatal(err)
		}
	}
	if got, want := queueState(t, s, 1, 2, 3, 4), "1:in_progress:- 2:queued:1 3:queued:2 4:queued:3"; got != want {
		t.Fatalf("state = %q, want %q", got, want)
	}

	if _, err := s.UpdateWorkItem(ctx, WorkItem{ID: 2, Title: "renamed", TeamID: &core}); !errors.Is(err, ErrInvalid) {
		t.Errorf("team change of a queued item = %v, want ErrInvalid", err)
	}
	if _, err := s.UpdateWorkItem(ctx, WorkItem{ID: 2, Title: "renamed", TeamID: &solo}); err != nil {
		t.Fatal(err)
	}
	if got, want := queueState(t, s, 1, 2, 3, 4), "1:in_progress:- 2:queued:1 3:queued:2 4:queued:3"; got != want {
		t.Errorf("after update = %q, want %q", got, want)
	}

	if err := s.DeleteWorkItem(ctx, 3); err != nil {
		t.Fatal(err)
	}
	if got, want := queueState(t, s, 1, 2, 4), "1:in_progress:- 2:queued:1 4:queued:2"; got != want {
		t.Errorf("after deleting a queued item = %q, want %q", got, want)
	}
	if err := s.DeleteWorkItem(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if got, want := queueState(t, s, 2, 4), "2:in_progress:- 4:queued:1"; got != want {
		t.Errorf("after deleting the item in progress = %q, want %q", got, want)
	}

	if _, err := s.MoveWorkItem(ctx, 2, _dropped, ByUser); err != nil {
		t.Fatal(err)
	}
	if got, want := queueState(t, s, 2, 4), "2:dropped:- 4:in_progress:-"; got != want {
		t.Errorf("after dropping the item in progress = %q, want %q", got, want)
	}
	if w, err := s.WorkItem(ctx, 2); err != nil || w.ClosedAt == nil {
		t.Errorf("dropped item closedAt = %v, %v, want set", w.ClosedAt, err)
	}
}

func TestStartQueuesAfterRestart(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.CreateProject(ctx, Project{Name: "reef", Path: "/tmp/reef"}); err != nil {
		t.Fatal(err)
	}
	team := int64(1)
	for _, status := range []string{_queued, _queued, _review} {
		if _, err := s.CreateWorkItem(ctx, WorkItem{
			ProjectID: 1, Title: "item", TeamID: &team, Status: status}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.StartQueues(ctx); err != nil {
		t.Fatal(err)
	}
	if got, want := queueState(t, s, 1, 2, 3), "1:in_progress:- 2:queued:- 3:review:-"; got != want {
		t.Errorf("state = %q, want %q", got, want)
	}
	if err := s.StartQueues(ctx); err != nil {
		t.Fatal(err)
	}
	if got, want := queueState(t, s, 1, 2, 3), "1:in_progress:- 2:queued:- 3:review:-"; got != want {
		t.Errorf("state after a second start = %q, want %q", got, want)
	}
}
