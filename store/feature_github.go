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

// NotifiedGitHubCheck reports whether the check's run, identified by key, already posted a
// note. A check of the same name with a different key is a new run, not yet notified.
func (s *Store) NotifiedGitHubCheck(ctx context.Context, featureID int64, name, key string) (bool, error) {
	var notified bool
	var runKey string
	err := s.db.QueryRowContext(ctx,
		"SELECT notified, run_key FROM feature_github_check WHERE feature_id = ? AND name = ?",
		featureID, name).Scan(&notified, &runKey)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read github check: %w", err)
	}
	return notified && runKey == key, nil
}

// MarkGitHubCheckNotified records that the check run identified by key was posted, so a
// later poll or a restart does not post it again; it also clears the run's wake, since a
// new run is not woken yet.
func (s *Store) MarkGitHubCheckNotified(ctx context.Context, featureID int64, name, key string) error {
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO feature_github_check (feature_id, name, notified, run_key, woken) VALUES (?, ?, 1, ?, 0) "+
			"ON CONFLICT(feature_id, name) DO UPDATE SET notified = 1, run_key = excluded.run_key, woken = 0",
		featureID, name, key)
	if err != nil {
		return fmt.Errorf("write github check: %w", err)
	}
	return nil
}

// WokenGitHubCheck reports whether the check's run, identified by key, already woke the
// developer.
func (s *Store) WokenGitHubCheck(ctx context.Context, featureID int64, name, key string) (bool, error) {
	var woken bool
	var runKey string
	err := s.db.QueryRowContext(ctx,
		"SELECT woken, run_key FROM feature_github_check WHERE feature_id = ? AND name = ?",
		featureID, name).Scan(&woken, &runKey)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read github check: %w", err)
	}
	return woken && runKey == key, nil
}

// MarkGitHubCheckWoken records that the check run identified by key woke the developer, so a
// later poll or a restart does not wake it again.
func (s *Store) MarkGitHubCheckWoken(ctx context.Context, featureID int64, name, key string) error {
	_, err := s.db.ExecContext(ctx,
		"UPDATE feature_github_check SET woken = 1 WHERE feature_id = ? AND name = ? AND run_key = ?",
		featureID, name, key)
	if err != nil {
		return fmt.Errorf("write github check: %w", err)
	}
	return nil
}
