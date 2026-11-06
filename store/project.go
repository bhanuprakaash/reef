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

	// RepoConnectionID and RemotePath bind the Project to a repository on a provider host;
	// SetProjectRemote sets them, UpdateProject leaves them alone.
	RepoConnectionID *int64 `json:"repoConnectionId"`
	RemotePath       string `json:"remotePath"`

	// DeletingAt is when deleting the Project began, DeletedAt when it finished. From DeletingAt
	// on, no work is admitted against it; the row stays, readable by id, as history.
	DeletingAt *time.Time `json:"deletingAt"`
	DeletedAt  *time.Time `json:"deletedAt"`
}

// Active reports whether work may be admitted against the Project.
func (p Project) Active() bool { return p.DeletingAt == nil && p.DeletedAt == nil }

const (
	_projectCols = "id, name, path, base_branch, purpose, icon, colour, created_at, repo_connection_id, remote_path, " +
		"deleting_at, deleted_at"
	_activeProject = "deleting_at IS NULL AND deleted_at IS NULL"
)

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
		"SELECT "+_projectCols+" FROM project WHERE id = ?", id)
	p, err := scanProject(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Project{}, fmt.Errorf("project %d: %w", id, ErrNotFound)
	}
	return p, err
}

// ProjectByPath returns the active project whose path is path or, failing that, the
// nearest parent folder of path that is a project path, or ErrNotFound.
func (s *Store) ProjectByPath(ctx context.Context, path string) (Project, error) {
	for dir := filepath.Clean(path); ; dir = filepath.Dir(dir) {
		row := s.db.QueryRowContext(ctx,
			"SELECT "+_projectCols+" FROM project WHERE path = ? AND "+_activeProject+" ORDER BY id LIMIT 1",
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

// Projects lists the projects work may be admitted against, by id; one being deleted or deleted
// is left out and is read with Project.
func (s *Store) Projects(ctx context.Context) ([]Project, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT "+_projectCols+" FROM project WHERE "+_activeProject+" ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	return scanAll(rows, scanProject)
}

// DeletedProjects lists projects whose deletion has started.
func (s *Store) DeletedProjects(ctx context.Context) ([]Project, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+_projectCols+" FROM project WHERE deleting_at IS NOT NULL ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("list deleted projects: %w", err)
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

// MarkProjectDeleting records that deleting the project began, and returns it. From then on no
// work is admitted against it. A project already deleting or deleted keeps its first times.
func (s *Store) MarkProjectDeleting(ctx context.Context, id int64) (Project, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return s.markProject(ctx, id, "deleting_at = COALESCE(deleting_at, ?)", now)
}

// MarkProjectDeleted records that the project is deleted, and returns it. Its row, repository
// folder and workspaces stay.
func (s *Store) MarkProjectDeleted(ctx context.Context, id int64) (Project, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return s.markProject(ctx, id,
		"deleting_at = COALESCE(deleting_at, ?), deleted_at = COALESCE(deleted_at, ?)", now, now)
}

func (s *Store) markProject(ctx context.Context, id int64, set string, args ...any) (Project, error) {
	res, err := s.db.ExecContext(ctx, "UPDATE project SET "+set+" WHERE id = ?", append(args, id)...)
	if err != nil {
		return Project{}, fmt.Errorf("mark project deleted: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return Project{}, fmt.Errorf("project %d: %w", id, ErrNotFound)
	}
	return s.Project(ctx, id)
}

// DeletingProjects lists the projects whose deletion began and did not finish, by id.
func (s *Store) DeletingProjects(ctx context.Context) ([]Project, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT "+_projectCols+" FROM project WHERE deleting_at IS NOT NULL AND deleted_at IS NULL ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("list deleting projects: %w", err)
	}
	return scanAll(rows, scanProject)
}

// _authorizedTarget selects, on a work_item_target table named t, a repository the user or an
// approval gave, not one a Bot only proposed.
const _authorizedTarget = "t.authorized_by != '' AND t.authorized_by NOT LIKE 'proposed by%'"

// _inactiveProject selects, on a project table named p, a project being deleted or deleted.
const _inactiveProject = "(p.deleting_at IS NOT NULL OR p.deleted_at IS NOT NULL)"

// requireActive is ErrInvalid when project id exists and is being deleted or deleted; an unknown
// id is left for the caller's own write to refuse.
func requireActive(ctx context.Context, q reader, id int64) error {
	var inactive bool
	err := q.QueryRowContext(ctx, "SELECT "+_inactiveProject+" FROM project p WHERE p.id = ?", id).Scan(&inactive)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read project %d: %w", id, err)
	}
	if inactive {
		return fmt.Errorf("%w: project %d is deleted", ErrInvalid, id)
	}
	return nil
}

// inactiveTarget returns the name of the first repository the top-level item of item id may
// change that is being deleted or deleted, "" when there is none.
func inactiveTarget(ctx context.Context, q reader, id int64) (string, error) {
	var name string
	err := q.QueryRowContext(ctx,
		"SELECT p.name FROM work_item_target t JOIN project p ON p.id = t.project_id "+
			"WHERE t.work_item_id = COALESCE((SELECT parent_id FROM work_item WHERE id = ?1), ?1) "+
			"AND "+_authorizedTarget+" AND "+_inactiveProject+" ORDER BY t.rowid LIMIT 1", id).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read targets of work item %d: %w", id, err)
	}
	return name, nil
}

// ItemInactiveTarget returns the name of the first repository work item id may change that is
// being deleted or deleted, "" when there is none.
func (s *Store) ItemInactiveTarget(ctx context.Context, id int64) (string, error) {
	return inactiveTarget(ctx, s.db, id)
}

// SpaceInactiveProject returns the name of the first repository being deleted or deleted that the
// work item of Space id may change, "" when there is none or the Space has no work item.
func (s *Store) SpaceInactiveProject(ctx context.Context, id int64) (string, error) {
	var name string
	err := s.db.QueryRowContext(ctx,
		"SELECT p.name FROM project p WHERE "+_inactiveProject+" AND ("+
			"p.id IN (SELECT t.project_id FROM space s JOIN work_item_target t ON t.work_item_id = "+
			"COALESCE((SELECT parent_id FROM work_item WHERE id = s.work_item_id), s.work_item_id) "+
			"WHERE s.id = ?1 AND "+_authorizedTarget+")) "+
			"ORDER BY p.id LIMIT 1", id).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read projects of Space %d: %w", id, err)
	}
	return name, nil
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
	var connID sql.NullInt64
	var deleting, deleted sql.NullString
	if err := r.Scan(&p.ID, &p.Name, &p.Path, &p.BaseBranch, &p.Purpose, &p.Icon, &p.Colour, &created,
		&connID, &p.RemotePath, &deleting, &deleted); err != nil {
		return Project{}, err
	}
	for _, f := range []struct {
		from sql.NullString
		to   **time.Time
	}{{deleting, &p.DeletingAt}, {deleted, &p.DeletedAt}} {
		if !f.from.Valid {
			continue
		}
		t, err := time.Parse(time.RFC3339Nano, f.from.String)
		if err != nil {
			return Project{}, fmt.Errorf("project %d: bad deletion time: %w", p.ID, err)
		}
		*f.to = &t
	}
	if connID.Valid {
		p.RepoConnectionID = &connID.Int64
	}
	t, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return Project{}, fmt.Errorf("project %d: bad created_at: %w", p.ID, err)
	}
	p.CreatedAt = t
	return p, nil
}
