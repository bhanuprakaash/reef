package store

import (
	"context"
	"fmt"
	"time"
)

// ClaimMemoryReview reports whether the caller took the review of Bot id's personal memory at
// now: true only when the last review is every or more ago and no review has been claimed
// within stale, so of several callers one wins. A Bot never reviewed starts its interval now and
// is not due.
func (s *Store) ClaimMemoryReview(
	ctx context.Context, id int64, now time.Time, every, stale time.Duration,
) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		"UPDATE bot SET memory_reviewed_at = ? WHERE id = ? AND memory_reviewed_at IS NULL",
		now.Unix(), id)
	if err != nil {
		return false, fmt.Errorf("start memory review interval: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil || n > 0 {
		return false, err
	}
	res, err = s.db.ExecContext(ctx,
		"UPDATE bot SET memory_review_started_at = ? WHERE id = ? AND memory_reviewed_at <= ? "+
			"AND (memory_review_started_at IS NULL OR memory_review_started_at < ?)",
		now.Unix(), id, now.Add(-every).Unix(), now.Add(-stale).Unix())
	if err != nil {
		return false, fmt.Errorf("claim memory review: %w", err)
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// FinishMemoryReview ends the claim of ClaimMemoryReview. A review that succeeded moves the
// last review to now; one that failed leaves it, so the Bot's next activity tries again.
func (s *Store) FinishMemoryReview(ctx context.Context, id int64, now time.Time, ok bool) error {
	reviewed := "memory_reviewed_at"
	args := []any{id}
	if ok {
		reviewed = "?"
		args = []any{now.Unix(), id}
	}
	_, err := s.db.ExecContext(ctx,
		"UPDATE bot SET memory_reviewed_at = "+reviewed+", memory_review_started_at = NULL WHERE id = ?",
		args...)
	if err != nil {
		return fmt.Errorf("finish memory review: %w", err)
	}
	return nil
}
