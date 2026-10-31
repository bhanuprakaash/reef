package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

const _queueOf = "parent_id IS NULL AND status = 'queued'"

// StartQueues starts every queued item that has an owner, in queue order. A move into the
// queue starts its item by itself; this catches up after a restart.
func (s *Store) StartQueues(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id FROM work_item WHERE "+_queueOf+" AND owner_bot_id IS NOT NULL "+
			"ORDER BY queue_position IS NULL, queue_position, id")
	if err != nil {
		return fmt.Errorf("list queue: %w", err)
	}
	ids, err := scanAll(rows, func(r interface{ Scan(...any) error }) (int64, error) {
		var id int64
		return id, r.Scan(&id)
	})
	if err != nil {
		return fmt.Errorf("list queue: %w", err)
	}
	for _, id := range ids {
		if _, err := s.MoveWorkItem(ctx, id, _inProgress, ByQueue); err != nil && !errors.Is(err, ErrConflict) {
			return err
		}
	}
	return nil
}

// requeue keeps the queue positions of the queued items as 1, 2, 3 after w moved to to: an
// item that joins the queue goes last, one sent back from review goes first, and one that
// leaves the queue closes its gap.
func requeue(ctx context.Context, tx *sql.Tx, w WorkItem, to string) error {
	var err error
	switch {
	case to == _queued && w.Status == _review:
		if _, err = tx.ExecContext(ctx,
			"UPDATE work_item SET queue_position = queue_position + 1 WHERE "+_queueOf); err == nil {
			_, err = tx.ExecContext(ctx, "UPDATE work_item SET queue_position = 1 WHERE id = ?", w.ID)
		}
	case to == _queued:
		_, err = tx.ExecContext(ctx,
			"UPDATE work_item SET queue_position = (SELECT COALESCE(MAX(queue_position), 0) + 1 "+
				"FROM work_item WHERE "+_queueOf+") WHERE id = ?", w.ID)
	case w.Status == _queued && w.QueuePosition != nil:
		if _, err = tx.ExecContext(ctx,
			"UPDATE work_item SET queue_position = NULL WHERE id = ?", w.ID); err == nil {
			_, err = tx.ExecContext(ctx,
				"UPDATE work_item SET queue_position = queue_position - 1 WHERE "+_queueOf+
					" AND queue_position > ?", *w.QueuePosition)
		}
	}
	if err != nil {
		return fmt.Errorf("update queue positions: %w", err)
	}
	return nil
}

// MoveInQueue puts the queued item at position in the queue and shifts the items between. The
// item must be a top-level queued item, and position within 1..n.
func (s *Store) MoveInQueue(ctx context.Context, id, position int64) (WorkItem, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return WorkItem{}, fmt.Errorf("begin move in queue: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	w, err := txWorkItem(ctx, tx, id)
	if err != nil {
		return WorkItem{}, err
	}
	if w.ParentID != nil || w.Status != _queued || w.QueuePosition == nil {
		return WorkItem{}, fmt.Errorf("%w: work item %d is not in the queue", ErrInvalid, id)
	}
	var n int64
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM work_item WHERE "+_queueOf).Scan(&n); err != nil {
		return WorkItem{}, fmt.Errorf("count queue: %w", err)
	}
	if position < 1 || position > n {
		return WorkItem{}, fmt.Errorf("%w: position %d is outside the queue of %d", ErrInvalid, position, n)
	}
	from := *w.QueuePosition
	switch {
	case position < from:
		_, err = tx.ExecContext(ctx,
			"UPDATE work_item SET queue_position = queue_position + 1 WHERE "+_queueOf+
				" AND queue_position >= ? AND queue_position < ?", position, from)
	case position > from:
		_, err = tx.ExecContext(ctx,
			"UPDATE work_item SET queue_position = queue_position - 1 WHERE "+_queueOf+
				" AND queue_position > ? AND queue_position <= ?", from, position)
	}
	if err == nil {
		_, err = tx.ExecContext(ctx, "UPDATE work_item SET queue_position = ? WHERE id = ?", position, id)
	}
	if err != nil {
		return WorkItem{}, fmt.Errorf("update queue positions: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return WorkItem{}, fmt.Errorf("commit move in queue: %w", err)
	}
	return s.WorkItem(ctx, id)
}
