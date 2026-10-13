package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type Project struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	CreatedAt time.Time `json:"createdAt"`
}

func (s *Store) CreateProject(ctx context.Context, p Project) (Project, error) {
	if err := p.validate(); err != nil {
		return Project{}, err
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO project (name, path, created_at) VALUES (?, ?, ?)",
		p.Name, p.Path, now.Format(time.RFC3339Nano))
	if err != nil {
		return Project{}, fmt.Errorf("insert project: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Project{}, fmt.Errorf("insert project: %w", err)
	}
	p.ID, p.CreatedAt = id, now
	return p, nil
}

func (s *Store) Project(ctx context.Context, id int64) (Project, error) {
	row := s.db.QueryRowContext(ctx,
		"SELECT id, name, path, created_at FROM project WHERE id = ?", id)
	p, err := scanProject(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Project{}, fmt.Errorf("project %d: %w", id, ErrNotFound)
	}
	return p, err
}

func (s *Store) Projects(ctx context.Context) ([]Project, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, name, path, created_at FROM project ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()
	var ps []Project
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		ps = append(ps, p)
	}
	return ps, rows.Err()
}

// UpdateProject sets the name and path of p.ID.
func (s *Store) UpdateProject(ctx context.Context, p Project) (Project, error) {
	if err := p.validate(); err != nil {
		return Project{}, err
	}
	res, err := s.db.ExecContext(ctx,
		"UPDATE project SET name = ?, path = ? WHERE id = ?", p.Name, p.Path, p.ID)
	if err != nil {
		return Project{}, fmt.Errorf("update project: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return Project{}, fmt.Errorf("update project: %w", err)
	}
	if n == 0 {
		return Project{}, fmt.Errorf("project %d: %w", p.ID, ErrNotFound)
	}
	return s.Project(ctx, p.ID)
}

func (s *Store) DeleteProject(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM project WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("project %d: %w", id, ErrNotFound)
	}
	return nil
}

func (p Project) validate() error {
	if p.Name == "" {
		return fmt.Errorf("%w: project name is required", ErrInvalid)
	}
	if p.Path == "" {
		return fmt.Errorf("%w: project path is required", ErrInvalid)
	}
	return nil
}

func scanProject(r interface{ Scan(...any) error }) (Project, error) {
	var p Project
	var created string
	if err := r.Scan(&p.ID, &p.Name, &p.Path, &created); err != nil {
		return Project{}, err
	}
	t, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return Project{}, fmt.Errorf("project %d: bad created_at: %w", p.ID, err)
	}
	p.CreatedAt = t
	return p, nil
}
