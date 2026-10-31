package store

import (
	"cmp"
	"context"
	"database/sql"
	"fmt"
	"slices"
	"time"
)

// Hold statuses. A claimed hold is reopened when its wake cannot start.
const (
	HoldPending = "pending"
	HoldClaimed = "approved"
	HoldDropped = "rejected"
)

const _holdKind = "hold"

const _holdCols = "id, space_id, from_role, to_role, message_id, direct_message_id, text, parent, status, created_at, scopes, from_bot_id, to_bot_id"

// Hold is a wake kept from running by a paused Space. Text is what To would run for the
// message MessageID that From wrote, as a reply in the thread of Parent when it is set.
type Hold struct {
	ID              int64 `json:"id"`
	SpaceID         int64
	From            string
	To              string
	FromBotID       *int64
	ToBotID         *int64
	MessageID       int64
	DirectMessageID *int64
	Text            string
	Parent          *int64
	Scopes          Scopes
	Status          string
	CreatedAt       time.Time
}

// CreateHold adds h as pending.
func (s *Store) CreateHold(ctx context.Context, h Hold) (Hold, error) {
	now := time.Now().UTC()
	var messageID any
	if h.MessageID != 0 {
		messageID = h.MessageID
	}
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO approval (space_id, from_role, to_role, message_id, direct_message_id, text, parent, kind, created_at, scopes, from_bot_id, to_bot_id) "+
			"VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		h.SpaceID, h.From, h.To, messageID, h.DirectMessageID, h.Text, h.Parent, _holdKind,
		now.Format(time.RFC3339Nano), h.Scopes, h.FromBotID, h.ToBotID)
	if err != nil {
		return Hold{}, s.writeErr(ctx, "insert hold", err, ErrNotFound,
			parent{table: "space", id: &h.SpaceID}, parent{table: "message", id: &h.MessageID})
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Hold{}, fmt.Errorf("insert hold: %w", err)
	}
	h.ID, h.Status, h.CreatedAt = id, HoldPending, now
	return h, nil
}

// PendingHolds lists the Space's pending holds, oldest first.
func (s *Store) PendingHolds(ctx context.Context, spaceID int64) ([]Hold, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT "+_holdCols+" FROM approval WHERE space_id = ? AND status = ? AND kind = ? ORDER BY id",
		spaceID, HoldPending, _holdKind)
	if err != nil {
		return nil, fmt.Errorf("list holds: %w", err)
	}
	return scanAll(rows, scanHold)
}

// ClaimHolds sets the Space's pending holds to claimed and returns them, oldest first.
// A second call finds none, so a hold is claimed once.
func (s *Store) ClaimHolds(ctx context.Context, spaceID int64) ([]Hold, error) {
	rows, err := s.db.QueryContext(ctx,
		"UPDATE approval SET status = ? WHERE space_id = ? AND status = ? AND kind = ? "+
			"RETURNING "+_holdCols,
		HoldClaimed, spaceID, HoldPending, _holdKind)
	if err != nil {
		return nil, fmt.Errorf("claim holds: %w", err)
	}
	hs, err := scanAll(rows, scanHold)
	if err != nil {
		return nil, fmt.Errorf("claim holds: %w", err)
	}
	slices.SortFunc(hs, func(a, b Hold) int { return cmp.Compare(a.ID, b.ID) })
	return hs, nil
}

// ReopenHold sets a claimed hold back to pending, for a wake that could not start.
func (s *Store) ReopenHold(ctx context.Context, id int64) error {
	if _, err := s.db.ExecContext(ctx,
		"UPDATE approval SET status = ? WHERE id = ? AND kind = ?", HoldPending, id, _holdKind); err != nil {
		return fmt.Errorf("reopen hold: %w", err)
	}
	return nil
}

func scanHold(r interface{ Scan(...any) error }) (Hold, error) {
	var h Hold
	var created string
	var messageID sql.NullInt64
	var directMessageID sql.NullInt64
	if err := r.Scan(&h.ID, &h.SpaceID, &h.From, &h.To, &messageID, &directMessageID, &h.Text, &h.Parent,
		&h.Status, &created, &h.Scopes, &h.FromBotID, &h.ToBotID); err != nil {
		return Hold{}, err
	}
	if messageID.Valid {
		h.MessageID = messageID.Int64
	}
	if directMessageID.Valid {
		v := directMessageID.Int64
		h.DirectMessageID = &v
	}
	t, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return Hold{}, fmt.Errorf("hold %d: bad created_at: %w", h.ID, err)
	}
	h.CreatedAt = t
	return h, nil
}
