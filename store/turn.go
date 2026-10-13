package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bhanuprakaash/reef/agent"
)

const (
	_turnRunning = "running"
	_turnDone    = "done"
	_turnFailed  = "failed"
)

const (
	_restartError  = "server restarted"
	_restartAuthor = "reef"
	_restartAdvice = "The worktree may be dirty; retry or roll back."
)

const (
	_turnCols = "id, channel_id, role, backend, session_id, status, error, " +
		"input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, started_at, ended_at"
)

// Turn is one agent run in a channel. It starts running and ends once, as
// done or failed. There is no delete.
type Turn struct {
	ID        int64       `json:"id"`
	ChannelID int64       `json:"channelId"`
	Role      string      `json:"role"`
	Backend   string      `json:"backend"`
	SessionID string      `json:"sessionId"`
	Status    string      `json:"status"`
	Error     string      `json:"error"`
	Usage     agent.Usage `json:"usage"`
	StartedAt time.Time   `json:"startedAt"`
	EndedAt   *time.Time  `json:"endedAt"`
}

// TurnFilter narrows Turns. A nil ChannelID or empty Status matches everything.
type TurnFilter struct {
	ChannelID *int64
	Status    string
}

// CreateTurn starts a running turn; any status in t is ignored.
func (s *Store) CreateTurn(ctx context.Context, t Turn) (Turn, error) {
	if t.Role == "" {
		return Turn{}, fmt.Errorf("%w: turn role is required", ErrInvalid)
	}
	if !isBackend(t.Backend) {
		return Turn{}, fmt.Errorf("%w: unknown backend %q", ErrInvalid, t.Backend)
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO turn (channel_id, role, backend, session_id, status, started_at) "+
			"VALUES (?, ?, ?, ?, ?, ?)",
		t.ChannelID, t.Role, t.Backend, t.SessionID, _turnRunning, now.Format(time.RFC3339Nano))
	if err != nil {
		return Turn{}, s.writeErr(ctx, "insert turn", err, ErrInvalid,
			parent{"channel", &t.ChannelID})
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Turn{}, fmt.Errorf("insert turn: %w", err)
	}
	return Turn{
		ID: id, ChannelID: t.ChannelID, Role: t.Role, Backend: t.Backend,
		SessionID: t.SessionID, Status: _turnRunning, StartedAt: now,
	}, nil
}

// Turn returns the turn with the id, or ErrNotFound.
func (s *Store) Turn(ctx context.Context, id int64) (Turn, error) {
	row := s.db.QueryRowContext(ctx, "SELECT "+_turnCols+" FROM turn WHERE id = ?", id)
	t, err := scanTurn(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Turn{}, fmt.Errorf("turn %d: %w", id, ErrNotFound)
	}
	return t, err
}

// Turns lists turns by id, narrowed by f.
func (s *Store) Turns(ctx context.Context, f TurnFilter) ([]Turn, error) {
	if f.Status != "" && !isTurnStatus(f.Status) {
		return nil, fmt.Errorf("%w: unknown status %q", ErrInvalid, f.Status)
	}
	q, args := "SELECT "+_turnCols+" FROM turn WHERE 1 = 1", []any{}
	if f.ChannelID != nil {
		q, args = q+" AND channel_id = ?", append(args, *f.ChannelID)
	}
	if f.Status != "" {
		q, args = q+" AND status = ?", append(args, f.Status)
	}
	rows, err := s.db.QueryContext(ctx, q+" ORDER BY id", args...)
	if err != nil {
		return nil, fmt.Errorf("list turns: %w", err)
	}
	return scanAll(rows, scanTurn)
}

