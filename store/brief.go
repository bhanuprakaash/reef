package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Brief is one version of a feature's brief.
type Brief struct {
	ID        int64     `json:"id"`
	FeatureID int64     `json:"featureId"`
	Body      string    `json:"body"`
	Author    string    `json:"author"`
	CreatedAt time.Time `json:"createdAt"`
}

// PutBrief appends b as a new version of the feature's brief.
func (s *Store) PutBrief(ctx context.Context, b Brief) (Brief, error) {
	if b.Body == "" {
		return Brief{}, fmt.Errorf("%w: brief body is required", ErrInvalid)
	}
	if b.Author == "" {
		return Brief{}, fmt.Errorf("%w: brief author is required", ErrInvalid)
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO brief (feature_id, body, author, created_at) VALUES (?, ?, ?, ?)",
		b.FeatureID, b.Body, b.Author, now.Format(time.RFC3339Nano))
	if err != nil {
		return Brief{}, s.writeErr(ctx, "insert brief", err, ErrNotFound,
			parent{table: "feature", id: &b.FeatureID})
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Brief{}, fmt.Errorf("insert brief: %w", err)
	}
	b.ID, b.CreatedAt = id, now
	return b, nil
}

// Brief returns the latest version of the feature's brief.
func (s *Store) Brief(ctx context.Context, featureID int64) (Brief, error) {
	row := s.db.QueryRowContext(ctx,
		"SELECT id, feature_id, body, author, created_at FROM brief "+
			"WHERE feature_id = ? ORDER BY id DESC LIMIT 1",
		featureID)
	b, err := scanBrief(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Brief{}, fmt.Errorf("brief of feature %d: %w", featureID, ErrNotFound)
	}
	return b, err
}

// BriefVersions returns every version of the feature's brief, oldest first.
func (s *Store) BriefVersions(ctx context.Context, featureID int64) ([]Brief, error) {
	if err := s.requireFeature(ctx, featureID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, feature_id, body, author, created_at FROM brief "+
			"WHERE feature_id = ? ORDER BY id",
		featureID)
	if err != nil {
		return nil, fmt.Errorf("list brief versions: %w", err)
	}
	return scanAll(rows, scanBrief)
}

func scanBrief(r interface{ Scan(...any) error }) (Brief, error) {
	var b Brief
	var created string
	if err := r.Scan(&b.ID, &b.FeatureID, &b.Body, &b.Author, &created); err != nil {
		return Brief{}, err
	}
	t, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return Brief{}, fmt.Errorf("brief %d: bad created_at: %w", b.ID, err)
	}
	b.CreatedAt = t
	return b, nil
}
