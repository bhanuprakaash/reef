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

const _interruptedError = "interrupted"

// _retryable is true for a turn t that retry may run again: it failed, was not retried, kept
// a wake, and no later turn of its role ran in the channel.
const _retryable = `t.status = '` + _turnFailed + `' AND t.retried = 0 AND t.wake_text IS NOT NULL
	AND NOT EXISTS (SELECT 1 FROM turn n WHERE n.channel_id = t.channel_id AND n.role = t.role AND n.id > t.id)`

const (
	_restartError  = "server restarted"
	_restartAdvice = "The worktree may be dirty; retry or roll back."
)

const (
	_turnCols = "id, channel_id, role, backend, session_id, status, error, " +
		"input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, started_at, ended_at, " +
		"wake_text, wake_parent, retried, checkpoint, sent, model, cost_usd, prompt, direct_message_id, reply, " +
		"session_cost_usd, session_input_tokens, session_output_tokens, session_cache_read_tokens, " +
		"session_cache_write_tokens, scopes, compacted"
)

// Turn is one agent run in a channel. It starts running and ends once, as
// done or failed. There is no delete. WakeText and WakeParent are the prompt the turn ran
// and the message its reply answers; WakeText is empty for a turn that kept none. Retried is
// set once a failed turn has been run again. Sent is true once the turn's prompt reached the
// agent; false for a turn that failed before then.
type Turn struct {
	ID         int64       `json:"id"`
	ChannelID  int64       `json:"channelId"`
	Role       string      `json:"role"`
	Backend    string      `json:"backend"`
	SessionID  string      `json:"sessionId"`
	Status     string      `json:"status"`
	Error      string      `json:"error"`
	Usage      agent.Usage `json:"usage"`
	StartedAt  time.Time   `json:"startedAt"`
	EndedAt    *time.Time  `json:"endedAt"`
	WakeText   string      `json:"wakeText"`
	WakeParent *int64      `json:"wakeParent"`
	// DirectMessageID is the private wake this turn answers, nil for a public wake.
	DirectMessageID *int64 `json:"directMessageId"`
	Retried         bool   `json:"retried"`
	Sent            bool   `json:"sent"`
	// Checkpoint is the git ref of the snapshot taken before the turn ran, nil for a
	// turn that has none.
	Checkpoint *string `json:"checkpoint"`
	// Model is the backend's model name, set from the agent's first event. Empty until then.
	Model string `json:"model"`
	// CostUSD is nil until the agent reports one; codex turns never get one.
	CostUSD *float64 `json:"costUsd"`
	// Prompt is the full text sent to the agent, set once assembled. Empty for a turn that
	// never sent one. Hidden from Turn's own JSON; the inspector has its own view of it.
	Prompt string `json:"-"`
	// Reply is the plain reply a done turn kept off the channel because the turn already
	// posted with post_to_channel. Empty otherwise. Hidden from Turn's own JSON; the inspector
	// has its own view of it.
	Reply string `json:"-"`
	// SessionCostUSD and SessionUsage are the running totals of the turn's whole session as the
	// backend reported them when the turn ended; nil for a turn from before they were kept, a
	// turn that ended with no report and every codex turn. The turn's own CostUSD and Usage are
	// these minus the totals its session's earlier turns reported. Hidden from Turn's own JSON.
	SessionCostUSD *float64     `json:"-"`
	SessionUsage   *agent.Usage `json:"-"`
	// Scopes are the threads the turn's wake was routed in. Once the turn sent, each carries
	// the newest message it sent, so a later turn of the session skips what it read. Nil for
	// a turn that is not a routed wake, and for one from before scopes: it read nothing.
	// Hidden from Turn's own JSON.
	Scopes    Scopes `json:"-"`
	Compacted bool   `json:"-"`
}

