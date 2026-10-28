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

// Token lets one teammate's MCP process act as role in one Space.
type Token struct {
	Token     string    `json:"token"`
	SpaceID   int64     `json:"spaceId"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"createdAt"`
}

// CreateToken adds a token. It never expires; it goes when its Space is deleted.
func (s *Store) CreateToken(ctx context.Context, spaceID int64, role string) (Token, error) {
	if role == "" {
		return Token{}, fmt.Errorf("%w: token role is required", ErrInvalid)
	}
	raw := make([]byte, _tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return Token{}, fmt.Errorf("generate token: %w", err)
	}
	t := Token{
		Token: hex.EncodeToString(raw), SpaceID: spaceID, Role: role,
		CreatedAt: time.Now().UTC(),
	}
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO token (token, space_id, role, created_at) VALUES (?, ?, ?, ?)",
		t.Token, t.SpaceID, t.Role, t.CreatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return Token{}, s.writeErr(ctx, "insert token", err, ErrNotFound,
			parent{table: "space", id: &spaceID})
	}
	return t, nil
}

// Token looks a token up by its value, or returns ErrNotFound.
func (s *Store) Token(ctx context.Context, value string) (Token, error) {
	t := Token{Token: value}
	var created string
	err := s.db.QueryRowContext(ctx,
		"SELECT space_id, role, created_at FROM token WHERE token = ?", value).
		Scan(&t.SpaceID, &t.Role, &created)
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
