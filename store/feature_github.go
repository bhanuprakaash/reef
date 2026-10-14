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

// NotifiedGitHubCheck reports whether a finished check of name on the feature's PR already
// posted a note.
func (s *Store) NotifiedGitHubCheck(ctx context.Context, featureID int64, name string) (bool, error) {
	var notified bool
	err := s.db.QueryRowContext(ctx,
		"SELECT notified FROM feature_github_check WHERE feature_id = ? AND name = ?",
		featureID, name).Scan(&notified)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read github check: %w", err)
	}
	return notified, nil
}

// MarkGitHubCheckNotified records that the check's finish was posted, so a later poll or a
// restart does not post it again.
func (s *Store) MarkGitHubCheckNotified(ctx context.Context, featureID int64, name string) error {
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO feature_github_check (feature_id, name, notified) VALUES (?, ?, 1) "+
			"ON CONFLICT(feature_id, name) DO UPDATE SET notified = 1",
		featureID, name)
	if err != nil {
		return fmt.Errorf("write github check: %w", err)
	}
	return nil
}
