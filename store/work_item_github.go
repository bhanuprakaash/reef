package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// WorkItemGitHub is the last GitHub PR state Reef posted about for a work item, so a restart
// of reef serve does not post it again.
type WorkItemGitHub struct {
	WorkItemID int64
	PRNumber   int64
	PRState    string
	PRURL      string
}

// WorkItemGitHub returns the item's last seen PR state, and false when it has none yet.
func (s *Store) WorkItemGitHub(ctx context.Context, workItemID int64) (WorkItemGitHub, bool, error) {
	row := s.db.QueryRowContext(ctx,
		"SELECT work_item_id, pr_number, pr_state, pr_url FROM work_item_github WHERE work_item_id = ?",
		workItemID)
	var g WorkItemGitHub
	err := row.Scan(&g.WorkItemID, &g.PRNumber, &g.PRState, &g.PRURL)
	if errors.Is(err, sql.ErrNoRows) {
		return WorkItemGitHub{}, false, nil
	}
	if err != nil {
		return WorkItemGitHub{}, false, fmt.Errorf("read work item github: %w", err)
	}
	return g, true, nil
}

// SetWorkItemGitHub stores g as the item's last seen PR state.
func (s *Store) SetWorkItemGitHub(ctx context.Context, g WorkItemGitHub) error {
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO work_item_github (work_item_id, pr_number, pr_state, pr_url) VALUES (?, ?, ?, ?) "+
			"ON CONFLICT(work_item_id) DO UPDATE SET pr_number = excluded.pr_number, "+
			"pr_state = excluded.pr_state, pr_url = excluded.pr_url",
		g.WorkItemID, g.PRNumber, g.PRState, g.PRURL)
	if err != nil {
		return fmt.Errorf("write work item github: %w", err)
	}
	return nil
}

// GitHubCheckNoted reports whether the check run identified by name and key was already
// posted about. A check of the same name with a different key is a different run, its own
// row, noted on its own.
func (s *Store) GitHubCheckNoted(ctx context.Context, workItemID int64, name, key string) (bool, error) {
	var noted bool
	err := s.db.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM work_item_github_check WHERE work_item_id = ? AND name = ? AND run_key = ?)",
		workItemID, name, key).Scan(&noted)
	if err != nil {
		return false, fmt.Errorf("read github check: %w", err)
	}
	return noted, nil
}

// RecordGitHubCheck records that the check run identified by name and key was posted about.
// Call it only once the note was actually posted, so a note that could not be posted (no
// Space yet, or a failed write) is retried on a later poll instead of recorded as done.
func (s *Store) RecordGitHubCheck(ctx context.Context, workItemID int64, name, key string) error {
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO work_item_github_check (work_item_id, name, run_key) VALUES (?, ?, ?) "+
			"ON CONFLICT(work_item_id, name, run_key) DO NOTHING",
		workItemID, name, key)
	if err != nil {
		return fmt.Errorf("write github check: %w", err)
	}
	return nil
}

// WokenGitHubCheck reports whether the check run identified by name and key already woke the
// developer.
func (s *Store) WokenGitHubCheck(ctx context.Context, workItemID int64, name, key string) (bool, error) {
	var woken bool
	err := s.db.QueryRowContext(ctx,
		"SELECT woken FROM work_item_github_check WHERE work_item_id = ? AND name = ? AND run_key = ?",
		workItemID, name, key).Scan(&woken)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read github check: %w", err)
	}
	return woken, nil
}

// MarkGitHubCheckWoken records that the check run identified by name and key woke the
// developer, so a later poll or a restart does not wake it again.
func (s *Store) MarkGitHubCheckWoken(ctx context.Context, workItemID int64, name, key string) error {
	_, err := s.db.ExecContext(ctx,
		"UPDATE work_item_github_check SET woken = 1 WHERE work_item_id = ? AND name = ? AND run_key = ?",
		workItemID, name, key)
	if err != nil {
		return fmt.Errorf("write github check: %w", err)
	}
	return nil
}
