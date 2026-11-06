package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

const _tokenBytes = 32

// Token lets one bot's MCP process act as role in one Space.
type Token struct {
	Token     string    `json:"token"`
	SpaceID   int64     `json:"spaceId"`
	Role      string    `json:"role"`
	BotID     *int64    `json:"botId,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	// Review marks the token of a memory review, which may only recall and change its Bot's own memory.
	Review bool `json:"review,omitempty"`
}

// CreateToken adds a token. It never expires; it goes when its Space is deleted.
func (s *Store) CreateToken(ctx context.Context, spaceID, botID int64) (Token, error) {
	return s.createToken(ctx, spaceID, botID, false)
}

// CreateReviewToken adds the token of a memory review of the Bot, kept in Space spaceID.
func (s *Store) CreateReviewToken(ctx context.Context, spaceID, botID int64) (Token, error) {
	return s.createToken(ctx, spaceID, botID, true)
}

func (s *Store) createToken(ctx context.Context, spaceID, botID int64, review bool) (Token, error) {
	raw := make([]byte, _tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return Token{}, fmt.Errorf("generate token: %w", err)
	}
	t := Token{Token: hex.EncodeToString(raw), SpaceID: spaceID, BotID: &botID, CreatedAt: time.Now().UTC(), Review: review}
	err := s.db.QueryRowContext(ctx, "INSERT INTO token (token, space_id, role, bot_id, created_at, review) SELECT ?, ?, bot.name, bot.id, ?, ? FROM bot JOIN space_member ON space_member.bot_id = bot.id WHERE space_member.space_id = ? AND bot.id = ? RETURNING role", t.Token, spaceID, t.CreatedAt.Format(time.RFC3339Nano), review, spaceID, botID).Scan(&t.Role)
	if errors.Is(err, sql.ErrNoRows) {
		return Token{}, fmt.Errorf("%w: Bot is not a Space member", ErrInvalid)
	}
	if err != nil {
		return Token{}, fmt.Errorf("insert token: %w", err)
	}
	return t, nil
}

// Token looks a token up by its value, or returns ErrNotFound.
func (s *Store) Token(ctx context.Context, value string) (Token, error) {
	t := Token{Token: value}
	var created string
	err := s.db.QueryRowContext(ctx,
		"SELECT token.space_id, token.role, token.bot_id, token.created_at, token.review FROM token JOIN space_member ON space_member.space_id = token.space_id AND space_member.bot_id = token.bot_id WHERE token.token = ?", value).
		Scan(&t.SpaceID, &t.Role, &t.BotID, &created, &t.Review)
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

// DeleteToken revokes the token, or returns ErrNotFound when it is gone.
func (s *Store) DeleteToken(ctx context.Context, value string) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM token WHERE token = ?", value)
	if err != nil {
		return fmt.Errorf("delete token: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete token: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("token: %w", ErrNotFound)
	}
	return nil
}
