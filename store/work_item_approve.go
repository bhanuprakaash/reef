package store

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"time"
)

// ApproveWorkItem authorizes the top-level backlog item id and queues it last, in one step. The
// owner and participants become what the user decided: owner and others, or the proposal's where
// nil. The owner is always a participant. targets is the final set of repositories, the home
// Project always among them, or nil to keep every proposed one; the user authorizes those and the
// proposed ones left out are dropped.
// The same key again returns the item as it is, with no second queue entry; a key already used by
// another item, an item approved under another key, or an item that is not in the backlog is
// ErrConflict.
func (s *Store) ApproveWorkItem(
	ctx context.Context, id int64, owner *int64, others []int64, targets []int64, key string,
) (WorkItem, error) {
	if key == "" {
		return WorkItem{}, fmt.Errorf("%w: an approval needs a key", ErrInvalid)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return WorkItem{}, fmt.Errorf("begin approve work item: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	w, err := txWorkItem(ctx, tx, id)
	if err != nil {
		return WorkItem{}, err
	}
	if w.ApprovalKey != nil && *w.ApprovalKey == key {
		return s.WorkItem(ctx, id)
	}
	switch {
	case w.ParentID != nil:
		return WorkItem{}, fmt.Errorf("%w: work item %d is a sub-item, only its parent is approved", ErrInvalid, id)
	case w.ApprovedAt != nil:
		return WorkItem{}, fmt.Errorf("%w: work item %d is already approved under another key", ErrConflict, id)
	case w.Status != _backlog:
		return WorkItem{}, fmt.Errorf("%w: work item %d is %s, only a backlog item is approved", ErrConflict, id, w.Status)
	}
	if w, err = withScope(ctx, tx, w); err != nil {
		return WorkItem{}, err
	}
	if owner == nil {
		owner = w.OwnerBotID
	}
	if owner == nil {
		return WorkItem{}, fmt.Errorf("%w: work item %d needs an owner before it is approved", ErrInvalid, id)
	}
	if others == nil {
		others = w.ParticipantBotIDs
	}
	if err := putParticipants(ctx, tx, id, owner, others); err != nil {
		return WorkItem{}, err
	}
	if err := authorizeTargets(ctx, tx, w, targets); err != nil {
		return WorkItem{}, err
	}
	name, err := inactiveTarget(ctx, tx, id)
	if err != nil {
		return WorkItem{}, err
	}
	if name != "" {
		return WorkItem{}, fmt.Errorf("work item %d targets deleted project %s, adjust its repositories first: %w",
			id, name, ErrConflict)
	}
	if _, err := tx.ExecContext(ctx,
		"UPDATE work_item SET status = ?, approved_at = ?, approval_key = ? WHERE id = ?",
		_queued, time.Now().UTC().Format(time.RFC3339Nano), key, id); err != nil {
		if isUnique(err) {
			return WorkItem{}, fmt.Errorf("%w: approval key %q belongs to another work item", ErrConflict, key)
		}
		return WorkItem{}, fmt.Errorf("approve work item: %w", err)
	}
	if err := requeue(ctx, tx, w, _queued); err != nil {
		return WorkItem{}, err
	}
	if err := tx.Commit(); err != nil {
		return WorkItem{}, fmt.Errorf("commit approve work item: %w", err)
	}
	if err := s.StartQueues(ctx); err != nil {
		return WorkItem{}, err
	}
	return s.WorkItem(ctx, id)
}

func authorizeTargets(ctx context.Context, tx *sql.Tx, w WorkItem, targets []int64) error {
	if targets != nil {
		keep := slices.Clone(targets)
		if !slices.Contains(keep, w.ProjectID) {
			keep = append(keep, w.ProjectID)
		}
		for _, t := range w.Targets {
			if slices.Contains(keep, t.ProjectID) {
				continue
			}
			if _, err := tx.ExecContext(ctx,
				"DELETE FROM work_item_target WHERE work_item_id = ? AND project_id = ?", w.ID, t.ProjectID); err != nil {
				return fmt.Errorf("drop target: %w", err)
			}
		}
		for _, p := range keep {
			if _, err := tx.ExecContext(ctx,
				"INSERT OR IGNORE INTO work_item_target (work_item_id, project_id, created_at) VALUES (?, ?, ?)",
				w.ID, p, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
				if isForeignKey(err) {
					return fmt.Errorf("%w: unknown project %d", ErrInvalid, p)
				}
				return fmt.Errorf("add target: %w", err)
			}
		}
	}
	if _, err := tx.ExecContext(ctx,
		"UPDATE work_item_target SET authorized_by = ? WHERE work_item_id = ? AND authorized_by = ''",
		ByUser, w.ID); err != nil {
		return fmt.Errorf("authorize targets: %w", err)
	}
	return nil
}

// AddWorkItemParticipant makes the Bot a participant of the item's top-level item, as when the
// user lets it into one of the item's Spaces. A Bot already a participant stays one.
func (s *Store) AddWorkItemParticipant(ctx context.Context, id, botID int64) error {
	w, err := s.WorkItem(ctx, id)
	if err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx,
		"INSERT OR IGNORE INTO work_item_participant (work_item_id, bot_id) VALUES (?, ?)",
		w.scopeID(), botID); err != nil {
		if isForeignKey(err) {
			return fmt.Errorf("%w: unknown bot %d", ErrInvalid, botID)
		}
		return fmt.Errorf("add participant: %w", err)
	}
	return nil
}