// TurnFilter narrows Turns. A nil ChannelID or empty Status matches everything. Retryable
// keeps the turns Retryable reports true for.
type TurnFilter struct {
	ChannelID *int64
	Status    string
	Retryable bool
}

// CreateTurn starts a running turn; any status in t is ignored, and so are Retried and Sent.
func (s *Store) CreateTurn(ctx context.Context, t Turn) (Turn, error) {
	if t.Role == "" {
		return Turn{}, fmt.Errorf("%w: turn role is required", ErrInvalid)
	}
	if !isBackend(t.Backend) {
		return Turn{}, fmt.Errorf("%w: unknown backend %q", ErrInvalid, t.Backend)
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO turn (channel_id, role, backend, session_id, status, started_at, "+
			"wake_text, wake_parent, direct_message_id, scopes) "+
			"VALUES (?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?)",
		t.ChannelID, t.Role, t.Backend, t.SessionID, _turnRunning, now.Format(time.RFC3339Nano),
		t.WakeText, t.WakeParent, t.DirectMessageID, t.Scopes)
	if err != nil {
		return Turn{}, s.writeErr(ctx, "insert turn", err, ErrInvalid,
			parent{table: "channel", id: &t.ChannelID}, parent{table: "message", id: t.WakeParent},
			parent{table: "direct_message", id: t.DirectMessageID})
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Turn{}, fmt.Errorf("insert turn: %w", err)
	}
	return Turn{
		ID: id, ChannelID: t.ChannelID, Role: t.Role, Backend: t.Backend,
		SessionID: t.SessionID, Status: _turnRunning, StartedAt: now,
		WakeText: t.WakeText, WakeParent: t.WakeParent, DirectMessageID: t.DirectMessageID,
		Scopes: t.Scopes,
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
	q, args := "SELECT "+_turnCols+" FROM turn t WHERE 1 = 1", []any{}
	if f.ChannelID != nil {
		q, args = q+" AND channel_id = ?", append(args, *f.ChannelID)
	}
	if f.Status != "" {
		q, args = q+" AND status = ?", append(args, f.Status)
	}
	if f.Retryable {
		q += " AND " + _retryable
	}
	rows, err := s.db.QueryContext(ctx, q+" ORDER BY id", args...)
	if err != nil {
		return nil, fmt.Errorf("list turns: %w", err)
	}
	return scanAll(rows, scanTurn)
}

// LastSentAt is when the newest turn of role in the session of channelID that sent its prompt
// started. It is false when no turn of the session sent one.
func (s *Store) LastSentAt(
	ctx context.Context, channelID int64, role, sessionID string,
) (time.Time, bool, error) {
	var started string
	err := s.db.QueryRowContext(ctx,
		"SELECT started_at FROM turn WHERE channel_id = ? AND role = ? AND session_id = ? "+
			"AND sent = 1 ORDER BY id DESC LIMIT 1", channelID, role, sessionID).Scan(&started)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("last sent turn: %w", err)
	}
	t, err := time.Parse(time.RFC3339Nano, started)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("bad started_at: %w", err)
	}
	return t, true, nil
}

// SessionCompacted reports whether the backend compacted the session during or after the
// newest turn of role in the session of channelID that sent its prompt.
func (s *Store) SessionCompacted(
	ctx context.Context, channelID int64, role, sessionID string,
) (bool, error) {
	var compacted bool
	err := s.db.QueryRowContext(ctx,
		"SELECT compacted FROM turn WHERE channel_id = ? AND role = ? AND session_id = ? "+
			"AND sent = 1 ORDER BY id DESC LIMIT 1", channelID, role, sessionID).Scan(&compacted)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("session compacted: %w", err)
	}
	return compacted, nil
}

