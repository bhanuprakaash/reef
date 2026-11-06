package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"
)

// Target is a repository, by its Project, a Work item may change. The item's own Project is
// always the first one.
type Target struct {
	ProjectID    int64     `json:"projectId"`
	ProjectName  string    `json:"projectName"`
	ProposedBy   string    `json:"proposedBy"`
	AuthorizedBy string    `json:"authorizedBy"`
	CreatedAt    time.Time `json:"createdAt"`
}

type reader interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// scopeID is the id of the top-level item whose owner, participants and targets w has: a
// sub-item reads its parent's.
func (w WorkItem) scopeID() int64 {
	if w.ParentID != nil {
		return *w.ParentID
	}
	return w.ID
}

// withScope fills the owner, participants and targets of w from its top-level item.
func withScope(ctx context.Context, q reader, w WorkItem) (WorkItem, error) {
	id := w.scopeID()
	if err := q.QueryRowContext(ctx, "SELECT owner_bot_id, pending_owner_bot_id FROM work_item WHERE id = ?", id).
		Scan(&w.OwnerBotID, &w.PendingOwnerBotID); err != nil {
		return WorkItem{}, fmt.Errorf("read owner of work item %d: %w", id, err)
	}
	rows, err := q.QueryContext(ctx,
		"SELECT bot_id FROM work_item_participant WHERE work_item_id = ? ORDER BY rowid", id)
	if err != nil {
		return WorkItem{}, fmt.Errorf("read participants of work item %d: %w", id, err)
	}
	w.ParticipantBotIDs, err = scanAll(rows, func(r interface{ Scan(...any) error }) (int64, error) {
		var b int64
		return b, r.Scan(&b)
	})
	if err != nil {
		return WorkItem{}, fmt.Errorf("read participants of work item %d: %w", id, err)
	}
	if w.ParticipantBotIDs == nil {
		w.ParticipantBotIDs = []int64{}
	}
	if w.Targets, err = readTargets(ctx, q, id); err != nil {
		return WorkItem{}, err
	}
	return w, nil
}

func readTargets(ctx context.Context, q reader, id int64) ([]Target, error) {
	rows, err := q.QueryContext(ctx,
		"SELECT t.project_id, COALESCE(p.name, 'deleted project ' || t.project_id), t.proposed_by, "+
			"t.authorized_by, t.created_at "+
			"FROM work_item_target t LEFT JOIN project p ON p.id = t.project_id "+
			"WHERE t.work_item_id = ? ORDER BY t.rowid", id)
	if err != nil {
		return nil, fmt.Errorf("read targets of work item %d: %w", id, err)
	}
	ts, err := scanAll(rows, func(r interface{ Scan(...any) error }) (Target, error) {
		var t Target
		var created string
		if err := r.Scan(&t.ProjectID, &t.ProjectName, &t.ProposedBy, &t.AuthorizedBy, &created); err != nil {
			return t, err
		}
		t.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
		return t, err
	})
	if err != nil {
		return nil, fmt.Errorf("read targets of work item %d: %w", id, err)
	}
	if ts == nil {
		ts = []Target{}
	}
	return ts, nil
}

// participantSet is the participants of an item with owner: the owner first, then the others
// once each.
func participantSet(owner *int64, others []int64) []int64 {
	var set []int64
	if owner != nil {
		set = append(set, *owner)
	}
	for _, b := range others {
		if !slices.Contains(set, b) {
			set = append(set, b)
		}
	}
	return set
}

// putParticipants sets the owner and participants of top-level item id in tx. The owner is a
// participant too.
func putParticipants(ctx context.Context, tx *sql.Tx, id int64, owner *int64, others []int64) error {
	if owner == nil && len(others) > 0 {
		return fmt.Errorf("%w: participants need an owner", ErrInvalid)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE work_item SET owner_bot_id = ? WHERE id = ?", owner, id); err != nil {
		if isForeignKey(err) {
			return fmt.Errorf("%w: unknown bot", ErrInvalid)
		}
		return fmt.Errorf("set owner: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM work_item_participant WHERE work_item_id = ?", id); err != nil {
		return fmt.Errorf("clear participants: %w", err)
	}
	for _, b := range participantSet(owner, others) {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO work_item_participant (work_item_id, bot_id) VALUES (?, ?)", id, b); err != nil {
			if isForeignKey(err) {
				return fmt.Errorf("%w: unknown bot %d", ErrInvalid, b)
			}
			return fmt.Errorf("add participant: %w", err)
		}
	}
	return nil
}

