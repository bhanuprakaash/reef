package store

import (
	"context"
	"errors"
	"fmt"
)

// Who moves a Work item. Each may make only the moves MoveWorkItem allows it.
const (
	ByUser   = "user"
	ByQueue  = "queue"
	ByOwner  = "owner"
	BySpace  = "space"
	BySubmit = "submit"
	ByReef   = "reef"
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

func validMover(by string) bool {
	switch by {
	case ByUser, ByQueue, ByOwner, BySpace, BySubmit, ByReef:
		return true
	}
	return false
}

// MoveWorkItem sets the status of the item to to, if by may make that move, and returns the
// item. A refused move is ErrInvalid and names the rule. The rules:
//
//   - backlog to queued: not a move, ApproveWorkItem authorizes and queues a top-level item. Queued
//     back to backlog: the user, which withdraws the approval.
//   - queued to in_progress: the queue.
//   - in_progress to needs_you and back: the Space, when it pauses and continues.
//   - in_progress to review: submit, and only when every sub-item is done or dropped.
//   - review to done: the user, on a top-level item. Review to queued, to send it back: the user,
//     or Reef when a check of its request fails.
//   - any status but dropped to dropped: the user. A top-level item that drops is closed, with
//     its Spaces.
//   - a top-level item joining the queue goes last, and one sent back from review goes first;
//     a queued item with an owner then moves to in_progress at once.
//   - a sub-item: the owner moves it to any other status while it is not done or dropped, and
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
	if to == _queued && w.ParentID == nil && w.OwnerBotID == nil {
		return WorkItem{}, fmt.Errorf("%w: work item %d needs an owner before it is queued", ErrInvalid, id)
	}
	if w.ParentID == nil && (to == _queued || by == ByQueue) {
		name, err := s.ItemInactiveTarget(ctx, id)
		if err != nil {
			return WorkItem{}, err
		}
		if name != "" {
			return WorkItem{}, fmt.Errorf("work item %d targets deleted project %s, adjust its repositories first: %w",
				id, name, ErrConflict)
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return WorkItem{}, fmt.Errorf("begin move work item: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx,
		"UPDATE work_item SET status = ? WHERE id = ? AND status = ?", to, id, w.Status)
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
	if w.ParentID == nil {
		if err := requeue(ctx, tx, w, to); err != nil {
			return WorkItem{}, err
		}
		if to == _backlog {
			if _, err := tx.ExecContext(ctx,
				"UPDATE work_item SET approved_at = NULL, approval_key = NULL WHERE id = ?", id); err != nil {
				return WorkItem{}, fmt.Errorf("withdraw approval: %w", err)
			}
		}
	}
	if to == _dropped && w.ParentID == nil {
		if err := s.closeInTx(ctx, tx, id); err != nil {
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
	if to == _queued && w.ParentID == nil {
		if err := s.StartQueues(ctx); err != nil {
			return WorkItem{}, err
		}
	}
	return s.WorkItem(ctx, id)
}

func checkMove(from, to, by string, sub bool) error {
	switch {
	case !validStatus(to):
		return fmt.Errorf("unknown status %q", to)
	case !validMover(by):
		return fmt.Errorf("unknown mover %q", by)
	case from == to:
		return fmt.Errorf("already %s", from)
	}
	switch by {
	case ByUser:
		switch {
		case from == _backlog && to == _queued && !sub:
			return errors.New("approve it to queue it")
		case to == _dropped,
			from == _backlog && to == _queued,
			from == _queued && to == _backlog,
			from == _review && to == _queued,
			from == _review && to == _done && !sub:
			return nil
		case from == _review && to == _done:
			return errors.New("only the owner sets a sub-item done")
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
	case ByReef:
		if from == _review && to == _queued && !sub {
			return nil
		}
	case ByOwner:
		if !sub {
			return errors.New("the owner moves sub-items only; the user sets a top-level item done")
		}
		if from != _done && from != _dropped {
			return nil
		}
	}
	return fmt.Errorf("%s cannot move an item from %s to %s", by, from, to)
}
