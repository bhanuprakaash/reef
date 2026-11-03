package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Workspace states. A workspace is made `creating` before any git command runs, so a restart
// finds it and finishes it; `paused` waits for the user to fix the repository.
const (
	WorkspaceCreating = "creating"
	WorkspaceReady    = "ready"
	WorkspacePaused   = "paused"
	WorkspaceRemoved  = "removed"
)

// Workspace is one working tree of one repository. An assignment has one per target, kept by
// its top-level Work item; a quick fix has WorkItemID nil, and is kept by the SpaceID of the
// conversation that holds it under its Name.
type Workspace struct {
	ID         int64     `json:"id"`
	ProjectID  int64     `json:"projectId"`
	WorkItemID *int64    `json:"workItemId"`
	SpaceID    *int64    `json:"spaceId"`
	Name       string    `json:"name"`
	Dir        string    `json:"dir"`
	Branch     string    `json:"branch"`
	Base       string    `json:"base"`
	BaseRef    string    `json:"baseRef"`
	BaseSource string    `json:"baseSource"`
	State      string    `json:"state"`
	Note       string    `json:"note"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

const _workspaceCols = "id, project_id, work_item_id, space_id, name, dir, branch, base, base_ref, " +
	"base_source, state, note, created_at, updated_at"

func scanWorkspace(r interface{ Scan(...any) error }) (Workspace, error) {
	var w Workspace
	var created, updated string
	err := r.Scan(&w.ID, &w.ProjectID, &w.WorkItemID, &w.SpaceID, &w.Name, &w.Dir, &w.Branch, &w.Base,
		&w.BaseRef, &w.BaseSource, &w.State, &w.Note, &created, &updated)
	if err != nil {
		return w, err
	}
	if w.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return w, fmt.Errorf("bad workspace created_at: %w", err)
	}
	if w.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated); err != nil {
		return w, fmt.Errorf("bad workspace updated_at: %w", err)
	}
	return w, nil
}

// EnsureWorkspace records w as `creating` and returns it, or returns the workspace already
// recorded for the same key: (WorkItemID, ProjectID) for an assignment, (SpaceID, Name,
// ProjectID) for a quick fix. The row found keeps its id, dir, branch and state, so a retry
// after a restart continues the same workspace.
func (s *Store) EnsureWorkspace(ctx context.Context, w Workspace) (Workspace, error) {
	if (w.WorkItemID == nil) == (w.SpaceID == nil) {
		return Workspace{}, fmt.Errorf("%w: a workspace belongs to a work item or to a Space", ErrInvalid)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx,
		"INSERT OR IGNORE INTO workspace (project_id, work_item_id, space_id, name, dir, branch, base, "+
			"state, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, 'creating', ?, ?)",
		w.ProjectID, w.WorkItemID, w.SpaceID, w.Name, w.Dir, w.Branch, w.Base, now, now)
	if err != nil {
		return Workspace{}, fmt.Errorf("record workspace: %w", err)
	}
	row := s.db.QueryRowContext(ctx, "SELECT "+_workspaceCols+" FROM workspace WHERE project_id = ? AND "+
		"((work_item_id = ?) OR (work_item_id IS NULL AND ? IS NULL AND space_id = ? AND name = ?))",
		w.ProjectID, w.WorkItemID, w.WorkItemID, w.SpaceID, w.Name)
	got, err := scanWorkspace(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Workspace{}, fmt.Errorf("%w: workspace folder %s is taken", ErrConflict, w.Dir)
	}
	return got, err
}

// Workspace returns the workspace with id.
func (s *Store) Workspace(ctx context.Context, id int64) (Workspace, error) {
	w, err := scanWorkspace(s.db.QueryRowContext(ctx, "SELECT "+_workspaceCols+" FROM workspace WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return w, fmt.Errorf("workspace %d: %w", id, ErrNotFound)
	}
	return w, err
}

// SetWorkspace records how creating w went: its base, base ref and source, state and note.
func (s *Store) SetWorkspace(ctx context.Context, w Workspace) error {
	res, err := s.db.ExecContext(ctx,
		"UPDATE workspace SET base = ?, base_ref = ?, base_source = ?, state = ?, note = ?, updated_at = ? "+
			"WHERE id = ?",
		w.Base, w.BaseRef, w.BaseSource, w.State, w.Note, time.Now().UTC().Format(time.RFC3339Nano), w.ID)
	if err != nil {
		return fmt.Errorf("update workspace %d: %w", w.ID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("workspace %d: %w", w.ID, ErrNotFound)
	}
	return nil
}

// WorkItemWorkspaces lists the workspaces of the top-level item id, in the order they were
// recorded. A sub-item has none of its own: ask with its parent's id.
func (s *Store) WorkItemWorkspaces(ctx context.Context, id int64) ([]Workspace, error) {
	return s.workspaces(ctx, "work_item_id = ?", id)
}

// SpaceWorkspaces lists the quick-fix workspaces the conversation id holds.
func (s *Store) SpaceWorkspaces(ctx context.Context, id int64) ([]Workspace, error) {
	return s.workspaces(ctx, "work_item_id IS NULL AND space_id = ?", id)
}

// ProjectWorkspaces lists every workspace of project id, assignments and quick fixes.
func (s *Store) ProjectWorkspaces(ctx context.Context, id int64) ([]Workspace, error) {
	return s.workspaces(ctx, "project_id = ?", id)
}

func (s *Store) workspaces(ctx context.Context, where string, id int64) ([]Workspace, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT "+_workspaceCols+" FROM workspace WHERE "+where+" ORDER BY id", id)
	if err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}
	ws, err := scanAll(rows, scanWorkspace)
	if err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}
	if ws == nil {
		ws = []Workspace{}
	}
	return ws, nil
}