// SetTurnCompacted records that the backend compacted the session after the newest turn of
// role in the session of channelID that sent its prompt.
func (s *Store) SetTurnCompacted(
	ctx context.Context, channelID int64, role, sessionID string,
) error {
	_, err := s.db.ExecContext(ctx,
		"UPDATE turn SET compacted = 1 WHERE id = (SELECT id FROM turn WHERE channel_id = ? "+
			"AND role = ? AND session_id = ? AND sent = 1 ORDER BY id DESC LIMIT 1)",
		channelID, role, sessionID)
	if err != nil {
		return fmt.Errorf("set turn compacted: %w", err)
	}
	return nil
}

// SessionCostBefore is the running cost of the session with the id when its turns before turn
// beforeID ended: the total the latest of them reported. A session with only turns stored before
// totals were kept has the largest cost, a running total, plus the costs stored after it, which
// are a long-lived process's own shares. 0 for a session with no earlier turn.
func (s *Store) SessionCostBefore(ctx context.Context, sessionID string, beforeID int64) (float64, error) {
	var cost float64
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(
			(SELECT session_cost_usd FROM turn WHERE session_id = ?1 AND id < ?2
				AND session_cost_usd IS NOT NULL ORDER BY id DESC LIMIT 1),
			(SELECT SUM(cost_usd) FROM turn WHERE session_id = ?1 AND id < ?2 AND id >=
				(SELECT id FROM turn WHERE session_id = ?1 AND id < ?2
					ORDER BY cost_usd DESC, id DESC LIMIT 1)), 0)`,
		sessionID, beforeID).Scan(&cost)
	if err != nil {
		return 0, fmt.Errorf("session cost: %w", err)
	}
	return cost, nil
}

// SessionUsageBefore is the usage of the session with the id when its turns before turn
// beforeID ended: the total of the latest turn that reported one, plus the usage stored for the
// turns after it, which reported none, such as a stopped turn. With no turn that reported a
// total it is the usage stored for all of them.
func (s *Store) SessionUsageBefore(ctx context.Context, sessionID string, beforeID int64) (agent.Usage, error) {
	var since int64
	var u agent.Usage
	err := s.db.QueryRowContext(ctx,
		`SELECT id, session_input_tokens, session_output_tokens, session_cache_read_tokens,
		session_cache_write_tokens FROM turn WHERE session_id = ? AND id < ?
		AND session_input_tokens IS NOT NULL ORDER BY id DESC LIMIT 1`, sessionID, beforeID).
		Scan(&since, &u.InputTokens, &u.OutputTokens, &u.CacheReadTokens, &u.CacheWriteTokens)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return agent.Usage{}, fmt.Errorf("session usage: %w", err)
	}
	var rest agent.Usage
	err = s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0),
		COALESCE(SUM(cache_read_tokens), 0), COALESCE(SUM(cache_write_tokens), 0)
		FROM turn WHERE session_id = ? AND id > ? AND id < ?`, sessionID, since, beforeID).
		Scan(&rest.InputTokens, &rest.OutputTokens, &rest.CacheReadTokens, &rest.CacheWriteTokens)
	if err != nil {
		return agent.Usage{}, fmt.Errorf("session usage: %w", err)
	}
	u.Add(rest)
	return u, nil
}