// SetWorkItemParticipants sets the owner and the other participants of the top-level item id,
// while it is in the backlog; otherwise ErrInvalid. The owner is always among the participants.
func (s *Store) SetWorkItemParticipants(
	ctx context.Context, id int64, owner *int64, others []int64,
) (WorkItem, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return WorkItem{}, fmt.Errorf("begin set participants: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	w, err := txWorkItem(ctx, tx, id)
	if err != nil {
		return WorkItem{}, err
	}
	if w.ParentID != nil {
		return WorkItem{}, fmt.Errorf("%w: work item %d is a sub-item, it shares its parent's owner and participants",
			ErrInvalid, id)
	}
	if w.Status != _backlog {
		return WorkItem{}, fmt.Errorf("%w: work item %d is %s, its owner and participants are fixed",
			ErrInvalid, id, w.Status)
	}
	if err := putParticipants(ctx, tx, id, owner, others); err != nil {
		return WorkItem{}, err
	}
	if err := tx.Commit(); err != nil {
		return WorkItem{}, fmt.Errorf("commit set participants: %w", err)
	}
	return s.WorkItem(ctx, id)
}

// WorkItemTargets lists the repositories the item may change: its parent's when it is a sub-item.
func (s *Store) WorkItemTargets(ctx context.Context, id int64) ([]Target, error) {
	w, err := s.WorkItem(ctx, id)
	if err != nil {
		return nil, err
	}
	return w.Targets, nil
}

// AddWorkItemTarget lets the top-level item id change the repository of the Project, authorized
// by authorizedBy. A sub-item (ErrInvalid),
// an unknown Project (ErrInvalid) and a Project already a target (ErrConflict) are refused. The
// item's active Space, if it has one, gets a message naming the Project and who authorized it.
func (s *Store) AddWorkItemTarget(
	ctx context.Context, id, projectID int64, authorizedBy string,
) (Target, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Target{}, fmt.Errorf("begin add target: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	w, err := txWorkItem(ctx, tx, id)
	if err != nil {
		return Target{}, err
	}
	if w.ParentID != nil {
		return Target{}, fmt.Errorf("%w: work item %d is a sub-item, it shares its parent's targets", ErrInvalid, id)
	}
	if authorizedBy == "" {
		return Target{}, fmt.Errorf("%w: a target needs who authorized it", ErrInvalid)
	}
	if err := requireActive(ctx, tx, projectID); err != nil {
		return Target{}, err
	}
	var name string
	if err := tx.QueryRowContext(ctx, "SELECT name FROM project WHERE id = ?", projectID).Scan(&name); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Target{}, fmt.Errorf("%w: unknown project %d", ErrInvalid, projectID)
		}
		return Target{}, fmt.Errorf("read project %d: %w", projectID, err)
	}
	now := time.Now().UTC()
	res, err := tx.ExecContext(ctx,
		"INSERT OR IGNORE INTO work_item_target (work_item_id, project_id, authorized_by, created_at) "+
			"VALUES (?, ?, ?, ?)", id, projectID, authorizedBy, now.Format(time.RFC3339Nano))
	if err != nil {
		return Target{}, fmt.Errorf("add target: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return Target{}, fmt.Errorf("%w: work item %d already targets project %d", ErrConflict, id, projectID)
	}
	_, err = tx.ExecContext(ctx,
		"INSERT INTO message (space_id, author, body, created_at) "+
			"SELECT id, ?, ?, ? FROM space WHERE work_item_id = ? AND closed_at IS NULL",
		ReefAuthor, fmt.Sprintf("Added repository %s to this Work item, authorized by %s.", name, authorizedBy),
		now.Format(time.RFC3339Nano), id)
	if err != nil {
		return Target{}, fmt.Errorf("note target: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Target{}, fmt.Errorf("commit add target: %w", err)
	}
	return Target{
		ProjectID: projectID, ProjectName: name, AuthorizedBy: authorizedBy, CreatedAt: now,
	}, nil
}

// RemoveWorkItemTarget takes the repository of the Project off the top-level item id. Only a
// backlog item loses a target (ErrConflict otherwise, and for a Project with a workspace), unless
// the Project is being deleted or deleted: its workspace rows stay as history, and an item
// paused for it goes on without it. Its home Project and a Project that is not a target are
// ErrInvalid.
func (s *Store) RemoveWorkItemTarget(ctx context.Context, id, projectID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin remove target: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	w, err := txWorkItem(ctx, tx, id)
	if err != nil {
		return err
	}
	gone := requireActive(ctx, tx, projectID) != nil
	switch {
	case w.ParentID != nil:
		return fmt.Errorf("%w: work item %d is a sub-item, it shares its parent's targets", ErrInvalid, id)
	case w.ProjectID == projectID:
		return fmt.Errorf("%w: project %d is the home of work item %d", ErrInvalid, projectID, id)
	case w.Status != _backlog && !gone:
		return fmt.Errorf("%w: work item %d is %s, only a backlog item loses a repository", ErrConflict, id, w.Status)
	}
	var workspaces int
	if err := tx.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM workspace WHERE work_item_id = ? AND project_id = ?", id, projectID).
		Scan(&workspaces); err != nil {
		return fmt.Errorf("read workspaces of work item %d: %w", id, err)
	}
	if workspaces > 0 && !gone {
		return fmt.Errorf("%w: work item %d has a workspace in project %d", ErrConflict, id, projectID)
	}
	res, err := tx.ExecContext(ctx,
		"DELETE FROM work_item_target WHERE work_item_id = ? AND project_id = ?", id, projectID)
	if err != nil {
		return fmt.Errorf("remove target: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return fmt.Errorf("%w: work item %d does not target project %d", ErrInvalid, id, projectID)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit remove target: %w", err)
	}
	return nil
}