// EndTurn moves the running turn t.ID to t.Status (done or failed) and
// records t.Error, t.Usage and, when set, t.SessionID.
func (s *Store) EndTurn(ctx context.Context, t Turn) (Turn, error) {
	if t.Status != _turnDone && t.Status != _turnFailed {
		return Turn{}, fmt.Errorf("%w: turn status must be %s or %s, got %q",
			ErrInvalid, _turnDone, _turnFailed, t.Status)
	}
	u := t.Usage
	if u.InputTokens < 0 || u.OutputTokens < 0 || u.CacheReadTokens < 0 || u.CacheWriteTokens < 0 {
		return Turn{}, fmt.Errorf("%w: usage counts cannot be negative", ErrInvalid)
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE turn SET status = ?, error = ?, input_tokens = ?, output_tokens = ?,
		cache_read_tokens = ?, cache_write_tokens = ?,
		session_id = COALESCE(NULLIF(?, ''), session_id), ended_at = ?
		WHERE id = ? AND status = ?`,
		t.Status, t.Error, t.Usage.InputTokens, t.Usage.OutputTokens,
		t.Usage.CacheReadTokens, t.Usage.CacheWriteTokens, t.SessionID,
		time.Now().UTC().Format(time.RFC3339Nano), t.ID, _turnRunning)
	if err != nil {
		return Turn{}, fmt.Errorf("end turn: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return Turn{}, fmt.Errorf("end turn: %w", err)
	}
	if n == 0 {
		return Turn{}, s.notRunning(ctx, t.ID)
	}
	return s.Turn(ctx, t.ID)
}

// SetTurnSession records the session id of the running turn id without ending it.
func (s *Store) SetTurnSession(ctx context.Context, id int64, sessionID string) (Turn, error) {
	if sessionID == "" {
		return Turn{}, fmt.Errorf("%w: sessionId is required without a status", ErrInvalid)
	}
	res, err := s.db.ExecContext(ctx,
		"UPDATE turn SET session_id = ? WHERE id = ? AND status = ?", sessionID, id, _turnRunning)
	if err != nil {
		return Turn{}, fmt.Errorf("set turn session: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return Turn{}, fmt.Errorf("set turn session: %w", err)
	}
	if n == 0 {
		return Turn{}, s.notRunning(ctx, id)
	}
	return s.Turn(ctx, id)
}

// notRunning explains why no running turn id was updated.
func (s *Store) notRunning(ctx context.Context, id int64) error {
	if _, err := s.Turn(ctx, id); err != nil {
		return err
	}
	return fmt.Errorf("%w: turn %d already ended", ErrInvalid, id)
}

// FailRunningTurns marks every running turn failed, for turns the last server
// run left unfinished, and posts one message per affected channel naming them.
func (s *Store) FailRunningTurns(ctx context.Context) ([]Turn, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("fail running turns: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx,
		"SELECT "+_turnCols+" FROM turn WHERE status = ? ORDER BY channel_id, id", _turnRunning)
	if err != nil {
		return nil, fmt.Errorf("list running turns: %w", err)
	}
	turns, err := scanAll(rows, scanTurn)
	if err != nil {
		return nil, err
	}
	if len(turns) == 0 {
		return nil, nil
	}

	now := time.Now().UTC()
	stamp := now.Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx,
		"UPDATE turn SET status = ?, error = ?, ended_at = ? WHERE status = ?",
		_turnFailed, _restartError, stamp, _turnRunning); err != nil {
		return nil, fmt.Errorf("fail running turns: %w", err)
	}

	var names []string
	for i := range turns {
		t := &turns[i]
		t.Status, t.Error, t.EndedAt = _turnFailed, _restartError, &now
		label := "turn"
		if len(names) == 0 {
			label = "Turn"
		}
		names = append(names, fmt.Sprintf("%s %d by %s", label, t.ID, t.Role))
		if i+1 < len(turns) && turns[i+1].ChannelID == t.ChannelID {
			continue
		}
		body := fmt.Sprintf("%s failed: %s. %s",
			strings.Join(names, ", "), _restartError, _restartAdvice)
		names = nil
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO message (channel_id, author, body, created_at) VALUES (?, ?, ?, ?)",
			t.ChannelID, _restartAuthor, body, stamp); err != nil {
			return nil, fmt.Errorf("insert restart message: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("fail running turns: %w", err)
	}
	return turns, nil
}

func isTurnStatus(s string) bool {
	switch s {
	case _turnRunning, _turnDone, _turnFailed:
		return true
	}
	return false
}

func isBackend(s string) bool {
	switch s {
	case "claude", "codex":
		return true
	}
	return false
}

func scanTurn(r interface{ Scan(...any) error }) (Turn, error) {
	var t Turn
	var started string
	var ended sql.NullString
	if err := r.Scan(&t.ID, &t.ChannelID, &t.Role, &t.Backend, &t.SessionID, &t.Status, &t.Error,
		&t.Usage.InputTokens, &t.Usage.OutputTokens, &t.Usage.CacheReadTokens,
		&t.Usage.CacheWriteTokens, &started, &ended); err != nil {
		return Turn{}, err
	}
	var err error
	if t.StartedAt, err = time.Parse(time.RFC3339Nano, started); err != nil {
		return Turn{}, fmt.Errorf("turn %d: bad started_at: %w", t.ID, err)
	}
	if ended.Valid {
		e, err := time.Parse(time.RFC3339Nano, ended.String)
		if err != nil {
			return Turn{}, fmt.Errorf("turn %d: bad ended_at: %w", t.ID, err)
		}
		t.EndedAt = &e
	}
	return t, nil
}
