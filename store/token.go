package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

const _tokenBytes = 32

// Token lets one teammate's MCP process act as role in one channel.
type Token struct {
	Token     string    `json:"token"`
	ChannelID int64     `json:"channelId"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"createdAt"`
}

// MessageHit is one message that matched a search.
type MessageHit struct {
	ChannelID int64  `json:"channelId"`
	MessageID int64  `json:"messageId"`
	Author    string `json:"author"`
	Snippet   string `json:"snippet"`
}

// CreateToken adds a token for role in the channel.
func (s *Store) CreateToken(ctx context.Context, channelID int64, role string) (Token, error) {
	if role == "" {
		return Token{}, fmt.Errorf("%w: token role is required", ErrInvalid)
	}
	raw := make([]byte, _tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return Token{}, fmt.Errorf("generate token: %w", err)
	}
	t := Token{
		Token: hex.EncodeToString(raw), ChannelID: channelID, Role: role,
		CreatedAt: time.Now().UTC(),
	}
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO token (token, channel_id, role, created_at) VALUES (?, ?, ?, ?)",
		t.Token, t.ChannelID, t.Role, t.CreatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return Token{}, s.writeErr(ctx, "insert token", err, ErrNotFound,
			parent{table: "channel", id: &channelID})
	}
	return t, nil
}

// Token returns the token with the value, or ErrNotFound.
func (s *Store) Token(ctx context.Context, value string) (Token, error) {
	t := Token{Token: value}
	var created string
	err := s.db.QueryRowContext(ctx,
		"SELECT channel_id, role, created_at FROM token WHERE token = ?", value).
		Scan(&t.ChannelID, &t.Role, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return Token{}, fmt.Errorf("token: %w", ErrNotFound)
	}
	if err != nil {
		return Token{}, fmt.Errorf("read token: %w", err)
	}
	if t.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return Token{}, fmt.Errorf("token: bad created_at: %w", err)
	}
	return t, nil
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
