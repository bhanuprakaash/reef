package store

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"
)

const (
	_defaultType   = "feature"
	_defaultStatus = "backlog"
	_workItemCols  = "id, project_id, parent_id, type, title, description, status, team_id, " +
		"proposed_by, suggested_team_id, queue_position, token_budget, created_at, closed_at"
)

var (
	_types    = []string{"feature", "bug", "incident", "investigation", "task", "improvement"}
	_statuses = []string{"backlog", "queued", "in_progress", "needs_you", "review", "done", "dropped"}
)

// AcceptanceLine is one thing that must hold for a Work item to be done. Position counts
// from 1 and is set by the order of the lines written.
type AcceptanceLine struct {
	Position int    `json:"position"`
	Text     string `json:"text"`
	Checked  bool   `json:"checked"`
	Note     string `json:"note"`
}

// WorkItem is a unit of work in a project. A sub-item has a ParentID; a sub-item has no
// sub-items of its own.
type WorkItem struct {
	ID              int64            `json:"id"`
	ProjectID       int64            `json:"projectId"`
	ParentID        *int64           `json:"parentId"`
	Type            string           `json:"type"`
	Title           string           `json:"title"`
	Description     string           `json:"description"`
	Status          string           `json:"status"`
	TeamID          *int64           `json:"teamId"`
	ProposedBy      string           `json:"proposedBy"`
	SuggestedTeamID *int64           `json:"suggestedTeamId"`
	QueuePosition   *int64           `json:"queuePosition"`
	TokenBudget     int64            `json:"tokenBudget"`
	CreatedAt       time.Time        `json:"createdAt"`
	ClosedAt        *time.Time       `json:"closedAt"`
	AcceptanceLines []AcceptanceLine `json:"acceptanceLines"`
}

// WorkItemFilter narrows WorkItems. A nil id or an empty Status or Type matches everything.
type WorkItemFilter struct {
	ProjectID *int64
	ParentID  *int64
	Status    string
	Type      string
}

// TreeKey names the worktree folder and branch of the item's top-level item: a sub-item works
// in its parent's tree, so every tree, branch and PR lookup goes through here.
func (w WorkItem) TreeKey() string {
	if w.ParentID != nil {
		return strconv.FormatInt(*w.ParentID, 10)
	}
	return strconv.FormatInt(w.ID, 10)
}

