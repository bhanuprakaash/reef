package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Message is append-only: there is no update or delete. A reply has the thread root as
// ParentID; a top-level message has none.
type Message struct {
	ID        int64     `json:"id"`
	ChannelID int64     `json:"channelId"`
	ParentID  *int64    `json:"parentId"`
	Author    string    `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
}

// MessageHit is one message that matched a search.
type MessageHit struct {
	ChannelID int64  `json:"channelId"`
	MessageID int64  `json:"messageId"`
	Author    string `json:"author"`
	Snippet   string `json:"snippet"`
}

// CreateMessage appends m to its channel.
func (s *Store) CreateMessage(ctx context.Context, m Message) (Message, error) {
	if m.Author == "" {
		return Message{}, fmt.Errorf("%w: message author is required", ErrInvalid)
	}
	if m.Body == "" {
		return Message{}, fmt.Errorf("%w: message body is required", ErrInvalid)
	}
	if m.ParentID != nil {
		var root bool
		err := s.db.QueryRowContext(ctx,
			"SELECT parent_id IS NULL FROM message WHERE id = ? AND channel_id = ?",
			*m.ParentID, m.ChannelID).Scan(&root)
		switch {
		case errors.Is(err, sql.ErrNoRows), err == nil && !root:
			return Message{}, fmt.Errorf(
				"%w: parent %d is not a top-level message of channel %d",
				ErrInvalid, *m.ParentID, m.ChannelID)
		case err != nil:
			return Message{}, fmt.Errorf("check parent: %w", err)
		}
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO message (channel_id, parent_id, author, body, created_at) "+
			"VALUES (?, ?, ?, ?, ?)",
		m.ChannelID, m.ParentID, m.Author, m.Body, now.Format(time.RFC3339Nano))
	if err != nil {
		return Message{}, s.writeErr(ctx, "insert message", err, ErrInvalid,
			parent{table: "channel", id: &m.ChannelID})
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
		"SELECT id, channel_id, parent_id, author, body, created_at FROM message "+
			"WHERE channel_id = ? AND id > ? ORDER BY id",
		channelID, after)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	return scanAll(rows, scanMessage)
}

// Thread lists the top-level message root of the channel and its replies, oldest first.
func (s *Store) Thread(ctx context.Context, channelID, root int64) ([]Message, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, channel_id, parent_id, author, body, created_at FROM message "+
			"WHERE channel_id = ? AND (id = ? OR parent_id = ?) ORDER BY id",
		channelID, root, root)
	if err != nil {
		return nil, fmt.Errorf("list thread: %w", err)
	}
	msgs, err := scanAll(rows, scanMessage)
	if err != nil {
		return nil, err
	}
	if len(msgs) == 0 || msgs[0].ParentID != nil {
		return nil, fmt.Errorf("thread %d: %w", root, ErrNotFound)
	}
	return msgs, nil
}

// ThreadOwner returns the author of the root of the thread that message id is in.
func (s *Store) ThreadOwner(ctx context.Context, id int64) (string, error) {
	var author string
	err := s.db.QueryRowContext(ctx,
		"SELECT r.author FROM message m JOIN message r ON r.id = COALESCE(m.parent_id, m.id) "+
			"WHERE m.id = ?", id).Scan(&author)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("message %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return "", fmt.Errorf("thread owner: %w", err)
	}
	return author, nil
}

// CountMessages returns how many messages the channel has.
func (s *Store) CountMessages(ctx context.Context, channelID int64) (int, error) {
	if err := s.requireChannel(ctx, channelID); err != nil {
		return 0, err
	}
	var n int
	err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM message WHERE channel_id = ?", channelID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count messages: %w", err)
	}
	return n, nil
}

// SearchMessages finds messages in the feature's channels whose body contains
// query, ignoring ASCII case, newest first. A hit's snippet is about 120
// characters of the body around the match.
func (s *Store) SearchMessages(
	ctx context.Context, featureID int64, query string, limit int,
) ([]MessageHit, error) {
	like := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(query)
	rows, err := s.db.QueryContext(ctx,
		`SELECT m.channel_id, m.id, m.author,
		  substr(m.body, max(instr(lower(m.body), lower(?)) - 40, 1), 120)
		FROM message m JOIN channel c ON c.id = m.channel_id
		WHERE c.feature_id = ? AND m.body LIKE '%' || ? || '%' ESCAPE '\'
		ORDER BY m.id DESC LIMIT ?`,
		query, featureID, like, limit)
	if err != nil {
		return nil, fmt.Errorf("search messages: %w", err)
	}
	return scanAll(rows, func(r interface{ Scan(...any) error }) (MessageHit, error) {
		var h MessageHit
		err := r.Scan(&h.ChannelID, &h.MessageID, &h.Author, &h.Snippet)
		return h, err
	})
}

func scanMessage(r interface{ Scan(...any) error }) (Message, error) {
	var m Message
	var created string
	if err := r.Scan(&m.ID, &m.ChannelID, &m.ParentID, &m.Author, &m.Body, &created); err != nil {
		return Message{}, err
	}
	t, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return Message{}, fmt.Errorf("message %d: bad created_at: %w", m.ID, err)
	}
	m.CreatedAt = t
	return m, nil
}
