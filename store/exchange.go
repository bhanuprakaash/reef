package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// An exchange is a direct request that lives in its destination Space and records the
// conversation it came from. Its reply is the request's response row, also posted as a
// visible message in the origin conversation.
const (
	ExchangeRequest  = "request"
	ExchangeQuestion = "question"
	ExchangeGuidance = "guidance"
)

// ErrNotWaiting is wrapped when a turn is started for an exchange that is not waiting, so an
// exchange never runs twice.
var ErrNotWaiting = fmt.Errorf("exchange is not waiting: %w", ErrConflict)

// ExchangeInterrupted is the body of the failed response an exchange gets when its turn was
// cut while running: what it did is unknown, so it is not replayed.
const ExchangeInterrupted = "interrupted while running; check its activity before retrying"

// Exchange is a request with its newest response, nil while there is none.
type Exchange struct {
	Request  DirectMessage  `json:"request"`
	Response *DirectMessage `json:"response"`
}

// IsUserGuidance reports whether m is guidance the user wrote, which runs ahead of the Bot
// handoffs waiting in its Space.
func (m DirectMessage) IsUserGuidance() bool {
	return m.Kind == ExchangeGuidance && m.FromBotID == nil && m.OriginMessageID != nil
}

// CreateExchange stores request m in its destination Space m.SpaceID, with the conversation
// m.OriginSpaceID it came from, and posts originNote there as a message linked to it, written by the sender, or by Reef for a Bot. A key
// already stored returns the first row and false, and starts nothing. The sender may be nil
// for a request from the user.
func (s *Store) CreateExchange(
	ctx context.Context, m DirectMessage, originNote string,
) (DirectMessage, bool, error) {
	if m.Kind == "" {
		m.Kind = ExchangeRequest
	}
	switch {
	case m.OriginSpaceID == nil || m.ToBotID == nil || m.From == "" || m.To == "":
		return DirectMessage{}, false, fmt.Errorf("%w: exchange origin, sender and recipient are required", ErrInvalid)
	case strings.TrimSpace(m.Body) == "":
		return DirectMessage{}, false, fmt.Errorf("%w: exchange body is required", ErrInvalid)
	case m.RequestKey != nil && *m.RequestKey == "":
		return DirectMessage{}, false, fmt.Errorf("%w: exchange request key is empty", ErrInvalid)
	}
	if m.RequestKey != nil {
		if first, err := s.exchangeByKey(ctx, *m.RequestKey); err == nil {
			return first, false, nil
		} else if !errors.Is(err, ErrNotFound) {
			return DirectMessage{}, false, err
		}
	}
	now := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return DirectMessage{}, false, fmt.Errorf("begin exchange: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx,
		`INSERT INTO direct_message (space_id, source_message_id, from_role, to_role, body, status, state,
		created_at, from_bot_id, to_bot_id, origin_space_id, origin_message_id, kind, request_key)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.SpaceID, m.SourceMessageID, m.From, m.To, m.Body, DirectSent, DirectWaiting,
		now.Format(time.RFC3339Nano), m.FromBotID, m.ToBotID, m.OriginSpaceID, m.OriginMessageID,
		m.Kind, m.RequestKey)
	if err != nil {
		if m.RequestKey != nil && isUnique(err) {
			_ = tx.Rollback()
			first, ferr := s.exchangeByKey(ctx, *m.RequestKey)
			return first, false, ferr
		}
		return DirectMessage{}, false, s.writeErr(ctx, "insert exchange", err, ErrInvalid,
			parent{table: "space", id: &m.SpaceID}, parent{table: "space", id: m.OriginSpaceID})
	}
	id, err := res.LastInsertId()
	if err != nil {
		return DirectMessage{}, false, fmt.Errorf("insert exchange: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE direct_message SET chain_id = ? WHERE id = ?", id, id); err != nil {
		return DirectMessage{}, false, fmt.Errorf("set exchange chain: %w", err)
	}
	if originNote != "" {
		author := ReefAuthor
		if m.FromBotID == nil {
			author = m.From
		}
		note, err := s.insertMessage(ctx, tx, Message{
			SpaceID: *m.OriginSpaceID, Author: author, Body: originNote, ExchangeID: &id}, now)
		if err != nil {
			return DirectMessage{}, false, err
		}
		if _, err := tx.ExecContext(ctx,
			"UPDATE direct_message SET origin_message_id = ? WHERE id = ?", note.ID, id); err != nil {
			return DirectMessage{}, false, fmt.Errorf("set exchange origin message: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return DirectMessage{}, false, fmt.Errorf("commit exchange: %w", err)
	}
	created, err := s.DirectMessage(ctx, id)
	return created, true, err
}

func (s *Store) exchangeByKey(ctx context.Context, key string) (DirectMessage, error) {
	m, err := scanDirect(s.db.QueryRowContext(ctx,
		"SELECT "+_directCols+" FROM direct_message WHERE request_key = ?", key))
	if errors.Is(err, sql.ErrNoRows) {
		return DirectMessage{}, fmt.Errorf("exchange %q: %w", key, ErrNotFound)
	}
	return m, err
}

// Exchange returns the exchange with the request id: the request and its newest response.
func (s *Store) Exchange(ctx context.Context, id int64) (Exchange, error) {
	req, err := s.DirectMessage(ctx, id)
	if err != nil {
		return Exchange{}, err
	}
	if req.ReplyToID != nil || req.OriginSpaceID == nil {
		return Exchange{}, fmt.Errorf("exchange %d: %w", id, ErrNotFound)
	}
	ex := Exchange{Request: req}
	resp, err := scanDirect(s.db.QueryRowContext(ctx,
		"SELECT "+_directCols+" FROM direct_message WHERE reply_to_id = ? ORDER BY id DESC LIMIT 1", id))
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return Exchange{}, err
	default:
		ex.Response = &resp
	}
	return ex, nil
}

// SpaceExchanges lists the exchange requests stored in the Space and those that came from it,
// oldest first.
func (s *Store) SpaceExchanges(ctx context.Context, spaceID int64) ([]DirectMessage, error) {
	if err := s.requireSpace(ctx, spaceID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, "SELECT "+_directCols+
		" FROM direct_message WHERE reply_to_id IS NULL AND origin_space_id IS NOT NULL"+
		" AND (space_id = ? OR origin_space_id = ?) ORDER BY id", spaceID, spaceID)
	if err != nil {
		return nil, fmt.Errorf("list exchanges: %w", err)
	}
	return scanAll(rows, scanDirect)
}

// WaitingExchanges lists the exchange requests of the Space that no turn has started for.
func (s *Store) WaitingExchanges(ctx context.Context, spaceID int64) ([]DirectMessage, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+_directCols+
		" FROM direct_message WHERE space_id = ? AND reply_to_id IS NULL AND origin_space_id IS NOT NULL"+
		" AND state = ? ORDER BY id", spaceID, DirectWaiting)
	if err != nil {
		return nil, fmt.Errorf("list waiting exchanges: %w", err)
	}
	return scanAll(rows, scanDirect)
}

// RetryExchange moves failed exchange id back to waiting. Of two calls only one succeeds, and
// an exchange that has a done response or is in any other state is refused.
func (s *Store) RetryExchange(ctx context.Context, id int64) (DirectMessage, error) {
	return s.moveExchange(ctx, id, DirectFailed, DirectWaiting,
		" AND NOT EXISTS (SELECT 1 FROM direct_message r WHERE r.reply_to_id = direct_message.id AND r.status = 'done')")
}

// CancelExchange withdraws exchange id while it waits.
func (s *Store) CancelExchange(ctx context.Context, id int64) (DirectMessage, error) {
	return s.moveExchange(ctx, id, DirectWaiting, DirectCancelled, "")
}

func (s *Store) moveExchange(
	ctx context.Context, id int64, from, to, extra string,
) (DirectMessage, error) {
	res, err := s.db.ExecContext(ctx,
		"UPDATE direct_message SET state = ? WHERE id = ? AND state = ? AND reply_to_id IS NULL"+
			" AND origin_space_id IS NOT NULL"+extra, to, id, from)
	if err != nil {
		return DirectMessage{}, fmt.Errorf("move exchange %d to %s: %w", id, to, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return DirectMessage{}, fmt.Errorf("move exchange %d to %s: %w", id, to, err)
	}
	ex, err := s.Exchange(ctx, id)
	if err != nil {
		return DirectMessage{}, err
	}
	if n == 0 {
		return DirectMessage{}, fmt.Errorf("exchange %d is %s, not %s: %w", id, ex.Request.State, from, ErrConflict)
	}
	return ex.Request, nil
}

// RestartReason is the pause reason of a Space that kept wakes waiting across a restart.
const RestartReason = "app restarted: wakes were waiting to run"

// PauseWaitingExchangeSpaces pauses every open Space that holds a waiting exchange, so continue
// delivers it after a restart.
func (s *Store) PauseWaitingExchangeSpaces(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT d.space_id FROM direct_message d JOIN space c ON c.id = d.space_id
		WHERE d.reply_to_id IS NULL AND d.origin_space_id IS NOT NULL AND d.state = ?
		AND c.paused = '' AND c.closed_at IS NULL ORDER BY d.space_id`, DirectWaiting)
	if err != nil {
		return fmt.Errorf("list Spaces with waiting exchanges: %w", err)
	}
	ids, err := scanAll(rows, func(r interface{ Scan(...any) error }) (int64, error) {
		var id int64
		return id, r.Scan(&id)
	})
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := s.PauseSpace(ctx, id, RestartReason); err != nil {
			return err
		}
	}
	return nil
}

