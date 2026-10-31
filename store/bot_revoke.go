package store

import (
	"context"
	"fmt"
	"time"
)

// _toolStopped is the result of a tool call that had none when its Bot was revoked.
const _toolStopped = "stopped: the Bot was revoked; the call may or may not have finished"

// RevokeBot ends every active BotSession of the Bot as revoked, deletes every token of the Bot and
// gives the tool calls of those sessions that have no result a stopped result, in one transaction.
// It returns the Spaces whose BotSession it ended and how many tokens and tool calls it changed.
func (s *Store) RevokeBot(ctx context.Context, botID int64) (spaces []int64, tokens, tools int64, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("begin revoke bot: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx,
		"SELECT space_id FROM bot_session WHERE bot_id = ? AND status = ? ORDER BY space_id",
		botID, BotSessionActive)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("read bot sessions: %w", err)
	}
	spaces, err = scanAll(rows, func(r interface{ Scan(...any) error }) (id int64, err error) {
		return id, r.Scan(&id)
	})
	if err != nil {
		return nil, 0, 0, fmt.Errorf("read bot sessions: %w", err)
	}
	res, err := tx.ExecContext(ctx,
		"UPDATE tool_call SET result = ? WHERE result = '' AND turn_id IN (SELECT turn.id FROM turn "+
			"JOIN bot_session ON bot_session.id = turn.bot_session_id "+
			"WHERE bot_session.bot_id = ? AND bot_session.status = ?)",
		_toolStopped, botID, BotSessionActive)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("stop tool calls: %w", err)
	}
	if tools, err = res.RowsAffected(); err != nil {
		return nil, 0, 0, fmt.Errorf("stop tool calls: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		"UPDATE bot_session SET status = ?, ended_at = ? WHERE bot_id = ? AND status = ?",
		BotSessionRevoked, time.Now().UTC().Format(time.RFC3339Nano), botID, BotSessionActive); err != nil {
		return nil, 0, 0, fmt.Errorf("revoke bot sessions: %w", err)
	}
	res, err = tx.ExecContext(ctx, "DELETE FROM token WHERE bot_id = ?", botID)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("delete tokens: %w", err)
	}
	if tokens, err = res.RowsAffected(); err != nil {
		return nil, 0, 0, fmt.Errorf("delete tokens: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, 0, 0, fmt.Errorf("commit revoke bot: %w", err)
	}
	return spaces, tokens, tools, nil
}

// BotSessionRevoked reports whether the BotSession with the id was revoked.
func (s *Store) BotSessionRevoked(ctx context.Context, id int64) (bool, error) {
	var revoked bool
	if err := s.db.QueryRowContext(ctx,
		"SELECT status = ? FROM bot_session WHERE id = ?", BotSessionRevoked, id).Scan(&revoked); err != nil {
		return false, fmt.Errorf("read bot session %d: %w", id, err)
	}
	return revoked, nil
}
