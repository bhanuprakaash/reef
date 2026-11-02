package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// _tileColours are the colours a project tile can take, the palette of web/src/utils/project-tile.ts.
var _tileColours = []string{"#B84E22", "#1F7A6F", "#6A59E0", "#B03E73", "#2F72C0", "#5F7A1E", "#8F6A12", "#5E6B78"}

// _projectIcons are the icon names a project tile can take, the set of web/src/components/project-icons.tsx.
var _projectIcons = []string{
	"rocket", "bolt", "flame", "wave", "leaf", "globe", "star", "bulb", "flask", "wrench",
	"box", "puzzle", "target", "lock", "chart", "note", "palette", "gamepad", "bot", "brain",
	"cart", "chat", "phone", "cloud", "home", "compass", "code", "book",
}

// Project is a repo folder Reef works in.
type Project struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`
	Path       string    `json:"path"`
	BaseBranch string    `json:"baseBranch"`
	Purpose    string    `json:"purpose"`
	Icon       string    `json:"icon"`
	Colour     string    `json:"colour"`
	CreatedAt  time.Time `json:"createdAt"`
}

// CreateProject adds p; name and path are required.
func (s *Store) CreateProject(ctx context.Context, p Project) (Project, error) {
	if err := p.validate(); err != nil {
		return Project{}, err
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO project (name, path, purpose, icon, colour, created_at) VALUES (?, ?, ?, ?, ?, ?)",
		p.Name, p.Path, p.Purpose, p.Icon, p.Colour, now.Format(time.RFC3339Nano))
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

// Project returns the project with the id, or ErrNotFound.
func (s *Store) Project(ctx context.Context, id int64) (Project, error) {
	row := s.db.QueryRowContext(ctx,
		"SELECT id, name, path, base_branch, purpose, icon, colour, created_at FROM project WHERE id = ?", id)
	p, err := scanProject(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Project{}, fmt.Errorf("project %d: %w", id, ErrNotFound)
	}
	return p, err
}

// ProjectByPath returns the project whose path is path or, failing that, the
// nearest parent folder of path that is a project path, or ErrNotFound.
func (s *Store) ProjectByPath(ctx context.Context, path string) (Project, error) {
	for dir := filepath.Clean(path); ; dir = filepath.Dir(dir) {
		row := s.db.QueryRowContext(ctx,
			"SELECT id, name, path, base_branch, purpose, icon, colour, created_at FROM project WHERE path = ? ORDER BY id LIMIT 1",
			dir)
		p, err := scanProject(row)
		if err == nil {
			return p, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return Project{}, err
		}
		if filepath.Dir(dir) == dir {
			return Project{}, fmt.Errorf("project for path %q: %w", path, ErrNotFound)
		}
	}
}

// Projects lists every project by id.
func (s *Store) Projects(ctx context.Context) ([]Project, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, name, path, base_branch, purpose, icon, colour, created_at FROM project ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	return scanAll(rows, scanProject)
}

// UpdateProject sets the name, path, base branch, purpose, icon and colour of p.ID.
func (s *Store) UpdateProject(ctx context.Context, p Project) (Project, error) {
	if err := p.validate(); err != nil {
		return Project{}, err
	}
	res, err := s.db.ExecContext(ctx,
		"UPDATE project SET name = ?, path = ?, base_branch = ?, purpose = ?, icon = ?, colour = ? WHERE id = ?",
		p.Name, p.Path, p.BaseBranch, p.Purpose, p.Icon, p.Colour, p.ID)
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

// DeleteProject removes the project and, by cascade, its work items, Spaces
// and tasks.
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

// ProjectMemory is the conventions and architecture notes shared by every work item and
// quick session of a project. The user writes it; there is one body per project.
type ProjectMemory struct {
	ProjectID int64  `json:"projectId"`
	Body      string `json:"body"`
}

// ProjectMemory returns the project's memory, empty when none has been set.
func (s *Store) ProjectMemory(ctx context.Context, projectID int64) (ProjectMemory, error) {
	m := ProjectMemory{ProjectID: projectID}
	err := s.db.QueryRowContext(ctx, "SELECT memory FROM project WHERE id = ?", projectID).
		Scan(&m.Body)
	if errors.Is(err, sql.ErrNoRows) {
		return ProjectMemory{}, fmt.Errorf("project %d: %w", projectID, ErrNotFound)
	}
	return m, err
}

// PutProjectMemory sets the project's memory, replacing any before.
func (s *Store) PutProjectMemory(ctx context.Context, m ProjectMemory) (ProjectMemory, error) {
	res, err := s.db.ExecContext(ctx,
		"UPDATE project SET memory = ? WHERE id = ?", m.Body, m.ProjectID)
	if err != nil {
		return ProjectMemory{}, fmt.Errorf("update project memory: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return ProjectMemory{}, fmt.Errorf("update project memory: %w", err)
	}
	if n == 0 {
		return ProjectMemory{}, fmt.Errorf("project %d: %w", m.ProjectID, ErrNotFound)
	}
	return m, nil
}

func (p Project) validate() error {
	if p.Name == "" {
		return fmt.Errorf("%w: project name is required", ErrInvalid)
	}
	if p.Path == "" {
		return fmt.Errorf("%w: project path is required", ErrInvalid)
	}
	if p.Icon != "" && !slices.Contains(_projectIcons, p.Icon) {
		return fmt.Errorf("%w: project icon %q is not one of %s", ErrInvalid, p.Icon, strings.Join(_projectIcons, ", "))
	}
	if p.Colour != "" && !slices.ContainsFunc(_tileColours, func(c string) bool { return strings.EqualFold(c, p.Colour) }) {
		return fmt.Errorf("%w: project colour %q is not one of %s", ErrInvalid, p.Colour, strings.Join(_tileColours, ", "))
	}
	return nil
}

func scanProject(r interface{ Scan(...any) error }) (Project, error) {
	var p Project
	var created string
	if err := r.Scan(&p.ID, &p.Name, &p.Path, &p.BaseBranch, &p.Purpose, &p.Icon, &p.Colour, &created); err != nil {
		return Project{}, err
	}
	t, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return Project{}, fmt.Errorf("project %d: bad created_at: %w", p.ID, err)
	}
	p.CreatedAt = t
	return p, nil
}