// claimExchange moves the exchange request id from waiting to delivered inside the tx that
// creates its turn. A request that is not an exchange is left alone.
func claimExchange(ctx context.Context, tx *sql.Tx, id int64) error {
	res, err := tx.ExecContext(ctx,
		`UPDATE direct_message SET state = ? WHERE id = ? AND reply_to_id IS NULL
		AND origin_space_id IS NOT NULL AND state = ?`, DirectDelivered, id, DirectWaiting)
	if err != nil {
		return fmt.Errorf("claim exchange %d: %w", id, err)
	}
	if n, err := res.RowsAffected(); err != nil || n > 0 {
		return err
	}
	var state string
	err = tx.QueryRowContext(ctx,
		`SELECT state FROM direct_message WHERE id = ? AND reply_to_id IS NULL
		AND origin_space_id IS NOT NULL`, id).Scan(&state)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil
	case err != nil:
		return fmt.Errorf("claim exchange %d: %w", id, err)
	}
	return fmt.Errorf("exchange %d is %s: %w", id, state, ErrNotWaiting)
}

// postExchangeReply posts the response to an exchange as a message in its origin conversation.
func (s *Store) postExchangeReply(
	ctx context.Context, db execer, request, response DirectMessage, state string,
) error {
	if request.OriginSpaceID == nil || *request.OriginSpaceID == request.SpaceID {
		return nil
	}
	body := response.Body
	if response.Status == DirectFailed {
		body = "No reply: " + state
		if response.Body != "" && response.Body != state {
			body += ", " + response.Body
		}
	}
	_, err := s.insertMessage(ctx, db, Message{
		SpaceID: *request.OriginSpaceID, Author: response.From, BotID: response.FromBotID,
		Body: body, ExchangeID: &request.ID}, response.CreatedAt)
	return err
}
