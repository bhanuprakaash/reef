package store

import (
	"context"
	"fmt"
	"slices"
)

// Who moves a Work item. Each may make only the moves MoveWorkItem allows it.
const (
	ByUser   = "user"
	ByQueue  = "queue"
	ByLead   = "lead"
	BySpace  = "space"
	BySubmit = "submit"
)

const (
	_backlog    = "backlog"
	_queued     = "queued"
	_inProgress = "in_progress"
	_needsYou   = "needs_you"
	_review     = "review"
	_done       = "done"
	_dropped    = "dropped"
)

var _movers = []string{ByUser, ByQueue, ByLead, BySpace, BySubmit}

// MoveWorkItem sets the status of the item to to, if by may make that move, and returns the
// item. A refused move is ErrInvalid and names the rule. The rules:
//
//   - backlog to queued and back: the user.
//   - queued to in_progress: the queue.
//   - in_progress to needs_you and back: the Space, when it pauses and continues.
//   - in_progress to review: submit, and only when every sub-item is done or dropped.
//   - review to done: the user, on a top-level item. Review to queued, to send it back: the user.
//   - any status but dropped to dropped: the user.
//   - a top-level item with a Team: a Team has one item in progress or needing the user at a
//     time. When one leaves those statuses, or an item joins the queue, the Team's first queued
//     item moves to in_progress, if the Team is free. An item joining the queue goes last; one
//     sent back from review goes first.
//   - a sub-item: the lead moves it to any other status while it is not done or dropped, and
//     is the one that sets it done.
func (s *Store) MoveWorkItem(ctx context.Context, id int64, to, by string) (WorkItem, error) {
	w, err := s.WorkItem(ctx, id)
	if err != nil {
		return WorkItem{}, err
	}
	if err := checkMove(w.Status, to, by, w.ParentID != nil); err != nil {
		return WorkItem{}, fmt.Errorf("%w: work item %d: %w", ErrInvalid, id, err)
	}
	if to == _review && w.ParentID == nil {
		subs, err := s.WorkItems(ctx, WorkItemFilter{ParentID: &id})
		if err != nil {
			return WorkItem{}, err
		}
		for _, sub := range subs {
			if sub.Status != _done && sub.Status != _dropped {
				return WorkItem{}, fmt.Errorf(
					"%w: work item %d cannot go to review while sub-item %d %q is %s",
					ErrInvalid, id, sub.ID, sub.Title, sub.Status)
			}
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return WorkItem{}, fmt.Errorf("begin move work item: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx,
		"UPDATE work_item SET status = ? WHERE id = ? AND status = ?", to, id, w.Status)
	if isUnique(err) {
		return WorkItem{}, fmt.Errorf("%w: team %d is busy with another work item", ErrConflict, *w.TeamID)
	}
	if err != nil {
		return WorkItem{}, fmt.Errorf("move work item: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return WorkItem{}, fmt.Errorf("move work item: %w", err)
	}
	if n == 0 {
		return WorkItem{}, fmt.Errorf("work item %d changed status meanwhile: %w", id, ErrConflict)
	}
	teamed := w.ParentID == nil && w.TeamID != nil
	if teamed {
		if err := requeue(ctx, tx, w, to); err != nil {
			return WorkItem{}, err
		}
	}
	if by == ByQueue {
		if err := addFYI(ctx, tx, KindStarted, id, "Started from the queue: "+w.Title); err != nil {
			return WorkItem{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return WorkItem{}, fmt.Errorf("commit move work item: %w", err)
	}
	if teamed && (to == _queued || holdsTeam(w.Status) && !holdsTeam(to)) {
		if err := s.startNext(ctx, *w.TeamID); err != nil {
			return WorkItem{}, err
		}
	}
	return s.WorkItem(ctx, id)
}

func checkMove(from, to, by string, sub bool) error {
	switch {
	case !slices.Contains(_statuses, to):
		return fmt.Errorf("unknown status %q", to)
	case !slices.Contains(_movers, by):
		return fmt.Errorf("unknown mover %q", by)
	case from == to:
		return fmt.Errorf("already %s", from)
	}
	switch by {
	case ByUser:
		switch {
		case to == _dropped,
			from == _backlog && to == _queued,
			from == _queued && to == _backlog,
			from == _review && to == _queued,
			from == _review && to == _done && !sub:
			return nil
		case from == _review && to == _done:
			return fmt.Errorf("only the lead sets a sub-item done")
		}
	case ByQueue:
		if from == _queued && to == _inProgress {
			return nil
		}
	case BySpace:
		if from == _inProgress && to == _needsYou || from == _needsYou && to == _inProgress {
			return nil
		}
	case BySubmit:
		if from == _inProgress && to == _review {
			return nil
		}
	case ByLead:
		if !sub {
			return fmt.Errorf("the lead moves sub-items only; the user sets a top-level item done")
		}
		if from != _done && from != _dropped {
			return nil
		}
	}
	return fmt.Errorf("%s cannot move an item from %s to %s", by, from, to)
}
