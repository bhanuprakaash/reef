package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	_defaultStage = "ideation"
	_featureCols  = "id, project_id, slug, name, goal, stage, created_at, closed_at"
)

var _slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// Feature is a unit of work in a project. Its slug names its branch and folder.
type Feature struct {
	ID        int64      `json:"id"`
	ProjectID int64      `json:"projectId"`
	Slug      string     `json:"slug"`
	Name      string     `json:"name"`
	Goal      string     `json:"goal"`
	Stage     string     `json:"stage"`
	CreatedAt time.Time  `json:"createdAt"`
	ClosedAt  *time.Time `json:"closedAt"`
}

// CreateFeature adds f. The slug must be unique within the project.
func (s *Store) CreateFeature(ctx context.Context, f Feature) (Feature, error) {
	if err := f.validateSlug(); err != nil {
		return Feature{}, err
	}
	if err := f.validate(); err != nil {
		return Feature{}, err
	}
	f, err := insertFeature(ctx, s.db, f)
	if err != nil && !errors.Is(err, ErrConflict) {
		return Feature{}, s.writeErr(ctx, "write feature", err, ErrInvalid,
			parent{table: "project", id: &f.ProjectID})
	}
	return f, err
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

var (
	_ execer = (*sql.DB)(nil)
	_ execer = (*sql.Tx)(nil)
)

func insertFeature(ctx context.Context, x execer, f Feature) (Feature, error) {
	now := time.Now().UTC()
	res, err := x.ExecContext(ctx,
		"INSERT INTO feature (project_id, slug, name, goal, stage, created_at) "+
			"VALUES (?, ?, ?, ?, ?, ?)",
		f.ProjectID, f.Slug, f.Name, f.Goal, f.Stage, now.Format(time.RFC3339Nano))
	if isUnique(err) {
		return f, fmt.Errorf("feature slug %q is taken in its project: %w", f.Slug, ErrConflict)
	}
	if err != nil {
		return f, fmt.Errorf("insert feature: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return f, fmt.Errorf("insert feature: %w", err)
	}
	f.ID, f.CreatedAt = id, now
	return f, nil
}

// Feature returns the feature with the id, or ErrNotFound.
func (s *Store) Feature(ctx context.Context, id int64) (Feature, error) {
	row := s.db.QueryRowContext(ctx, "SELECT "+_featureCols+" FROM feature WHERE id = ?", id)
	f, err := scanFeature(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Feature{}, fmt.Errorf("feature %d: %w", id, ErrNotFound)
	}
	return f, err
}

// FeatureByRef returns the feature that ref names: a feature id when ref is all
// digits, else a slug. A slug used in more than one project is ErrInvalid.
func (s *Store) FeatureByRef(ctx context.Context, ref string) (Feature, error) {
	if id, err := strconv.ParseUint(ref, 10, 63); err == nil {
		return s.Feature(ctx, int64(id))
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT "+_featureCols+" FROM feature WHERE slug = ? ORDER BY id", ref)
	if err != nil {
		return Feature{}, fmt.Errorf("find feature: %w", err)
	}
	found, err := scanAll(rows, scanFeature)
	if err != nil {
		return Feature{}, fmt.Errorf("find feature: %w", err)
	}
	switch len(found) {
	case 0:
		return Feature{}, fmt.Errorf("feature %q: %w", ref, ErrNotFound)
	case 1:
		return found[0], nil
	}
	projects := make([]string, 0, len(found))
	for _, f := range found {
		projects = append(projects, strconv.FormatInt(f.ProjectID, 10))
	}
	return Feature{}, fmt.Errorf("%w: feature slug %q is in projects %s; use the feature id",
		ErrInvalid, ref, strings.Join(projects, ", "))
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
	return scanAll(rows, scanFeature)
}

// UpdateFeature sets the name, goal and stage of f.ID. A feature never changes
// project or slug: the slug names its branch.
func (s *Store) UpdateFeature(ctx context.Context, f Feature) (Feature, error) {
	if err := f.validate(); err != nil {
		return Feature{}, err
	}
	res, err := s.db.ExecContext(ctx,
		"UPDATE feature SET name = ?, goal = ?, stage = ? WHERE id = ?",
		f.Name, f.Goal, f.Stage, f.ID)
	if err != nil {
		return Feature{}, fmt.Errorf("update feature: %w", err)
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

// CloseFeature sets closed_at on the feature and on each of its channels still open, in one
// transaction.
func (s *Store) CloseFeature(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin close feature: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	at := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := tx.ExecContext(ctx, "UPDATE feature SET closed_at = ? WHERE id = ?", at, id)
	if err != nil {
		return fmt.Errorf("close feature: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("close feature: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("feature %d: %w", id, ErrNotFound)
	}
	if _, err := tx.ExecContext(ctx,
		"UPDATE channel SET closed_at = ? WHERE feature_id = ? AND closed_at IS NULL", at, id); err != nil {
		return fmt.Errorf("close channels: %w", err)
	}
	return tx.Commit()
}

// ReopenFeature clears the feature's closed_at; its channels stay as they are.
func (s *Store) ReopenFeature(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, "UPDATE feature SET closed_at = NULL WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("reopen feature: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("reopen feature: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("feature %d: %w", id, ErrNotFound)
	}
	return nil
}

// DeleteFeature removes the feature and, by cascade, its briefs, notes,
// decisions and channels.
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

func (f *Feature) validateSlug() error {
	if f.Slug == "" {
		return fmt.Errorf("%w: feature slug is required", ErrInvalid)
	}
	if !_slugPattern.MatchString(f.Slug) {
		return fmt.Errorf("%w: feature slug %q must match %s", ErrInvalid, f.Slug, _slugPattern)
	}
	if strings.Trim(f.Slug, "0123456789") == "" {
		return fmt.Errorf("%w: feature slug %q must contain a letter or -", ErrInvalid, f.Slug)
	}
	return nil
}

// validate also fills the default stage.
func (f *Feature) validate() error {
	if f.Name == "" {
		return fmt.Errorf("%w: feature name is required", ErrInvalid)
	}
	if f.Stage == "" {
		f.Stage = _defaultStage
	}
	return nil
}

func scanFeature(r interface{ Scan(...any) error }) (Feature, error) {
	var f Feature
	var created string
	var closed sql.NullString
	if err := r.Scan(
		&f.ID, &f.ProjectID, &f.Slug, &f.Name, &f.Goal, &f.Stage, &created, &closed,
	); err != nil {
		return Feature{}, err
	}
	t, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return Feature{}, fmt.Errorf("feature %d: bad created_at: %w", f.ID, err)
	}
	f.CreatedAt = t
	if f.ClosedAt, err = parseClosed(closed); err != nil {
		return Feature{}, fmt.Errorf("feature %d: %w", f.ID, err)
	}
	return f, nil
}
