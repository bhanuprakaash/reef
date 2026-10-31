package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// RequestOwnerTransfer records that the top-level item id goes to the Bot botID. It changes
// nothing else: ApplyOwnerTransfer makes the Bot the owner. A later request replaces an earlier
// one that is still pending, and one for the current owner withdraws it.
func (s *Store) RequestOwnerTransfer(ctx context.Context, id, botID int64) (WorkItem, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return WorkItem{}, fmt.Errorf("begin transfer: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	w, err := txWorkItem(ctx, tx, id)
	if err != nil {
		return WorkItem{}, err
	}
	switch {
	case w.ParentID != nil:
		return WorkItem{}, fmt.Errorf("%w: work item %d is a sub-item, it shares its parent's owner", ErrInvalid, id)
	case w.Status == _done || w.Status == _dropped:
		return WorkItem{}, fmt.Errorf("%w: work item %d is %s, its owner is fixed", ErrInvalid, id, w.Status)
	}
	var owner *int64
	if err := tx.QueryRowContext(ctx, "SELECT owner_bot_id FROM work_item WHERE id = ?", id).Scan(&owner); err != nil {
		return WorkItem{}, fmt.Errorf("read owner: %w", err)
	}
	var pending *int64
	if owner == nil || *owner != botID {
		pending = &botID
	}
	if _, err := tx.ExecContext(ctx, "UPDATE work_item SET pending_owner_bot_id = ? WHERE id = ?", pending, id); err != nil {
		if isForeignKey(err) {
			return WorkItem{}, fmt.Errorf("%w: unknown bot %d", ErrInvalid, botID)
		}
		return WorkItem{}, fmt.Errorf("record transfer: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return WorkItem{}, fmt.Errorf("commit transfer: %w", err)
	}
	return s.WorkItem(ctx, id)
}

// PendingOwnerTransfers lists the items with a transfer waiting for the turn of their Space to
// end, or for the restart: those of Space spaceID, or of every Space when it is 0.
func (s *Store) PendingOwnerTransfers(ctx context.Context, spaceID int64) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT w.id FROM work_item w WHERE w.pending_owner_bot_id IS NOT NULL AND (? = 0 OR EXISTS "+
			"(SELECT 1 FROM space WHERE work_item_id = w.id AND closed_at IS NULL AND id = ?)) ORDER BY w.id",
		spaceID, spaceID)
	if err != nil {
		return nil, fmt.Errorf("list pending transfers: %w", err)
	}
	return scanAll(rows, func(r interface{ Scan(...any) error }) (id int64, err error) {
		return id, r.Scan(&id)
	})
}

// ApplyOwnerTransfer makes the pending Bot the owner of item id and a participant. When the item
// has an open Space, in the same transaction the Bot joins it if it is not a member, becomes its
// coordinator, and gets a handover exchange from the user, whose origin message tells the Space
// that ownership moved. The old owner stays a member and keeps the requests addressed to it. It
// reports whether a handover exchange was made: a transfer with no Space, or none pending, makes
// none. Applying twice delivers one handover.
func (s *Store) ApplyOwnerTransfer(ctx context.Context, id int64) (DirectMessage, bool, error) {
	var botID, oldID *int64
	var title, name, oldName string
	var spaceID *int64
	var closed sql.NullString
	err := s.db.QueryRowContext(ctx,
		"SELECT w.pending_owner_bot_id, w.owner_bot_id, w.title, w.closed_at, COALESCE(b.name, ''), "+
			"COALESCE(o.name, ''), (SELECT id FROM space WHERE work_item_id = w.id AND closed_at IS NULL) "+
			"FROM work_item w LEFT JOIN bot b ON b.id = w.pending_owner_bot_id "+
			"LEFT JOIN bot o ON o.id = w.owner_bot_id WHERE w.id = ?", id).
		Scan(&botID, &oldID, &title, &closed, &name, &oldName, &spaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return DirectMessage{}, false, fmt.Errorf("work item %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return DirectMessage{}, false, fmt.Errorf("read transfer of work item %d: %w", id, err)
	}
	if botID == nil {
		return DirectMessage{}, false, nil
	}
	if spaceID == nil || closed.Valid {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return DirectMessage{}, false, fmt.Errorf("begin transfer: %w", err)
		}
		defer func() { _ = tx.Rollback() }()
		if err := setOwner(ctx, tx, id, *botID); err != nil {
			return DirectMessage{}, false, err
		}
		return DirectMessage{}, false, tx.Commit()
	}
	n, err := s.countOwnerHandovers(ctx, id)
	if err != nil {
		return DirectMessage{}, false, err
	}
	body := fmt.Sprintf("You are now the owner and coordinator of Work item %d, %s. ", id, title)
	note := fmt.Sprintf("Ownership moved to %s.", name)
	if oldName != "" {
		body += "It was handed over by " + oldName + ". Requests already addressed to " + oldName + " stay with " + oldName + ". "
		note = fmt.Sprintf("Ownership moved from %s to %s.", oldName, name)
	}
	body += "The Space, its workspaces and its branches are unchanged: read the Space's latest messages and the " +
		"branches' state before you continue."
	key := fmt.Sprintf("owner:%d:%d:%d", id, *botID, n)
	dm, created, err := s.createExchange(ctx, DirectMessage{
		SpaceID: *spaceID, OriginSpaceID: spaceID, From: "user", To: name, ToBotID: botID,
		Body: body, Kind: ExchangeGuidance, RequestKey: &key,
	}, note, func(tx *sql.Tx, _ *DirectMessage) error {
		if err := setOwner(ctx, tx, id, *botID); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx,
			"INSERT OR IGNORE INTO space_member (space_id, bot_id, lead) VALUES (?, ?, 0)", *spaceID, *botID)
		if err != nil {
			return fmt.Errorf("add owner to Space: %w", err)
		}
		if added, err := res.RowsAffected(); err == nil && added > 0 {
			if _, err := s.insertMessage(ctx, tx,
				Message{SpaceID: *spaceID, Author: ReefAuthor, Body: name + JoinedSuffix}, time.Now().UTC()); err != nil {
				return err
			}
		}
		return setLead(ctx, tx, *spaceID, *botID)
	})
	return dm, created, err
}

