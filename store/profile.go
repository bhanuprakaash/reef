package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Profile is the person using Reef. There is one row.
type Profile struct {
	Name      string    `json:"name"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Profile returns the profile, or ErrNotFound before a name was first set.
func (s *Store) Profile(ctx context.Context) (Profile, error) {
	var p Profile
	var updated string
	err := s.db.QueryRowContext(ctx, "SELECT name, updated_at FROM profile WHERE id = 1").
		Scan(&p.Name, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return Profile{}, fmt.Errorf("profile: %w", ErrNotFound)
	}
	if err != nil {
		return Profile{}, fmt.Errorf("read profile: %w", err)
	}
	if p.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated); err != nil {
		return Profile{}, fmt.Errorf("profile: bad updated_at: %w", err)
	}
	return p, nil
}

// PutProfile sets the name, trimmed, replacing any before.
func (s *Store) PutProfile(ctx context.Context, p Profile) (Profile, error) {
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		return Profile{}, fmt.Errorf("%w: profile name is required", ErrInvalid)
	}
	p.UpdatedAt = time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO profile (id, name, updated_at) VALUES (1, ?, ?)
		ON CONFLICT (id) DO UPDATE SET name = excluded.name, updated_at = excluded.updated_at`,
		p.Name, p.UpdatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return Profile{}, fmt.Errorf("upsert profile: %w", err)
	}
	return p, nil
}
