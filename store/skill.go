package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/bhanuprakaash/reef/skill"
)

// Skill is a package in Reef's skill library. Version is the current one; older versions stay
// readable through SkillFiles. Source, Ref and Dir say where an update re-reads it from;
// Revision is the commit of a Git import.
type Skill struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	SourceKind  string    `json:"sourceKind"`
	Source      string    `json:"source"`
	Ref         string    `json:"ref"`
	Dir         string    `json:"dir"`
	Revision    string    `json:"revision"`
	Version     int       `json:"version"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

const _skillCols = "id, name, description, source_kind, source, ref, dir, revision, version, created_at, updated_at"

// ImportSkill saves p as version 1 of a new library skill, or fails with ErrConflict when the
// name is taken. The row and its files are saved together or not at all.
func (s *Store) ImportSkill(ctx context.Context, kind, source, ref, dir string, p skill.Package) (Skill, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Skill{}, fmt.Errorf("begin import skill: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	id, err := importSkill(ctx, tx, kind, source, ref, dir, p)
	if err != nil {
		return Skill{}, err
	}
	if err := tx.Commit(); err != nil {
		return Skill{}, fmt.Errorf("commit import skill: %w", err)
	}
	return s.Skill(ctx, id)
}

func importSkill(ctx context.Context, tx *sql.Tx, kind, source, ref, dir string, p skill.Package) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := tx.ExecContext(ctx,
		"INSERT INTO skill (name, description, source_kind, source, ref, dir, revision, version, created_at, updated_at) "+
			"VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?, ?)",
		p.Name, p.Description, kind, source, ref, dir, p.Revision, now, now)
	if isUnique(err) {
		return 0, fmt.Errorf("%w: a skill named %q is already in the library; update it instead", ErrConflict, p.Name)
	}
	if err != nil {
		return 0, fmt.Errorf("insert skill: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("insert skill: %w", err)
	}
	return id, insertSkillFiles(ctx, tx, id, 1, p.Files)
}

// UpdateSkill saves p as the next version of skill id, keeping its older versions. A package
// that renames the skill is ErrInvalid.
func (s *Store) UpdateSkill(ctx context.Context, id int64, p skill.Package) (Skill, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Skill{}, fmt.Errorf("begin update skill: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := updateSkill(ctx, tx, id, p); err != nil {
		return Skill{}, err
	}
	if err := tx.Commit(); err != nil {
		return Skill{}, fmt.Errorf("commit update skill: %w", err)
	}
	return s.Skill(ctx, id)
}

func updateSkill(ctx context.Context, tx *sql.Tx, id int64, p skill.Package) error {
	var name string
	var version int
	err := tx.QueryRowContext(ctx, "SELECT name, version FROM skill WHERE id = ?", id).Scan(&name, &version)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("skill %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return fmt.Errorf("read skill: %w", err)
	}
	if p.Name != name {
		return fmt.Errorf("%w: the package is now named %q, not %q", ErrInvalid, p.Name, name)
	}
	version++
	if _, err := tx.ExecContext(ctx,
		"UPDATE skill SET description = ?, revision = ?, version = ?, updated_at = ? WHERE id = ?",
		p.Description, p.Revision, version, time.Now().UTC().Format(time.RFC3339Nano), id); err != nil {
		return fmt.Errorf("update skill: %w", err)
	}
	return insertSkillFiles(ctx, tx, id, version, p.Files)
}

func insertSkillFiles(ctx context.Context, tx *sql.Tx, id int64, version int, files []skill.File) error {
	for _, f := range files {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO skill_file (skill_id, version, path, body, executable) VALUES (?, ?, ?, ?, ?)",
			id, version, f.Path, f.Body, f.Executable); err != nil {
			return fmt.Errorf("insert skill file %s: %w", f.Path, err)
		}
	}
	return nil
}

// Skill returns the library skill with this id.
func (s *Store) Skill(ctx context.Context, id int64) (Skill, error) {
	sk, err := scanSkill(s.db.QueryRowContext(ctx, "SELECT "+_skillCols+" FROM skill WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return Skill{}, fmt.Errorf("skill %d: %w", id, ErrNotFound)
	}
	return sk, err
}

// Skills returns the library, ordered by name.
func (s *Store) Skills(ctx context.Context) ([]Skill, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+_skillCols+" FROM skill ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("list skills: %w", err)
	}
	return scanAll(rows, scanSkill)
}

// SkillFiles returns the files of one version of skill id, ordered by path.
func (s *Store) SkillFiles(ctx context.Context, id int64, version int) ([]skill.File, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT path, body, executable FROM skill_file WHERE skill_id = ? AND version = ? ORDER BY path", id, version)
	if err != nil {
		return nil, fmt.Errorf("list skill files: %w", err)
	}
	files, err := scanAll(rows, func(r interface{ Scan(...any) error }) (skill.File, error) {
		var f skill.File
		return f, r.Scan(&f.Path, &f.Body, &f.Executable)
	})
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("skill %d version %d: %w", id, version, ErrNotFound)
	}
	return files, nil
}

func scanSkill(r interface{ Scan(...any) error }) (Skill, error) {
	var sk Skill
	var created, updated string
	if err := r.Scan(&sk.ID, &sk.Name, &sk.Description, &sk.SourceKind, &sk.Source, &sk.Ref, &sk.Dir,
		&sk.Revision, &sk.Version, &created, &updated); err != nil {
		return Skill{}, err
	}
	var err error
	if sk.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return Skill{}, fmt.Errorf("skill %s: bad created_at: %w", sk.Name, err)
	}
	if sk.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated); err != nil {
		return Skill{}, fmt.Errorf("skill %s: bad updated_at: %w", sk.Name, err)
	}
	return sk, nil
}
