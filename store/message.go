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
	if isForeignKey(err) {
		return Message{}, fmt.Errorf("%w: unknown channel %d", ErrInvalid, m.ChannelID)
	}
	if err != nil {
		return Message{}, fmt.Errorf("insert message: %w", err)
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
		"SELECT id, channel_id, author, body, created_at FROM message WHERE channel_id = ? AND id > ? ORDER BY id",
		channelID, after)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	defer rows.Close()
	var ms []Message
	for rows.Next() {
		var m Message
		var created string
		if err := rows.Scan(&m.ID, &m.ChannelID, &m.Author, &m.Body, &created); err != nil {
			return nil, err
		}
		if m.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
			return nil, fmt.Errorf("message %d: bad created_at: %w", m.ID, err)
		}
		ms = append(ms, m)
	}
	return ms, rows.Err()
}

func (s *Store) requireChannel(ctx context.Context, id int64) error {
	ok, err := s.exists(ctx, "channel", id)
	if err != nil {
		return fmt.Errorf("check channel: %w", err)
	}
	if !ok {
		return fmt.Errorf("channel %d: %w", id, ErrNotFound)
	}
	return nil
}
