package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// DirectMessage is a private message between two roles in one Space. A request starts a
// chain; its one automatic response points back to it and shares its chain.
// SourceMessageID is the public message whose wake the sender was answering when it sent
// the request, nil for a chain that did not start from one; a response keeps it.
type DirectMessage struct {
	ID              int64     `json:"id"`
	SpaceID         int64     `json:"spaceId"`
	ChainID         int64     `json:"chainId"`
	ReplyToID       *int64    `json:"replyToId"`
	SourceMessageID *int64    `json:"sourceMessageId"`
	FromBotID       *int64    `json:"fromBotId,omitempty"`
	ToBotID         *int64    `json:"toBotId,omitempty"`
	From            string    `json:"from"`
	To              string    `json:"to"`
	Body            string    `json:"body"`
	Status          string    `json:"status"`
	State           string    `json:"state,omitempty"`
	CreatedAt       time.Time `json:"createdAt"`
	// OriginSpaceID and OriginMessageID name the conversation an exchange came from and the
	// visible message there that stands for it; nil for a request inside its own Space.
	OriginSpaceID   *int64 `json:"originSpaceId,omitempty"`
	OriginMessageID *int64 `json:"originMessageId,omitempty"`
	Kind            string `json:"kind,omitempty"`
	// RequestKey makes creating an exchange idempotent: a second create with the same key
	// returns the first row.
	RequestKey *string `json:"requestKey,omitempty"`
}

const _directCols = "id, space_id, chain_id, reply_to_id, source_message_id, from_role, to_role, body, status, state, created_at, from_bot_id, to_bot_id, origin_space_id, origin_message_id, kind, request_key"

const (
	DirectSent   = "sent"
	DirectDone   = "done"
	DirectFailed = "failed"
)

// The state of a request: waiting until a turn starts for it, delivered while its recipient
// runs, answered once its response is stored or its recipient ended with nothing to say, and
// failed or stopped when it or its response wake did not run to the end, and cancelled when an
// exchange was withdrawn while it waited. A response has none.
const (
	DirectWaiting   = "waiting"
	DirectDelivered = "delivered"
	DirectAnswered  = "answered"
	DirectStopped   = "stopped"
	DirectCancelled = "cancelled"
)

// FailedDirectState is the state a request takes when its failed response has body: stopped
// for the body "stopped" a wake gets when the user stops its Space, failed for any other.
func FailedDirectState(body string) string {
	if body == "stopped" {
		return DirectStopped
	}
	return DirectFailed
}

// startDirect sets the state of the request a turn woken by direct message id belongs to: a
// request is delivered, and a turn for its response puts back answered, so a response wake that
// failed and is retried does not leave the request failed.
func (s *Store) startDirect(ctx context.Context, id int64) error {
	dm, err := s.DirectMessage(ctx, id)
	if err != nil {
		return err
	}
	if dm.ReplyToID != nil {
		return setDirectState(ctx, s.db, *dm.ReplyToID, DirectAnswered)
	}
	return setDirectState(ctx, s.db, dm.ID, DirectDelivered)
}

// SetDirectRequestState sets the state of request id.
func (s *Store) SetDirectRequestState(ctx context.Context, id int64, state string) error {
	return setDirectState(ctx, s.db, id, state)
}

func setDirectState(ctx context.Context, db execer, id int64, state string) error {
	if _, err := db.ExecContext(ctx,
		"UPDATE direct_message SET state = ? WHERE id = ?", state, id); err != nil {
		return fmt.Errorf("set direct request %d state %s: %w", id, state, err)
	}
	return nil
}

// stopWaitingDirects gives each request waiting in the Spaces matched by inSpaces, a condition on
// space_id with one placeholder for arg, a stopped response, so a closed Space keeps none waiting.
func (s *Store) stopWaitingDirects(
	ctx context.Context, tx *sql.Tx, inSpaces string, arg int64,
) error {
	rows, err := tx.QueryContext(ctx,
		"SELECT id, to_role, to_bot_id FROM direct_message WHERE reply_to_id IS NULL AND state = ? AND "+inSpaces,
		DirectWaiting, arg)
	if err != nil {
		return fmt.Errorf("list waiting direct requests: %w", err)
	}
	type request struct {
		id    int64
		to    string
		botID *int64
	}
	var waiting []request
	for rows.Next() {
		var r request
		if err := rows.Scan(&r.id, &r.to, &r.botID); err != nil {
			_ = rows.Close()
			return fmt.Errorf("list waiting direct requests: %w", err)
		}
		waiting = append(waiting, r)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return fmt.Errorf("list waiting direct requests: %w", err)
	}
	for _, r := range waiting {
		if _, err := s.CreateFailedDirectResponseTx(ctx, tx, r.id, r.botID, r.to, "stopped"); err != nil {
			return fmt.Errorf("stop direct request %d: %w", r.id, err)
		}
	}
	return nil
}

