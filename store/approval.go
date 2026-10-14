package store

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"
)

// Approval statuses. Only a pending approval can be decided.
const (
	ApprovalPending  = "pending"
	ApprovalApproved = "approved"
	ApprovalRejected = "rejected"
)

// Approval kinds: a gate holds a wake until the user approves it; a hold keeps a wake a pause
// kept from running until the channel continues.
const (
	ApprovalGate = "gate"
	ApprovalHold = "hold"
)

const _approvalCols = "id, channel_id, from_role, to_role, message_id, text, parent, status, kind, created_at"

// Approval is a wake held at a gate or by a pause: Text is what To would run for the message
// MessageID that From wrote, as a reply in the thread of Parent when it is set. An empty Kind
// is a gate.
type Approval struct {
	ID        int64     `json:"id"`
	ChannelID int64     `json:"channelId"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	MessageID int64     `json:"messageId"`
	Text      string    `json:"text"`
	Parent    *int64    `json:"-"`
	Status    string    `json:"status"`
	Kind      string    `json:"-"`
	CreatedAt time.Time `json:"createdAt"`
}

// CreateApproval adds a as pending.
func (s *Store) CreateApproval(ctx context.Context, a Approval) (Approval, error) {
	if a.Kind == "" {
		a.Kind = ApprovalGate
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO approval (channel_id, from_role, to_role, message_id, text, parent, kind, created_at) "+
			"VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		a.ChannelID, a.From, a.To, a.MessageID, a.Text, a.Parent, a.Kind,
		now.Format(time.RFC3339Nano))
	if err != nil {
		return Approval{}, s.writeErr(ctx, "insert approval", err, ErrNotFound,
			parent{table: "channel", id: &a.ChannelID}, parent{table: "message", id: &a.MessageID})
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Approval{}, fmt.Errorf("insert approval: %w", err)
	}
	a.ID, a.Status, a.CreatedAt = id, ApprovalPending, now
	return a, nil
}

// PendingApprovals lists the channel's pending gate approvals, oldest first.
func (s *Store) PendingApprovals(ctx context.Context, channelID int64) ([]Approval, error) {
	if err := s.requireChannel(ctx, channelID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT "+_approvalCols+" FROM approval WHERE channel_id = ? AND status = ? AND kind = ? ORDER BY id",
		channelID, ApprovalPending, ApprovalGate)
	if err != nil {
		return nil, fmt.Errorf("list approvals: %w", err)
	}
	return scanAll(rows, scanApproval)
}

// Approval returns the gate approval of the channel with the id in any status, or ErrNotFound.
func (s *Store) Approval(ctx context.Context, channelID, id int64) (Approval, error) {
	row := s.db.QueryRowContext(ctx,
		"SELECT "+_approvalCols+" FROM approval WHERE id = ? AND channel_id = ? AND kind = ?",
		id, channelID, ApprovalGate)
	a, err := scanApproval(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Approval{}, fmt.Errorf("approval %d of channel %d: %w", id, channelID, ErrNotFound)
	}
	return a, err
}

// DecideApproval sets a pending approval to status, approved or rejected, or returns
// ErrConflict when it was decided already.
func (s *Store) DecideApproval(ctx context.Context, id int64, status string) error {
	res, err := s.db.ExecContext(ctx,
		"UPDATE approval SET status = ? WHERE id = ? AND status = ?", status, id, ApprovalPending)
	if err != nil {
		return fmt.Errorf("decide approval: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("decide approval: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("approval %d is not pending: %w", id, ErrConflict)
	}
	return nil
}

// ClaimHolds sets the channel's pending holds to approved and returns them, oldest first.
// A second call finds none, so a hold is claimed once.
func (s *Store) ClaimHolds(ctx context.Context, channelID int64) ([]Approval, error) {
	rows, err := s.db.QueryContext(ctx,
		"UPDATE approval SET status = ? WHERE channel_id = ? AND status = ? AND kind = ? "+
			"RETURNING "+_approvalCols,
		ApprovalApproved, channelID, ApprovalPending, ApprovalHold)
	if err != nil {
		return nil, fmt.Errorf("claim holds: %w", err)
	}
	as, err := scanAll(rows, scanApproval)
	if err != nil {
		return nil, fmt.Errorf("claim holds: %w", err)
	}
	slices.SortFunc(as, func(a, b Approval) int { return cmp.Compare(a.ID, b.ID) })
	return as, nil
}

// ReopenApproval sets a decided approval back to pending, for a wake that could not start.
func (s *Store) ReopenApproval(ctx context.Context, id int64) error {
	if _, err := s.db.ExecContext(ctx,
		"UPDATE approval SET status = ? WHERE id = ?", ApprovalPending, id); err != nil {
		return fmt.Errorf("reopen approval: %w", err)
	}
	return nil
}

func scanApproval(r interface{ Scan(...any) error }) (Approval, error) {
	var a Approval
	var created string
	if err := r.Scan(&a.ID, &a.ChannelID, &a.From, &a.To, &a.MessageID, &a.Text, &a.Parent,
		&a.Status, &a.Kind, &created); err != nil {
		return Approval{}, err
	}
	t, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return Approval{}, fmt.Errorf("approval %d: bad created_at: %w", a.ID, err)
	}
	a.CreatedAt = t
	return a, nil
}