// CreateWorkItem adds w and its acceptance lines. A parent must be a top-level item of
// w.ProjectID.
func (s *Store) CreateWorkItem(ctx context.Context, w WorkItem) (WorkItem, error) {
	if err := w.validate(); err != nil {
		return WorkItem{}, err
	}
	if err := s.checkParent(ctx, w); err != nil {
		return WorkItem{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return WorkItem{}, fmt.Errorf("begin create work item: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	w, err = insertWorkItem(ctx, tx, w)
	if err != nil {
		return WorkItem{}, s.writeErr(ctx, "write work item", err, ErrInvalid,
			parent{table: "project", id: &w.ProjectID}, parent{table: "team", id: w.TeamID},
			parent{table: "team", id: w.SuggestedTeamID})
	}
	if err := putLines(ctx, tx, w.ID, w.AcceptanceLines); err != nil {
		return WorkItem{}, err
	}
	if err := tx.Commit(); err != nil {
		return WorkItem{}, fmt.Errorf("commit create work item: %w", err)
	}
	return s.WorkItem(ctx, w.ID)
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

var (
	_ execer = (*sql.DB)(nil)
	_ execer = (*sql.Tx)(nil)
)

func insertWorkItem(ctx context.Context, x execer, w WorkItem) (WorkItem, error) {
	if w.TokenBudget == 0 {
		w.TokenBudget = TokenBudget
	}
	now := time.Now().UTC()
	res, err := x.ExecContext(ctx,
		"INSERT INTO work_item (project_id, parent_id, type, title, description, status, team_id, "+
			"proposed_by, suggested_team_id, queue_position, token_budget, created_at) "+
			"VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		w.ProjectID, w.ParentID, w.Type, w.Title, w.Description, w.Status, w.TeamID,
		w.ProposedBy, w.SuggestedTeamID, w.QueuePosition, w.TokenBudget,
		now.Format(time.RFC3339Nano))
	if err != nil {
		return w, fmt.Errorf("insert work item: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return w, fmt.Errorf("insert work item: %w", err)
	}
	w.ID, w.CreatedAt = id, now
	return w, nil
}

// checkParent is ErrInvalid unless w has no parent, or its parent is a top-level item of the
// same project.
func (s *Store) checkParent(ctx context.Context, w WorkItem) error {
	if w.ParentID == nil {
		return nil
	}
	p, err := s.WorkItem(ctx, *w.ParentID)
	if errors.Is(err, ErrNotFound) {
		return fmt.Errorf("%w: unknown work item %d", ErrInvalid, *w.ParentID)
	}
	if err != nil {
		return err
	}
	if p.ParentID != nil {
		return fmt.Errorf("%w: work item %d is a sub-item and cannot have sub-items", ErrInvalid, p.ID)
	}
	if p.ProjectID != w.ProjectID {
		return fmt.Errorf("%w: work item %d is not in project %d", ErrInvalid, p.ID, w.ProjectID)
	}
	return nil
}

// WorkItem returns the item with the id and its acceptance lines, or ErrNotFound.
func (s *Store) WorkItem(ctx context.Context, id int64) (WorkItem, error) {
	row := s.db.QueryRowContext(ctx, "SELECT "+_workItemCols+" FROM work_item WHERE id = ?", id)
	w, err := scanWorkItem(row)
	if errors.Is(err, sql.ErrNoRows) {
		return WorkItem{}, fmt.Errorf("work item %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return WorkItem{}, err
	}
	return s.withLines(ctx, w)
}

// WorkItems lists items by id, narrowed by f.
func (s *Store) WorkItems(ctx context.Context, f WorkItemFilter) ([]WorkItem, error) {
	if f.Status != "" && !slices.Contains(_statuses, f.Status) {
		return nil, fmt.Errorf("%w: unknown status %q", ErrInvalid, f.Status)
	}
	if f.Type != "" && !slices.Contains(_types, f.Type) {
		return nil, fmt.Errorf("%w: unknown type %q", ErrInvalid, f.Type)
	}
	q, args := "SELECT "+_workItemCols+" FROM work_item WHERE 1 = 1", []any{}
	if f.ProjectID != nil {
		q, args = q+" AND project_id = ?", append(args, *f.ProjectID)
	}
	if f.ParentID != nil {
		q, args = q+" AND parent_id = ?", append(args, *f.ParentID)
	}
	if f.Status != "" {
		q, args = q+" AND status = ?", append(args, f.Status)
	}
	if f.Type != "" {
		q, args = q+" AND type = ?", append(args, f.Type)
	}
	rows, err := s.db.QueryContext(ctx, q+" ORDER BY id", args...)
	if err != nil {
		return nil, fmt.Errorf("list work items: %w", err)
	}
	ws, err := scanAll(rows, scanWorkItem)
	if err != nil {
		return nil, fmt.Errorf("list work items: %w", err)
	}
	for i := range ws {
		if ws[i], err = s.withLines(ctx, ws[i]); err != nil {
			return nil, err
		}
	}
	return ws, nil
}

// UpdateWorkItem sets the editable fields of w.ID. An empty Type, Status or a zero TokenBudget
// keeps the old one, and nil AcceptanceLines keep the old lines; any other lines replace them.
// An item never changes project, parent or proposer.
func (s *Store) UpdateWorkItem(ctx context.Context, w WorkItem) (WorkItem, error) {
	old, err := s.WorkItem(ctx, w.ID)
	if err != nil {
		return WorkItem{}, err
	}
	w.Type, w.Status = cmp.Or(w.Type, old.Type), cmp.Or(w.Status, old.Status)
	if w.TokenBudget == 0 {
		w.TokenBudget = old.TokenBudget
	}
	if err := w.validate(); err != nil {
		return WorkItem{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return WorkItem{}, fmt.Errorf("begin update work item: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx,
		"UPDATE work_item SET type = ?, title = ?, description = ?, status = ?, team_id = ?, "+
			"suggested_team_id = ?, queue_position = ?, token_budget = ? WHERE id = ?",
		w.Type, w.Title, w.Description, w.Status, w.TeamID, w.SuggestedTeamID, w.QueuePosition,
		w.TokenBudget, w.ID)
	if err != nil {
		return WorkItem{}, s.writeErr(ctx, "update work item", err, ErrInvalid,
			parent{table: "team", id: w.TeamID}, parent{table: "team", id: w.SuggestedTeamID})
	}
	if w.AcceptanceLines != nil {
		if _, err := tx.ExecContext(ctx,
			"DELETE FROM acceptance_line WHERE work_item_id = ?", w.ID); err != nil {
			return WorkItem{}, fmt.Errorf("replace acceptance lines: %w", err)
		}
		if err := putLines(ctx, tx, w.ID, w.AcceptanceLines); err != nil {
			return WorkItem{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return WorkItem{}, fmt.Errorf("commit update work item: %w", err)
	}
	return s.WorkItem(ctx, w.ID)
}

// CloseWorkItem sets closed_at on the item and on each of its Spaces still open, in one
// transaction.
func (s *Store) CloseWorkItem(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin close work item: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	at := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := tx.ExecContext(ctx, "UPDATE work_item SET closed_at = ? WHERE id = ?", at, id)
	if err != nil {
		return fmt.Errorf("close work item: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("close work item: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("work item %d: %w", id, ErrNotFound)
	}
	if _, err := tx.ExecContext(ctx,
		"UPDATE space SET closed_at = ? WHERE work_item_id = ? AND closed_at IS NULL", at, id); err != nil {
		return fmt.Errorf("close Spaces: %w", err)
	}
	return tx.Commit()
}

// ReopenWorkItem clears the item's closed_at; its Spaces stay as they are.
func (s *Store) ReopenWorkItem(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, "UPDATE work_item SET closed_at = NULL WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("reopen work item: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("reopen work item: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("work item %d: %w", id, ErrNotFound)
	}
	return nil
}

// DeleteWorkItem removes the item and, by cascade, its sub-items, acceptance lines, briefs,
// notes, decisions and Spaces.
func (s *Store) DeleteWorkItem(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM work_item WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete work item: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete work item: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("work item %d: %w", id, ErrNotFound)
	}
	return nil
}

func putLines(ctx context.Context, tx *sql.Tx, id int64, lines []AcceptanceLine) error {
	for i, l := range lines {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO acceptance_line (work_item_id, position, text, checked, note) "+
				"VALUES (?, ?, ?, ?, ?)", id, i+1, l.Text, l.Checked, l.Note); err != nil {
			return fmt.Errorf("write acceptance line: %w", err)
		}
	}
	return nil
}

func (s *Store) withLines(ctx context.Context, w WorkItem) (WorkItem, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT position, text, checked, note FROM acceptance_line WHERE work_item_id = ? "+
			"ORDER BY position", w.ID)
	if err != nil {
		return WorkItem{}, fmt.Errorf("read acceptance lines: %w", err)
	}
	w.AcceptanceLines, err = scanAll(rows, func(r interface{ Scan(...any) error }) (AcceptanceLine, error) {
		var l AcceptanceLine
		err := r.Scan(&l.Position, &l.Text, &l.Checked, &l.Note)
		return l, err
	})
	if err != nil {
		return WorkItem{}, fmt.Errorf("read acceptance lines: %w", err)
	}
	if w.AcceptanceLines == nil {
		w.AcceptanceLines = []AcceptanceLine{}
	}
	return w, nil
}

// validate also fills the default type and status.
func (w *WorkItem) validate() error {
	if w.Title == "" {
		return fmt.Errorf("%w: work item title is required", ErrInvalid)
	}
	w.Type, w.Status = cmp.Or(w.Type, _defaultType), cmp.Or(w.Status, _defaultStatus)
	if !slices.Contains(_types, w.Type) {
		return fmt.Errorf("%w: unknown type %q", ErrInvalid, w.Type)
	}
	if !slices.Contains(_statuses, w.Status) {
		return fmt.Errorf("%w: unknown status %q", ErrInvalid, w.Status)
	}
	for i, l := range w.AcceptanceLines {
		if l.Text == "" {
			return fmt.Errorf("%w: acceptance line %d has no text", ErrInvalid, i+1)
		}
	}
	return nil
}

func scanWorkItem(r interface{ Scan(...any) error }) (WorkItem, error) {
	var w WorkItem
	var created string
	var closed sql.NullString
	if err := r.Scan(
		&w.ID, &w.ProjectID, &w.ParentID, &w.Type, &w.Title, &w.Description, &w.Status, &w.TeamID,
		&w.ProposedBy, &w.SuggestedTeamID, &w.QueuePosition, &w.TokenBudget, &created, &closed,
	); err != nil {
		return WorkItem{}, err
	}
	t, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return WorkItem{}, fmt.Errorf("work item %d: bad created_at: %w", w.ID, err)
	}
	w.CreatedAt = t
	if w.ClosedAt, err = parseClosed(closed); err != nil {
		return WorkItem{}, fmt.Errorf("work item %d: %w", w.ID, err)
	}
	return w, nil
}