// IDRef returns the message id as a pointer for records that optionally refer to a direct
// wake, such as turns.
func (m *DirectMessage) IDRef() *int64 {
	if m == nil {
		return nil
	}
	return &m.ID
}

// CreateDirectMessage starts a direct chain.
func (s *Store) CreateDirectMessage(ctx context.Context, m DirectMessage) (DirectMessage, error) {
	if m.From == "" || m.To == "" || m.FromBotID == nil || m.ToBotID == nil {
		return DirectMessage{}, fmt.Errorf("%w: direct message sender and recipient IDs and names are required", ErrInvalid)
	}
	if *m.FromBotID == *m.ToBotID {
		return DirectMessage{}, fmt.Errorf("%w: cannot send a direct message to yourself", ErrInvalid)
	}
	if strings.TrimSpace(m.Body) == "" {
		return DirectMessage{}, fmt.Errorf("%w: direct message body is required", ErrInvalid)
	}
	now := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return DirectMessage{}, fmt.Errorf("begin direct message: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx,
		"INSERT INTO direct_message (space_id, source_message_id, from_role, to_role, body, status, state, created_at, from_bot_id, to_bot_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		m.SpaceID, m.SourceMessageID, m.From, m.To, m.Body, DirectSent, DirectWaiting, now.Format(time.RFC3339Nano), m.FromBotID, m.ToBotID)
	if err != nil {
		return DirectMessage{}, s.writeErr(ctx, "insert direct message", err, ErrInvalid,
			parent{table: "space", id: &m.SpaceID})
	}
	id, err := res.LastInsertId()
	if err != nil {
		return DirectMessage{}, fmt.Errorf("insert direct message: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE direct_message SET chain_id = ? WHERE id = ?", id, id); err != nil {
		return DirectMessage{}, fmt.Errorf("set direct message chain: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return DirectMessage{}, fmt.Errorf("commit direct message: %w", err)
	}
	m.ID, m.ChainID, m.Status, m.State, m.CreatedAt = id, id, DirectSent, DirectWaiting, now
	return m, nil
}

// CreateDirectResponse records the automatic response to a request.
func (s *Store) CreateDirectResponse(ctx context.Context, requestID int64, botID *int64, from, body string) (DirectMessage, error) {
	return s.respond(ctx, requestID, from, body, DirectDone, botID)
}

func (s *Store) CreateFailedDirectResponseTx(ctx context.Context, tx *sql.Tx, requestID int64, botID *int64, from, body string) (DirectMessage, error) {
	return s.createDirectResponse(ctx, tx, requestID, from, body, DirectFailed, botID)
}

func (s *Store) CreateFailedDirectResponse(ctx context.Context, requestID int64, botID *int64, from, body string) (DirectMessage, error) {
	return s.respond(ctx, requestID, from, body, DirectFailed, botID)
}

// respond stores a response, the request's new state and the reply in its origin conversation
// together, so a crash leaves all of them or none.
func (s *Store) respond(
	ctx context.Context, requestID int64, from, body, status string, botID *int64,
) (DirectMessage, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return DirectMessage{}, fmt.Errorf("begin direct response: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	m, err := s.createDirectResponse(ctx, tx, requestID, from, body, status, botID)
	if err != nil {
		return DirectMessage{}, err
	}
	if err := tx.Commit(); err != nil {
		return DirectMessage{}, fmt.Errorf("commit direct response: %w", err)
	}
	return m, nil
}

func (s *Store) createDirectResponse(
	ctx context.Context, db execer, requestID int64, from, body, status string, botID *int64,
) (DirectMessage, error) {
	request, err := s.DirectMessage(ctx, requestID)
	if err != nil {
		return DirectMessage{}, err
	}
	if request.ReplyToID != nil || botID == nil || request.ToBotID == nil || *request.ToBotID != *botID {
		return DirectMessage{}, fmt.Errorf("%w: direct message %d cannot be answered by %s",
			ErrInvalid, requestID, from)
	}
	if status == DirectDone && strings.TrimSpace(body) == "" {
		return DirectMessage{}, fmt.Errorf("%w: direct message body is required", ErrInvalid)
	}
	now := time.Now().UTC()
	m := DirectMessage{SpaceID: request.SpaceID, ChainID: request.ChainID,
		ReplyToID: &request.ID, SourceMessageID: request.SourceMessageID,
		OriginSpaceID: request.OriginSpaceID, Kind: request.Kind, From: from, To: request.From, FromBotID: request.ToBotID, ToBotID: request.FromBotID, Body: body, Status: status, CreatedAt: now}
	res, err := db.ExecContext(ctx,
		`INSERT INTO direct_message
		(space_id, chain_id, reply_to_id, source_message_id, from_role, to_role, body, status, state, created_at, from_bot_id, to_bot_id, origin_space_id, kind)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.SpaceID, m.ChainID, m.ReplyToID, m.SourceMessageID, m.From, m.To, m.Body, m.Status,
		"", now.Format(time.RFC3339Nano), m.FromBotID, m.ToBotID, m.OriginSpaceID, m.Kind)
	if err != nil {
		if isUnique(err) {
			return DirectMessage{}, fmt.Errorf("direct message %d already has a response: %w",
				requestID, ErrConflict)
		}
		return DirectMessage{}, fmt.Errorf("insert direct response: %w", err)
	}
	m.ID, err = res.LastInsertId()
	if err != nil {
		return DirectMessage{}, fmt.Errorf("insert direct response: %w", err)
	}
	state := DirectAnswered
	if status == DirectFailed {
		state = FailedDirectState(body)
	}
	if err := setDirectState(ctx, db, requestID, state); err != nil {
		return DirectMessage{}, err
	}
	return m, s.postExchangeReply(ctx, db, request, m, state)
}

// DirectMessage returns one direct message.
func (s *Store) DirectMessage(ctx context.Context, id int64) (DirectMessage, error) {
	m, err := scanDirect(s.db.QueryRowContext(ctx,
		"SELECT "+_directCols+" FROM direct_message WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return DirectMessage{}, fmt.Errorf("direct message %d: %w", id, ErrNotFound)
	}
	return m, err
}

// DirectMessages lists all direct messages in a Space, oldest first.
func (s *Store) DirectMessages(ctx context.Context, spaceID, after int64) ([]DirectMessage, error) {
	if err := s.requireSpace(ctx, spaceID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, "SELECT "+_directCols+
		" FROM direct_message WHERE space_id = ? AND id > ? ORDER BY id", spaceID, after)
	if err != nil {
		return nil, fmt.Errorf("list direct messages: %w", err)
	}
	return scanAll(rows, scanDirect)
}

func (s *Store) DirectMessagesForBot(ctx context.Context, spaceID, botID, after int64) ([]DirectMessage, error) {
	if err := s.requireSpace(ctx, spaceID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, "SELECT "+_directCols+" FROM direct_message WHERE space_id = ? AND id > ? AND (from_bot_id = ? OR to_bot_id = ?) ORDER BY id", spaceID, after, botID, botID)
	if err != nil {
		return nil, fmt.Errorf("list direct messages for Bot: %w", err)
	}
	return scanAll(rows, scanDirect)
}

// LastDirectMessageID returns the newest direct message id in a Space, or zero.
func (s *Store) LastDirectMessageID(ctx context.Context, spaceID int64) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx,
		"SELECT COALESCE(MAX(id), 0) FROM direct_message WHERE space_id = ?", spaceID).Scan(&id)
	return id, err
}

func scanDirect(r interface{ Scan(...any) error }) (DirectMessage, error) {
	var m DirectMessage
	var created string
	if err := r.Scan(&m.ID, &m.SpaceID, &m.ChainID, &m.ReplyToID, &m.SourceMessageID,
		&m.From, &m.To, &m.Body, &m.Status, &m.State, &created, &m.FromBotID, &m.ToBotID,
		&m.OriginSpaceID, &m.OriginMessageID, &m.Kind, &m.RequestKey); err != nil {
		return DirectMessage{}, err
	}
	t, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return DirectMessage{}, fmt.Errorf("direct message %d: bad created_at: %w", m.ID, err)
	}
	m.CreatedAt = t
	return m, nil
}
