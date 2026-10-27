package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

const _queueOf = "team_id = ? AND parent_id IS NULL AND status = 'queued'"

// holdsTeam reports whether a top-level item in status keeps its Team busy: Needs you does,
// because the item still holds its worktree.
func holdsTeam(status string) bool { return status == _inProgress || status == _needsYou }

// StartQueues starts the first queued item of each Team that is free. A move starts the next
// item by itself; this catches up after a restart.
func (s *Store) StartQueues(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx,
		"SELECT DISTINCT team_id FROM work_item WHERE parent_id IS NULL AND status = ? "+
			"AND team_id IS NOT NULL", _queued)
	if err != nil {
		return fmt.Errorf("list queues: %w", err)
	}
	teams, err := scanAll(rows, func(r interface{ Scan(...any) error }) (int64, error) {
		var id int64
		return id, r.Scan(&id)
	})
	if err != nil {
		return fmt.Errorf("list queues: %w", err)
	}
	for _, id := range teams {
		if err := s.startNext(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

// startNext moves the first queued item of the Team to in progress. A Team that is busy keeps
// it queued.
func (s *Store) startNext(ctx context.Context, teamID int64) error {
	var id int64
	err := s.db.QueryRowContext(ctx,
		"SELECT id FROM work_item WHERE "+_queueOf+
			" ORDER BY queue_position IS NULL, queue_position, id LIMIT 1", teamID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read queue of team %d: %w", teamID, err)
	}
	if _, err := s.MoveWorkItem(ctx, id, _inProgress, ByQueue); err != nil && !errors.Is(err, ErrConflict) {
		return err
	}
	return nil
}

// startAfter starts the next queued item of w's Team when w, now in status to (or deleted), has
// freed the Team or joined the queue. It is the one place a Team's turn is handed on.
func (s *Store) startAfter(ctx context.Context, w WorkItem, to string) error {
	if w.ParentID != nil || w.TeamID == nil || to != _queued && !(holdsTeam(w.Status) && !holdsTeam(to)) {
		return nil
	}
	return s.startNext(ctx, *w.TeamID)
}

// requeue keeps the queue positions of the Team of item w as 1, 2, 3 after w moved to to: an
// item that joins the queue goes last, one sent back from review goes first, and one that
// leaves the queue closes its gap.
func requeue(ctx context.Context, tx *sql.Tx, w WorkItem, to string) error {
	var err error
	switch {
	case to == _queued && w.Status == _review:
		if _, err = tx.ExecContext(ctx,
			"UPDATE work_item SET queue_position = queue_position + 1 WHERE "+_queueOf,
			*w.TeamID); err == nil {
			_, err = tx.ExecContext(ctx, "UPDATE work_item SET queue_position = 1 WHERE id = ?", w.ID)
		}
	case to == _queued:
		_, err = tx.ExecContext(ctx,
			"UPDATE work_item SET queue_position = (SELECT COALESCE(MAX(queue_position), 0) + 1 "+
				"FROM work_item WHERE "+_queueOf+") WHERE id = ?", *w.TeamID, w.ID)
	case w.Status == _queued && w.QueuePosition != nil:
		if _, err = tx.ExecContext(ctx,
			"UPDATE work_item SET queue_position = NULL WHERE id = ?", w.ID); err == nil {
			_, err = tx.ExecContext(ctx,
				"UPDATE work_item SET queue_position = queue_position - 1 WHERE "+_queueOf+
					" AND queue_position > ?", *w.TeamID, *w.QueuePosition)
		}
	}
	if err != nil {
		return fmt.Errorf("update queue positions: %w", err)
	}
	return nil
}

// MoveInQueue puts the queued item at position in the queue of its Team and shifts the items
// between. The item must be a top-level queued item with a Team, and position within 1..n.
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
	if w.ParentID != nil || w.Status != _queued || w.TeamID == nil || w.QueuePosition == nil {
		return WorkItem{}, fmt.Errorf("%w: work item %d is not in a Team queue", ErrInvalid, id)
	}
	var n int64
	if err := tx.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM work_item WHERE "+_queueOf, *w.TeamID).Scan(&n); err != nil {
		return WorkItem{}, fmt.Errorf("count queue of team %d: %w", *w.TeamID, err)
	}
	if position < 1 || position > n {
		return WorkItem{}, fmt.Errorf("%w: position %d is outside the queue of %d", ErrInvalid, position, n)
	}
	from := *w.QueuePosition
	switch {
	case position < from:
		_, err = tx.ExecContext(ctx,
			"UPDATE work_item SET queue_position = queue_position + 1 WHERE "+_queueOf+
				" AND queue_position >= ? AND queue_position < ?", *w.TeamID, position, from)
	case position > from:
		_, err = tx.ExecContext(ctx,
			"UPDATE work_item SET queue_position = queue_position - 1 WHERE "+_queueOf+
				" AND queue_position > ? AND queue_position <= ?", *w.TeamID, from, position)
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
