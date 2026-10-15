package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// FeatureGitHub is the last GitHub PR state Reef posted about for a feature, so a restart
// of reef serve does not post it again.
type FeatureGitHub struct {
	FeatureID int64
	PRNumber  int64
	PRState   string
	PRURL     string
}

// FeatureGitHub returns the feature's last seen PR state, and false when it has none yet.
func (s *Store) FeatureGitHub(ctx context.Context, featureID int64) (FeatureGitHub, bool, error) {
	row := s.db.QueryRowContext(ctx,
		"SELECT feature_id, pr_number, pr_state, pr_url FROM feature_github WHERE feature_id = ?",
		featureID)
	var g FeatureGitHub
	err := row.Scan(&g.FeatureID, &g.PRNumber, &g.PRState, &g.PRURL)
	if errors.Is(err, sql.ErrNoRows) {
		return FeatureGitHub{}, false, nil
	}
	if err != nil {
		return FeatureGitHub{}, false, fmt.Errorf("read feature github: %w", err)
	}
	return g, true, nil
}

// SetFeatureGitHub stores g as the feature's last seen PR state.
func (s *Store) SetFeatureGitHub(ctx context.Context, g FeatureGitHub) error {
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO feature_github (feature_id, pr_number, pr_state, pr_url) VALUES (?, ?, ?, ?) "+
			"ON CONFLICT(feature_id) DO UPDATE SET pr_number = excluded.pr_number, "+
			"pr_state = excluded.pr_state, pr_url = excluded.pr_url",
		g.FeatureID, g.PRNumber, g.PRState, g.PRURL)
	if err != nil {
		return fmt.Errorf("write feature github: %w", err)
	}
	return nil
}

// GitHubCheckNoted reports whether the check run identified by name and key was already
// posted about. A check of the same name with a different key is a different run, its own
// row, noted on its own.
func (s *Store) GitHubCheckNoted(ctx context.Context, featureID int64, name, key string) (bool, error) {
	var noted bool
	err := s.db.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM feature_github_check WHERE feature_id = ? AND name = ? AND run_key = ?)",
		featureID, name, key).Scan(&noted)
	if err != nil {
		return false, fmt.Errorf("read github check: %w", err)
	}
	return noted, nil
}

// RecordGitHubCheck records that the check run identified by name and key was posted about.
// Call it only once the note was actually posted, so a note that could not be posted (no
// channel yet, or a failed write) is retried on a later poll instead of recorded as done.
func (s *Store) RecordGitHubCheck(ctx context.Context, featureID int64, name, key string) error {
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO feature_github_check (feature_id, name, run_key) VALUES (?, ?, ?) "+
			"ON CONFLICT(feature_id, name, run_key) DO NOTHING",
		featureID, name, key)
	if err != nil {
		return fmt.Errorf("write github check: %w", err)
	}
	return nil
}

// WokenGitHubCheck reports whether the check run identified by name and key already woke the
// developer.
func (s *Store) WokenGitHubCheck(ctx context.Context, featureID int64, name, key string) (bool, error) {
	var woken bool
	err := s.db.QueryRowContext(ctx,
		"SELECT woken FROM feature_github_check WHERE feature_id = ? AND name = ? AND run_key = ?",
		featureID, name, key).Scan(&woken)
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
func (s *Store) MarkGitHubCheckWoken(ctx context.Context, featureID int64, name, key string) error {
	_, err := s.db.ExecContext(ctx,
		"UPDATE feature_github_check SET woken = 1 WHERE feature_id = ? AND name = ? AND run_key = ?",
		featureID, name, key)
	if err != nil {
		return fmt.Errorf("write github check: %w", err)
	}
	return nil
}