// Retryable reports whether retry may run the turn id again: it failed, was not retried, kept
// a wake, and no later turn of its role ran in the channel.
func (s *Store) Retryable(ctx context.Context, id int64) (bool, error) {
	var ok bool
	err := s.db.QueryRowContext(ctx,
		"SELECT EXISTS (SELECT 1 FROM turn t WHERE t.id = ? AND "+_retryable+")", id).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("check turn %d: %w", id, err)
	}
	return ok, nil
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
	var session struct{ input, output, cacheRead, cacheWrite *int64 }
	if t.SessionUsage != nil {
		session.input, session.output = &t.SessionUsage.InputTokens, &t.SessionUsage.OutputTokens
		session.cacheRead, session.cacheWrite = &t.SessionUsage.CacheReadTokens, &t.SessionUsage.CacheWriteTokens
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE turn SET status = ?, error = ?, input_tokens = ?, output_tokens = ?,
		cache_read_tokens = ?, cache_write_tokens = ?,
		session_id = COALESCE(NULLIF(?, ''), session_id), ended_at = ?,
		model = ?, cost_usd = ?, prompt = ?, reply = ?, session_cost_usd = ?,
		session_input_tokens = ?, session_output_tokens = ?, session_cache_read_tokens = ?,
		session_cache_write_tokens = ?, compacted = ?
		WHERE id = ? AND status = ?`,
		t.Status, t.Error, t.Usage.InputTokens, t.Usage.OutputTokens,
		t.Usage.CacheReadTokens, t.Usage.CacheWriteTokens, t.SessionID,
		time.Now().UTC().Format(time.RFC3339Nano), t.Model, t.CostUSD, t.Prompt, t.Reply,
		t.SessionCostUSD, session.input, session.output, session.cacheRead, session.cacheWrite,
		t.Compacted, t.ID, _turnRunning)
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

// SetTurnRetried marks the failed turn id as retried, or, with retried false, as not retried
// again. A turn that is not failed, or is already in that state, is ErrConflict.
func (s *Store) SetTurnRetried(ctx context.Context, id int64, retried bool) error {
	res, err := s.db.ExecContext(ctx,
		"UPDATE turn SET retried = ? WHERE id = ? AND status = ? AND retried = ?",
		retried, id, _turnFailed, !retried)
	if err != nil {
		return fmt.Errorf("set turn retried: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("set turn retried: %w", err)
	}
	if n == 0 {
		if _, err := s.Turn(ctx, id); err != nil {
			return err
		}
		return fmt.Errorf("turn %d is not failed, or retried is already %t: %w",
			id, retried, ErrConflict)
	}
	return nil
}

// SetTurnSent marks the running turn id as having sent prompt to the agent, and keeps it.
func (s *Store) SetTurnSent(ctx context.Context, id int64, prompt string) (Turn, error) {
	res, err := s.db.ExecContext(ctx,
		"UPDATE turn SET sent = 1, prompt = ? WHERE id = ? AND status = ?", prompt, id, _turnRunning)
	if err != nil {
		return Turn{}, fmt.Errorf("set turn sent: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return Turn{}, fmt.Errorf("set turn sent: %w", err)
	}
	if n == 0 {
		return Turn{}, s.notRunning(ctx, id)
	}
	return s.Turn(ctx, id)
}

// SetTurnUsage records the usage so far of the running turn id without ending it, so a turn
// that never ends keeps the tokens it used.
func (s *Store) SetTurnUsage(ctx context.Context, id int64, u agent.Usage) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE turn SET input_tokens = ?, output_tokens = ?, cache_read_tokens = ?,
		cache_write_tokens = ? WHERE id = ? AND status = ?`,
		u.InputTokens, u.OutputTokens, u.CacheReadTokens, u.CacheWriteTokens, id, _turnRunning)
	if err != nil {
		return fmt.Errorf("set turn usage: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("set turn usage: %w", err)
	}
	if n == 0 {
		return s.notRunning(ctx, id)
	}
	return nil
}

