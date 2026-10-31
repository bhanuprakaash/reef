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

// queueFixture is a store with a project, a bot, and n backlog items; the bot is returned.
func queueFixture(t *testing.T, n int) (*Store, int64) {
	t.Helper()
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if _, err := s.CreateProject(ctx, Project{Name: "reef", Path: "/tmp/reef"}); err != nil {
		t.Fatal(err)
	}
	bot, err := s.CreateBot(ctx, Bot{Name: "ann", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= n; i++ {
		if _, err := s.CreateWorkItem(ctx, WorkItem{ProjectID: 1, Title: fmt.Sprint("item ", i)}); err != nil {
			t.Fatal(err)
		}
	}
	return s, bot.ID
}

// waiting puts the items in the queue without an owner, in order: the state an item that
// cannot start yet is in.
func waiting(t *testing.T, s *Store, ids ...int64) {
	t.Helper()
	for i, id := range ids {
		if _, err := s.db.Exec("UPDATE work_item SET status = 'queued', queue_position = ? WHERE id = ?", i+1, id); err != nil {
			t.Fatal(err)
		}
	}
}

func TestOwnedItemStartsWhenQueued(t *testing.T) {
	ctx := context.Background()
	s, bot := queueFixture(t, 3)
	if _, err := s.MoveWorkItem(ctx, 1, _queued, ByUser); !errors.Is(err, ErrInvalid) {
		t.Fatalf("queue without an owner = %v, want ErrInvalid", err)
	}
	for _, id := range []int64{1, 2} {
		if _, err := s.SetWorkItemParticipants(ctx, id, &bot, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := s.MoveWorkItem(ctx, id, _queued, ByUser); err != nil {
			t.Fatal(err)
		}
	}
	if got, want := queueState(t, s, 1, 2, 3), "1:in_progress:- 2:in_progress:- 3:backlog:-"; got != want {
		t.Errorf("state = %q, want %q", got, want)
	}

	if _, err := s.MoveWorkItem(ctx, 1, _review, BySubmit); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MoveWorkItem(ctx, 1, _queued, ByUser); err != nil {
		t.Fatal(err)
	}
	if got, want := queueState(t, s, 1, 2), "1:in_progress:- 2:in_progress:-"; got != want {
		t.Errorf("after send back = %q, want %q", got, want)
	}
}

func TestMoveInQueue(t *testing.T) {
	ctx := context.Background()
	s, _ := queueFixture(t, 5)
	waiting(t, s, 1, 2, 3, 4)

	for _, step := range []struct {
		id, position int64
		want         string
	}{
		{4, 1, "1:queued:2 2:queued:3 3:queued:4 4:queued:1"},
		{4, 3, "1:queued:1 2:queued:2 3:queued:4 4:queued:3"},
		{1, 2, "1:queued:2 2:queued:1 3:queued:4 4:queued:3"},
		{1, 2, "1:queued:2 2:queued:1 3:queued:4 4:queued:3"},
	} {
		w, err := s.MoveInQueue(ctx, step.id, step.position)
		if err != nil || *w.QueuePosition != step.position {
			t.Fatalf("move %d to %d: %v, %+v", step.id, step.position, err, w)
		}
		if got := queueState(t, s, 1, 2, 3, 4); got != step.want {
			t.Errorf("after moving %d to %d: %q, want %q", step.id, step.position, got, step.want)
		}
	}

	for _, bad := range []struct{ id, position int64 }{{4, 0}, {4, 5}, {5, 1}} {
		if _, err := s.MoveInQueue(ctx, bad.id, bad.position); !errors.Is(err, ErrInvalid) {
			t.Errorf("move %d to %d: err = %v, want ErrInvalid", bad.id, bad.position, err)
		}
	}
}

func TestDeleteAndDropKeepTheQueueRight(t *testing.T) {
	ctx := context.Background()
	s, _ := queueFixture(t, 4)
	waiting(t, s, 1, 2, 3, 4)

	if err := s.DeleteWorkItem(ctx, 2); err != nil {
		t.Fatal(err)
	}
	if got, want := queueState(t, s, 1, 3, 4), "1:queued:1 3:queued:2 4:queued:3"; got != want {
		t.Errorf("after deleting a queued item = %q, want %q", got, want)
	}
	if _, err := s.MoveWorkItem(ctx, 1, _dropped, ByUser); err != nil {
		t.Fatal(err)
	}
	if got, want := queueState(t, s, 1, 3, 4), "1:dropped:- 3:queued:1 4:queued:2"; got != want {
		t.Errorf("after dropping a queued item = %q, want %q", got, want)
	}
	if w, err := s.WorkItem(ctx, 1); err != nil || w.ClosedAt == nil {
		t.Errorf("dropped item closedAt = %v, %v, want set", w.ClosedAt, err)
	}
}

func TestStartQueuesAfterRestart(t *testing.T) {
	ctx := context.Background()
	s, bot := queueFixture(t, 3)
	waiting(t, s, 1, 2, 3)
	if _, err := s.db.Exec("UPDATE work_item SET owner_bot_id = ? WHERE id IN (1, 3)", bot); err != nil {
		t.Fatal(err)
	}
	if err := s.StartQueues(ctx); err != nil {
		t.Fatal(err)
	}
	if got, want := queueState(t, s, 1, 2, 3), "1:in_progress:- 2:queued:1 3:in_progress:-"; got != want {
		t.Errorf("state = %q, want %q", got, want)
	}
}
