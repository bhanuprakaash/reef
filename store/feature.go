package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const (
	_defaultStage = "ideation"
	_featureCols  = "id, project_id, slug, name, goal, stage, created_at"
)

type Feature struct {
	ID        int64     `json:"id"`
	ProjectID int64     `json:"projectId"`
	Slug      string    `json:"slug"`
	Name      string    `json:"name"`
	Goal      string    `json:"goal"`
	Stage     string    `json:"stage"`
	CreatedAt time.Time `json:"createdAt"`
}

func (s *Store) CreateFeature(ctx context.Context, f Feature) (Feature, error) {
	if err := f.validate(); err != nil {
		return Feature{}, err
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO feature (project_id, slug, name, goal, stage, created_at) VALUES (?, ?, ?, ?, ?, ?)",
		f.ProjectID, f.Slug, f.Name, f.Goal, f.Stage, now.Format(time.RFC3339Nano))
	if err != nil {
		return Feature{}, f.writeErr(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Feature{}, fmt.Errorf("insert feature: %w", err)
	}
	f.ID, f.CreatedAt = id, now
	return f, nil
}

func (s *Store) Feature(ctx context.Context, id int64) (Feature, error) {
	row := s.db.QueryRowContext(ctx, "SELECT "+_featureCols+" FROM feature WHERE id = ?", id)
	f, err := scanFeature(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Feature{}, fmt.Errorf("feature %d: %w", id, ErrNotFound)
	}
	return f, err
}

// Features lists features by id, only those of projectID when it is not nil.
func (s *Store) Features(ctx context.Context, projectID *int64) ([]Feature, error) {
	q, args := "SELECT "+_featureCols+" FROM feature", []any{}
	if projectID != nil {
		q, args = q+" WHERE project_id = ?", append(args, *projectID)
	}
	rows, err := s.db.QueryContext(ctx, q+" ORDER BY id", args...)
	if err != nil {
		return nil, fmt.Errorf("list features: %w", err)
	}
	defer rows.Close()
	var fs []Feature
	for rows.Next() {
		f, err := scanFeature(rows)
		if err != nil {
			return nil, err
		}
		fs = append(fs, f)
	}
	return fs, rows.Err()
}

// UpdateFeature sets the slug, name, goal and stage of f.ID. A feature never
// changes project.
func (s *Store) UpdateFeature(ctx context.Context, f Feature) (Feature, error) {
	if err := f.validate(); err != nil {
		return Feature{}, err
	}
	res, err := s.db.ExecContext(ctx,
		"UPDATE feature SET slug = ?, name = ?, goal = ?, stage = ? WHERE id = ?",
		f.Slug, f.Name, f.Goal, f.Stage, f.ID)
	if err != nil {
		return Feature{}, f.writeErr(err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return Feature{}, fmt.Errorf("update feature: %w", err)
	}
	if n == 0 {
		return Feature{}, fmt.Errorf("feature %d: %w", f.ID, ErrNotFound)
	}
	return s.Feature(ctx, f.ID)
}

func (s *Store) DeleteFeature(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM feature WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete feature: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete feature: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("feature %d: %w", id, ErrNotFound)
	}
	return nil
}

// validate also fills the default stage.
func (f *Feature) validate() error {
	if f.Slug == "" {
		return fmt.Errorf("%w: feature slug is required", ErrInvalid)
	}
	if f.Name == "" {
		return fmt.Errorf("%w: feature name is required", ErrInvalid)
	}
	if f.Stage == "" {
		f.Stage = _defaultStage
	}
	return nil
}

func (f Feature) writeErr(err error) error {
	if isForeignKey(err) {
		return fmt.Errorf("%w: unknown project %d", ErrInvalid, f.ProjectID)
	}
	if isUnique(err) {
		return fmt.Errorf("feature slug %q is taken in its project: %w", f.Slug, ErrConflict)
	}
	return fmt.Errorf("write feature: %w", err)
}

func scanFeature(r interface{ Scan(...any) error }) (Feature, error) {
	var f Feature
	var created string
	if err := r.Scan(&f.ID, &f.ProjectID, &f.Slug, &f.Name, &f.Goal, &f.Stage, &created); err != nil {
		return Feature{}, err
	}
	t, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return Feature{}, fmt.Errorf("feature %d: bad created_at: %w", f.ID, err)
	}
	f.CreatedAt = t
	return f, nil
}