// SetTurnCheckpoint records the checkpoint ref of the running turn id.
func (s *Store) SetTurnCheckpoint(ctx context.Context, id int64, ref string) (Turn, error) {
	res, err := s.db.ExecContext(ctx,
		"UPDATE turn SET checkpoint = ? WHERE id = ? AND status = ?", ref, id, _turnRunning)
	if err != nil {
		return Turn{}, fmt.Errorf("set turn checkpoint: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return Turn{}, fmt.Errorf("set turn checkpoint: %w", err)
	}
	if n == 0 {
		return Turn{}, s.notRunning(ctx, id)
	}
	return s.Turn(ctx, id)
}

// FailRunningTurns marks every running turn failed, for turns the last server
// run left unfinished, and posts one message per affected channel naming them.
// Turns of a channel for which keep returns true stay running; keep may be nil.
func (s *Store) FailRunningTurns(ctx context.Context, keep func(channelID int64) bool) ([]Turn, error) {
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
	running, err := scanAll(rows, scanTurn)
	if err != nil {
		return nil, err
	}
	var turns []Turn
	for _, t := range running {
		if keep == nil || !keep(t.ChannelID) {
			turns = append(turns, t)
		}
	}
	if len(turns) == 0 {
		return nil, nil
	}

	now := time.Now().UTC()
	stamp := now.Format(time.RFC3339Nano)
	var names []string
	for i := range turns {
		t := &turns[i]
		if _, err := tx.ExecContext(ctx,
			"UPDATE turn SET status = ?, error = ?, ended_at = ? WHERE id = ?",
			_turnFailed, _restartError, stamp, t.ID); err != nil {
			return nil, fmt.Errorf("fail running turns: %w", err)
		}
		t.Status, t.Error, t.EndedAt = _turnFailed, _restartError, &now
		if _, err := tx.ExecContext(ctx, "UPDATE channel SET active_turn = '' WHERE id = ?", t.ChannelID); err != nil {
			return nil, fmt.Errorf("fail running turns: %w", err)
		}
		if t.DirectMessageID != nil {
			// The turn was woken by a direct message. If that message is the original
			// request (no reply_to), leave a failed response so the sender sees that
			// delivery did not complete. A missing original message is not an error:
			// there is nothing to answer.
			var replyTo sql.NullInt64
			scanErr := tx.QueryRowContext(ctx,
				"SELECT reply_to_id FROM direct_message WHERE id = ?",
				*t.DirectMessageID).Scan(&replyTo)
			switch {
			case errors.Is(scanErr, sql.ErrNoRows):
				// missing original direct message: nothing to respond to.
			case scanErr != nil:
				return nil, fmt.Errorf("fail running turns: %w", scanErr)
			case !replyTo.Valid:
				if _, err := s.CreateFailedDirectResponseTx(ctx, tx, *t.DirectMessageID,
					t.Role, ""); err != nil {
					return nil, fmt.Errorf("fail running turns: %w", err)
				}
			}
		}
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
			t.ChannelID, ReefAuthor, body, stamp); err != nil {
			return nil, fmt.Errorf("insert restart message: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("fail running turns: %w", err)
	}
	return turns, nil
}

func (s *Store) notRunning(ctx context.Context, id int64) error {
	if _, err := s.Turn(ctx, id); err != nil {
		return err
	}
	return fmt.Errorf("%w: turn %d already ended", ErrInvalid, id)
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
	var ended, wake, checkpoint sql.NullString
	var session struct{ input, output, cacheRead, cacheWrite *int64 }
	if err := r.Scan(&t.ID, &t.ChannelID, &t.Role, &t.Backend, &t.SessionID, &t.Status, &t.Error,
		&t.Usage.InputTokens, &t.Usage.OutputTokens, &t.Usage.CacheReadTokens,
		&t.Usage.CacheWriteTokens, &started, &ended, &wake, &t.WakeParent, &t.Retried,
		&checkpoint, &t.Sent, &t.Model, &t.CostUSD, &t.Prompt, &t.DirectMessageID, &t.Reply,
		&t.SessionCostUSD, &session.input, &session.output, &session.cacheRead, &session.cacheWrite,
		&t.Scopes, &t.Compacted); err != nil {
		return Turn{}, err
	}
	if session.input != nil {
		t.SessionUsage = &agent.Usage{
			InputTokens: *session.input, OutputTokens: *session.output,
			CacheReadTokens: *session.cacheRead, CacheWriteTokens: *session.cacheWrite,
		}
	}
	t.WakeText = wake.String
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
	if checkpoint.Valid {
		t.Checkpoint = &checkpoint.String
	}
	return t, nil
}
