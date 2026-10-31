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
	_workItemCols  = "id, project_id, parent_id, type, title, description, status, " +
		"proposed_by, queue_position, token_budget, created_at, closed_at"
)

func validType(t string) bool {
	switch t {
	case "feature", "bug", "incident", "investigation", "task", "improvement":
		return true
	}
	return false
}

func validStatus(s string) bool {
	switch s {
	case _backlog, _queued, _inProgress, _needsYou, _review, _done, _dropped:
		return true
	}
	return false
}

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
	ProposedBy      string           `json:"proposedBy"`
	QueuePosition   *int64           `json:"queuePosition"`
	TokenBudget     int64            `json:"tokenBudget"`
	CreatedAt       time.Time        `json:"createdAt"`
	ClosedAt        *time.Time       `json:"closedAt"`
	AcceptanceLines []AcceptanceLine `json:"acceptanceLines"`
	// OwnerBotID, ParticipantBotIDs (the owner among them) and Targets belong to the top-level
	// item: a sub-item reads its parent's and cannot set its own.
	OwnerBotID        *int64   `json:"ownerBotId"`
	ParticipantBotIDs []int64  `json:"participantBotIds"`
	Targets           []Target `json:"targets"`
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
// w.ProjectID. A top-level item gets its Project as its first target, and the owner and
// participants as given; a sub-item cannot be given any.
func (s *Store) CreateWorkItem(ctx context.Context, w WorkItem) (WorkItem, error) {
	if err := w.validate(); err != nil {
		return WorkItem{}, err
	}
	if err := s.checkParent(ctx, w); err != nil {
		return WorkItem{}, err
	}
	if w.ParentID != nil && (w.OwnerBotID != nil || len(w.ParticipantBotIDs) > 0) {
		return WorkItem{}, fmt.Errorf("%w: a sub-item shares its parent's owner and participants", ErrInvalid)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return WorkItem{}, fmt.Errorf("begin create work item: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	owner, others := w.OwnerBotID, w.ParticipantBotIDs
	w, err = insertWorkItem(ctx, tx, w)
	if err != nil {
		return WorkItem{}, s.writeErr(ctx, "write work item", err, ErrInvalid,
			parent{table: "project", id: &w.ProjectID})
	}
	if w.ParentID == nil {
		if err := putParticipants(ctx, tx, w.ID, owner, others); err != nil {
			return WorkItem{}, err
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO work_item_target (work_item_id, project_id, authorized_by, created_at) "+
				"VALUES (?, ?, ?, ?)", w.ID, w.ProjectID, ByUser,
			w.CreatedAt.Format(time.RFC3339Nano)); err != nil {
			return WorkItem{}, fmt.Errorf("add home target: %w", err)
		}
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
		"INSERT INTO work_item (project_id, parent_id, type, title, description, status, "+
			"proposed_by, queue_position, token_budget, created_at) "+
			"VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		w.ProjectID, w.ParentID, w.Type, w.Title, w.Description, w.Status,
		w.ProposedBy, w.QueuePosition, w.TokenBudget,
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
	if f.Status != "" && !validStatus(f.Status) {
		return nil, fmt.Errorf("%w: unknown status %q", ErrInvalid, f.Status)
	}
	if f.Type != "" && !validType(f.Type) {
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

// UpdateWorkItem sets the editable fields of w.ID. An empty Type or a zero TokenBudget keeps the
// old one, and nil AcceptanceLines keep the old lines; any other lines replace them, and a line
// whose text is unchanged keeps its checked and note; an item in review or done refuses new text
// (ErrConflict). An item never changes project, parent, proposer, status or queue position:
// MoveWorkItem moves the status and the queue. An owner or participants, when given, change only
// while the item is in the backlog; nil for both keeps them.
func (s *Store) UpdateWorkItem(ctx context.Context, w WorkItem) (WorkItem, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return WorkItem{}, fmt.Errorf("begin update work item: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	old, err := txWorkItem(ctx, tx, w.ID)
	if err != nil {
		return WorkItem{}, err
	}
	w.Type = cmp.Or(w.Type, old.Type)
	if w.TokenBudget == 0 {
		w.TokenBudget = old.TokenBudget
	}
	if err := w.validate(); err != nil {
		return WorkItem{}, err
	}
	if old, err = withScope(ctx, tx, old); err != nil {
		return WorkItem{}, err
	}
	if w.OwnerBotID == nil && w.ParticipantBotIDs == nil {
		w.OwnerBotID, w.ParticipantBotIDs = old.OwnerBotID, old.ParticipantBotIDs
	}
	oldSet := participantSet(old.OwnerBotID, old.ParticipantBotIDs)
	newSet := participantSet(w.OwnerBotID, w.ParticipantBotIDs)
	if !sameBot(old.OwnerBotID, w.OwnerBotID) || !slices.Equal(oldSet, newSet) {
		if old.ParentID != nil || old.Status != _backlog {
			return WorkItem{}, fmt.Errorf("%w: work item %d keeps its owner and participants", ErrInvalid, w.ID)
		}
		if err := putParticipants(ctx, tx, w.ID, w.OwnerBotID, w.ParticipantBotIDs); err != nil {
			return WorkItem{}, err
		}
	}
	if old.AcceptanceLines, err = readLines(ctx, tx, w.ID); err != nil {
		return WorkItem{}, err
	}
	sameText := func(a, b AcceptanceLine) bool { return a.Text == b.Text }
	if (old.Status == _review || old.Status == _done) && w.AcceptanceLines != nil &&
		!slices.EqualFunc(old.AcceptanceLines, w.AcceptanceLines, sameText) {
		return WorkItem{}, fmt.Errorf("%w: work item %d is in %s, its acceptance lines are fixed",
			ErrConflict, w.ID, old.Status)
	}
	_, err = tx.ExecContext(ctx,
		"UPDATE work_item SET type = ?, title = ?, description = ?, token_budget = ? WHERE id = ?",
		w.Type, w.Title, w.Description, w.TokenBudget, w.ID)
	if err != nil {
		return WorkItem{}, fmt.Errorf("update work item: %w", err)
	}
	if w.AcceptanceLines != nil {
		if _, err := tx.ExecContext(ctx,
			"DELETE FROM acceptance_line WHERE work_item_id = ?", w.ID); err != nil {
			return WorkItem{}, fmt.Errorf("replace acceptance lines: %w", err)
		}
		if err := putLines(ctx, tx, w.ID, keepTicks(old.AcceptanceLines, w.AcceptanceLines)); err != nil {
			return WorkItem{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return WorkItem{}, fmt.Errorf("commit update work item: %w", err)
	}
	return s.WorkItem(ctx, w.ID)
}

// closeInTx sets closed_at on the item and on each of its Spaces still open, and gives the
// direct requests waiting in its Spaces a stopped response.
func (s *Store) closeInTx(ctx context.Context, tx *sql.Tx, id int64) error {
	at := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, "UPDATE work_item SET closed_at = ? WHERE id = ?", at, id); err != nil {
		return fmt.Errorf("close work item: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		"UPDATE space SET closed_at = ? WHERE work_item_id = ? AND closed_at IS NULL", at, id); err != nil {
		return fmt.Errorf("close Spaces: %w", err)
	}
	return s.stopWaitingDirects(ctx, tx,
		"space_id IN (SELECT id FROM space WHERE work_item_id = ?)", id)
}

// DeleteWorkItem removes the item and, by cascade, its sub-items, acceptance lines, briefs,
// notes, decisions and Spaces. One that waited in the queue closes its gap.
func (s *Store) DeleteWorkItem(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delete work item: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	w, err := txWorkItem(ctx, tx, id)
	if err != nil {
		return err
	}
	if w.ParentID == nil {
		if err := requeue(ctx, tx, w, _dropped); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM work_item WHERE id = ?", id); err != nil {
		return fmt.Errorf("delete work item: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete work item: %w", err)
	}
	return nil
}

// txWorkItem reads the item inside tx, without its acceptance lines.
func txWorkItem(ctx context.Context, tx *sql.Tx, id int64) (WorkItem, error) {
	w, err := scanWorkItem(tx.QueryRowContext(ctx, "SELECT "+_workItemCols+" FROM work_item WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return WorkItem{}, fmt.Errorf("work item %d: %w", id, ErrNotFound)
	}
	return w, err
}

// keepTicks gives each new line whose text matches an old line, one old line per new line, the
// old line's checked and note: the writer of an edit holds a copy that may predate the lead's ticks.
func keepTicks(old, lines []AcceptanceLine) []AcceptanceLine {
	left := slices.Clone(old)
	kept := make([]AcceptanceLine, len(lines))
	for i, l := range lines {
		kept[i] = l
		if j := slices.IndexFunc(left, func(o AcceptanceLine) bool { return o.Text == l.Text }); j >= 0 {
			kept[i].Checked, kept[i].Note = left[j].Checked, left[j].Note
			left = slices.Delete(left, j, j+1)
		}
	}
	return kept
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
	lines, err := readLines(ctx, s.db, w.ID)
	if err != nil {
		return WorkItem{}, err
	}
	w.AcceptanceLines = lines
	return withScope(ctx, s.db, w)
}

func readLines(ctx context.Context, q interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, id int64) ([]AcceptanceLine, error) {
	rows, err := q.QueryContext(ctx,
		"SELECT position, text, checked, note FROM acceptance_line WHERE work_item_id = ? "+
			"ORDER BY position", id)
	if err != nil {
		return nil, fmt.Errorf("read acceptance lines: %w", err)
	}
	lines, err := scanAll(rows, func(r interface{ Scan(...any) error }) (AcceptanceLine, error) {
		var l AcceptanceLine
		err := r.Scan(&l.Position, &l.Text, &l.Checked, &l.Note)
		return l, err
	})
	if err != nil {
		return nil, fmt.Errorf("read acceptance lines: %w", err)
	}
	if lines == nil {
		lines = []AcceptanceLine{}
	}
	return lines, nil
}

// validate also fills the default type and status.
func (w *WorkItem) validate() error {
	if w.Title == "" {
		return fmt.Errorf("%w: work item title is required", ErrInvalid)
	}
	w.Type, w.Status = cmp.Or(w.Type, _defaultType), cmp.Or(w.Status, _defaultStatus)
	if !validType(w.Type) {
		return fmt.Errorf("%w: unknown type %q", ErrInvalid, w.Type)
	}
	if !validStatus(w.Status) {
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
		&w.ID, &w.ProjectID, &w.ParentID, &w.Type, &w.Title, &w.Description, &w.Status,
		&w.ProposedBy, &w.QueuePosition, &w.TokenBudget, &created, &closed,
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

func sameBot(a, b *int64) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }
