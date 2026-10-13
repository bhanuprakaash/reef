package store

import (
	"context"
	"fmt"
	"time"
)

// Message is append-only: there is no update or delete.
type Message struct {
	ID        int64     `json:"id"`
	ChannelID int64     `json:"channelId"`
	Author    string    `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
}

// CreateMessage appends m to its channel.
func (s *Store) CreateMessage(ctx context.Context, m Message) (Message, error) {
	if m.Author == "" {
		return Message{}, fmt.Errorf("%w: message author is required", ErrInvalid)
	}
	if m.Body == "" {
		return Message{}, fmt.Errorf("%w: message body is required", ErrInvalid)
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO message (channel_id, author, body, created_at) VALUES (?, ?, ?, ?)",
		m.ChannelID, m.Author, m.Body, now.Format(time.RFC3339Nano))
	if err != nil {
		return Message{}, s.writeErr(ctx, "insert message", err, ErrInvalid,
			parent{"channel", &m.ChannelID})
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Message{}, fmt.Errorf("insert message: %w", err)
	}
	m.ID, m.CreatedAt = id, now
	return m, nil
}

// Messages lists the channel's messages with id > after, oldest first.
func (s *Store) Messages(ctx context.Context, channelID, after int64) ([]Message, error) {
	if err := s.requireChannel(ctx, channelID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, channel_id, author, body, created_at FROM message "+
			"WHERE channel_id = ? AND id > ? ORDER BY id",
		channelID, after)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	return scanAll(rows, scanMessage)
}

func scanMessage(r interface{ Scan(...any) error }) (Message, error) {
	var m Message
	var created string
	if err := r.Scan(&m.ID, &m.ChannelID, &m.Author, &m.Body, &created); err != nil {
		return Message{}, err
	}
	t, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return Message{}, fmt.Errorf("message %d: bad created_at: %w", m.ID, err)
	}
	m.CreatedAt = t
	return m, nil
}
