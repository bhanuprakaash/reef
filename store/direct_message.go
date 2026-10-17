package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// DirectMessage is a private message between two roles in one channel. A request starts a
// chain; its one automatic response points back to it and shares its chain.
type DirectMessage struct {
	ID        int64     `json:"id"`
	ChannelID int64     `json:"channelId"`
	ChainID   int64     `json:"chainId"`
	ReplyToID *int64    `json:"replyToId"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	Body      string    `json:"body"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
}

const _directCols = "id, channel_id, chain_id, reply_to_id, from_role, to_role, body, status, created_at"

const (
	DirectSent   = "sent"
	DirectDone   = "done"
	DirectFailed = "failed"
)

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
	if m.From == "" || m.To == "" {
		return DirectMessage{}, fmt.Errorf("%w: direct message sender and recipient are required", ErrInvalid)
	}
	if m.From == m.To {
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
		"INSERT INTO direct_message (channel_id, from_role, to_role, body, status, created_at) VALUES (?, ?, ?, ?, ?, ?)",
		m.ChannelID, m.From, m.To, m.Body, DirectSent, now.Format(time.RFC3339Nano))
	if err != nil {
		return DirectMessage{}, s.writeErr(ctx, "insert direct message", err, ErrInvalid,
			parent{table: "channel", id: &m.ChannelID})
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
	m.ID, m.ChainID, m.Status, m.CreatedAt = id, id, DirectSent, now
	return m, nil
}

// CreateDirectResponse records the one automatic response to requestID, addressed back to
// its sender. A response cannot itself receive an automatic response.
func (s *Store) CreateDirectResponse(
	ctx context.Context, requestID int64, from, body string,
) (DirectMessage, error) {
	return s.createDirectResponse(ctx, requestID, from, body, DirectDone)
}

// CreateFailedDirectResponse preserves partial output from a failed direct turn. It is
// inspectable but never routed, and does not prevent a later completed response.
func (s *Store) CreateFailedDirectResponse(
	ctx context.Context, requestID int64, from, body string,
) (DirectMessage, error) {
	return s.createDirectResponse(ctx, requestID, from, body, DirectFailed)
}

func (s *Store) createDirectResponse(
	ctx context.Context, requestID int64, from, body, status string,
) (DirectMessage, error) {
	request, err := s.DirectMessage(ctx, requestID)
	if err != nil {
		return DirectMessage{}, err
	}
	if request.ReplyToID != nil || request.To != from {
		return DirectMessage{}, fmt.Errorf("%w: direct message %d cannot be answered by %s",
			ErrInvalid, requestID, from)
	}
	if strings.TrimSpace(body) == "" {
		return DirectMessage{}, fmt.Errorf("%w: direct message body is required", ErrInvalid)
	}
	now := time.Now().UTC()
	m := DirectMessage{ChannelID: request.ChannelID, ChainID: request.ChainID,
		ReplyToID: &request.ID, From: from, To: request.From, Body: body, Status: status, CreatedAt: now}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO direct_message
		(channel_id, chain_id, reply_to_id, from_role, to_role, body, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ChannelID, m.ChainID, m.ReplyToID, m.From, m.To, m.Body, m.Status,
		now.Format(time.RFC3339Nano))
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
	return m, nil
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

// DirectMessages lists all direct messages in a channel, oldest first.
func (s *Store) DirectMessages(ctx context.Context, channelID, after int64) ([]DirectMessage, error) {
	if err := s.requireChannel(ctx, channelID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, "SELECT "+_directCols+
		" FROM direct_message WHERE channel_id = ? AND id > ? ORDER BY id", channelID, after)
	if err != nil {
		return nil, fmt.Errorf("list direct messages: %w", err)
	}
	return scanAll(rows, scanDirect)
}

// DirectMessagesForRole lists only exchanges in which role participated.
func (s *Store) DirectMessagesForRole(
	ctx context.Context, channelID int64, role string, after int64,
) ([]DirectMessage, error) {
	if err := s.requireChannel(ctx, channelID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, "SELECT "+_directCols+
		" FROM direct_message WHERE channel_id = ? AND id > ? AND (from_role = ? OR to_role = ?) ORDER BY id",
		channelID, after, role, role)
	if err != nil {
		return nil, fmt.Errorf("list direct messages for %s: %w", role, err)
	}
	return scanAll(rows, scanDirect)
}

// LastDirectMessageID returns the newest direct message id in a channel, or zero.
func (s *Store) LastDirectMessageID(ctx context.Context, channelID int64) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx,
		"SELECT COALESCE(MAX(id), 0) FROM direct_message WHERE channel_id = ?", channelID).Scan(&id)
	return id, err
}

func scanDirect(r interface{ Scan(...any) error }) (DirectMessage, error) {
	var m DirectMessage
	var created string
	if err := r.Scan(&m.ID, &m.ChannelID, &m.ChainID, &m.ReplyToID, &m.From, &m.To,
		&m.Body, &m.Status, &created); err != nil {
		return DirectMessage{}, err
	}
	t, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return DirectMessage{}, fmt.Errorf("direct message %d: bad created_at: %w", m.ID, err)
	}
	m.CreatedAt = t
	return m, nil
}