// setOwner makes the Bot botID the owner and a participant of item id, if it is still the pending
// one, and clears the pending transfer.
func setOwner(ctx context.Context, tx *sql.Tx, id, botID int64) error {
	res, err := tx.ExecContext(ctx,
		"UPDATE work_item SET owner_bot_id = ?, pending_owner_bot_id = NULL WHERE id = ? AND pending_owner_bot_id = ?",
		botID, id, botID)
	if err != nil {
		return fmt.Errorf("set owner: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return fmt.Errorf("%w: the transfer of work item %d changed", ErrConflict, id)
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT OR IGNORE INTO work_item_participant (work_item_id, bot_id) VALUES (?, ?)", id, botID); err != nil {
		return fmt.Errorf("add participant: %w", err)
	}
	return nil
}

// setLead makes the member botID the one lead of Space id. The old lead steps down first: one
// UPDATE would meet the one-lead index when the new lead's row comes before the old lead's.
func setLead(ctx context.Context, tx *sql.Tx, id, botID int64) error {
	if _, err := tx.ExecContext(ctx, "UPDATE space_member SET lead = 0 WHERE space_id = ?", id); err != nil {
		return fmt.Errorf("set coordinator: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		"UPDATE space_member SET lead = 1 WHERE space_id = ? AND bot_id = ?", id, botID); err != nil {
		return fmt.Errorf("set coordinator: %w", err)
	}
	return nil
}

func (s *Store) countOwnerHandovers(ctx context.Context, id int64) (n int, err error) {
	err = s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM direct_message WHERE request_key LIKE ?", fmt.Sprintf("owner:%d:%%", id)).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count handovers: %w", err)
	}
	return n, nil
}
