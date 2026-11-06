package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// CancelledPrefix starts the body of the failed response a request gets when its recipient left
// the Space, and gives the request the cancelled state.
const CancelledPrefix = "cancelled: "

// RemoveSpaceMember takes the Bot botID out of Space id in one transaction: its member row, its
// active BotSession and its pending holds go, every request still waiting for it gets a failed
// response and the cancelled state, and the Space gets a notice that it left. A Bot that leads
// needs coordinator, a remaining member, to lead in its place; the last Bot cannot leave.
func (s *Store) RemoveSpaceMember(ctx context.Context, id, botID, coordinator int64) error {
	c, err := s.Space(ctx, id)
	if err != nil {
		return err
	}
	if c.Kind == KindConversation || c.Kind == KindConsultation {
		return fmt.Errorf("%w: Space %d is a %s with one Bot", ErrInvalid, id, c.Kind)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin remove member: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var name string
	var lead bool
	err = tx.QueryRowContext(ctx,
		"SELECT bot.name, member.lead FROM space_member member JOIN bot ON bot.id = member.bot_id "+
			"WHERE member.space_id = ? AND member.bot_id = ?", id, botID).Scan(&name, &lead)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: bot %d is not a member of Space %d", ErrNotFound, botID, id)
	}
	if err != nil {
		return fmt.Errorf("read member: %w", err)
	}
	var others int
	if err := tx.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM space_member WHERE space_id = ? AND bot_id != ?", id, botID).Scan(&others); err != nil {
		return fmt.Errorf("count members: %w", err)
	}
	if others == 0 {
		return fmt.Errorf("%w: %s is the last Bot of Space %d; name a replacement", ErrConflict, name, id)
	}
	if _, err := tx.ExecContext(ctx,
		"DELETE FROM space_member WHERE space_id = ? AND bot_id = ?", id, botID); err != nil {
		return fmt.Errorf("delete member: %w", err)
	}
	if lead {
		res, err := tx.ExecContext(ctx,
			"UPDATE space_member SET lead = (bot_id = ?) WHERE space_id = ? "+
				"AND EXISTS (SELECT 1 FROM space_member WHERE space_id = ? AND bot_id = ?)",
			coordinator, id, id, coordinator)
		if err != nil {
			return fmt.Errorf("set coordinator: %w", err)
		}
		if n, err := res.RowsAffected(); err == nil && n == 0 {
			return fmt.Errorf("%w: %s coordinates Space %d; name a remaining member as coordinator",
				ErrConflict, name, id)
		}
	}
	rows, err := tx.QueryContext(ctx,
		"SELECT id FROM direct_message WHERE space_id = ? AND to_bot_id = ? AND reply_to_id IS NULL "+
			"AND state = ? ORDER BY id", id, botID, DirectWaiting)
	if err != nil {
		return fmt.Errorf("list waiting requests: %w", err)
	}
	waiting, err := scanAll(rows, func(r interface{ Scan(...any) error }) (id int64, err error) {
		return id, r.Scan(&id)
	})
	if err != nil {
		return fmt.Errorf("list waiting requests: %w", err)
	}
	for _, rid := range waiting {
		body := CancelledPrefix + name + " was removed from the Space"
		if _, err := s.CreateFailedDirectResponseTx(ctx, tx, rid, &botID, name, body); err != nil {
			return fmt.Errorf("cancel request %d: %w", rid, err)
		}
	}
	if _, err := tx.ExecContext(ctx,
		"UPDATE approval SET status = ? WHERE space_id = ? AND to_bot_id = ? AND status = ? AND kind = ?",
		HoldDropped, id, botID, HoldPending, _holdKind); err != nil {
		return fmt.Errorf("drop holds: %w", err)
	}
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx,
		"UPDATE bot_session SET status = ?, ended_at = ?, refresh_pending = 0 "+
			"WHERE space_id = ? AND bot_id = ? AND status = ?",
		BotSessionRevoked, now.Format(time.RFC3339Nano), id, botID, BotSessionActive); err != nil {
		return fmt.Errorf("end bot session: %w", err)
	}
	if _, err := s.insertMessage(ctx, tx,
		Message{SpaceID: id, Author: ReefAuthor, Body: name + LeftSuffix}, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit remove member: %w", err)
	}
	return nil
}

// PendingRemoval is a Bot marked to leave Space SpaceID once the turn that is running ends, and
// the member that leads in its place when it led.
type PendingRemoval struct {
	SpaceID     int64
	BotID       int64
	Coordinator int64
}

// MarkMemberLeaving records that the Bot leaves Space id when its running turn ends. The mark
// survives a restart; ApplyMemberRemoval ends it.
func (s *Store) MarkMemberLeaving(ctx context.Context, id, botID, coordinator int64) error {
	res, err := s.db.ExecContext(ctx,
		"UPDATE space_member SET leaving = 1, leaving_coordinator = ? WHERE space_id = ? AND bot_id = ?",
		coordinator, id, botID)
	if err != nil {
		return fmt.Errorf("mark member leaving: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return fmt.Errorf("%w: bot %d is not a member of Space %d", ErrNotFound, botID, id)
	}
	return nil
}

// LeavingMembers lists the removals still pending in Space id, or in every Space when id is 0.
func (s *Store) LeavingMembers(ctx context.Context, id int64) ([]PendingRemoval, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT space_id, bot_id, leaving_coordinator FROM space_member "+
			"WHERE leaving = 1 AND (? = 0 OR space_id = ?) ORDER BY id", id, id)
	if err != nil {
		return nil, fmt.Errorf("list leaving members: %w", err)
	}
	return scanAll(rows, func(r interface{ Scan(...any) error }) (p PendingRemoval, err error) {
		return p, r.Scan(&p.SpaceID, &p.BotID, &p.Coordinator)
	})
}

// ApplyMemberRemoval removes the Bot of a pending removal. A removal that no longer holds
// leaves the Bot a member that is not leaving, and returns why.
func (s *Store) ApplyMemberRemoval(ctx context.Context, p PendingRemoval) error {
	err := s.RemoveSpaceMember(ctx, p.SpaceID, p.BotID, p.Coordinator)
	if err != nil && !errors.Is(err, ErrNotFound) {
		if _, cerr := s.db.ExecContext(ctx,
			"UPDATE space_member SET leaving = 0, leaving_coordinator = 0 WHERE space_id = ? AND bot_id = ?",
			p.SpaceID, p.BotID); cerr != nil {
			return errors.Join(err, cerr)
		}
	}
	return err
}
