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
	Name       string    `json:"name"`
	Occupation string    `json:"occupation"`
	Style      []string  `json:"style"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// Profile returns the profile, or ErrNotFound before a name was first set.
func (s *Store) Profile(ctx context.Context) (Profile, error) {
	var p Profile
	var style, updated string
	err := s.db.QueryRowContext(ctx, "SELECT name, occupation, style, updated_at FROM profile WHERE id = 1").
		Scan(&p.Name, &p.Occupation, &style, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return Profile{}, fmt.Errorf("profile: %w", ErrNotFound)
	}
	if err != nil {
		return Profile{}, fmt.Errorf("read profile: %w", err)
	}
	p.Style = []string{}
	if style != "" {
		p.Style = strings.Split(style, "\n")
	}
	if p.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated); err != nil {
		return Profile{}, fmt.Errorf("profile: bad updated_at: %w", err)
	}
	return p, nil
}

// PutProfile sets the name, occupation and style, trimmed with empty style entries
// dropped, replacing any before. A line break in any of them is ErrInvalid: the context prompt
// is built from them.
func (s *Store) PutProfile(ctx context.Context, p Profile) (Profile, error) {
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		return Profile{}, fmt.Errorf("%w: profile name is required", ErrInvalid)
	}
	p.Occupation = strings.TrimSpace(p.Occupation)
	style := make([]string, 0, len(p.Style))
	for _, v := range p.Style {
		if v = strings.TrimSpace(v); v != "" {
			style = append(style, v)
		}
	}
	for _, v := range append([]string{p.Name, p.Occupation}, style...) {
		if strings.ContainsAny(v, "\r\n") {
			return Profile{}, fmt.Errorf("%w: profile fields cannot contain a line break", ErrInvalid)
		}
	}
	p.Style = style
	p.UpdatedAt = time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO profile (id, name, occupation, style, updated_at) VALUES (1, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET name = excluded.name, occupation = excluded.occupation,
		style = excluded.style, updated_at = excluded.updated_at`,
		p.Name, p.Occupation, strings.Join(style, "\n"), p.UpdatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return Profile{}, fmt.Errorf("upsert profile: %w", err)
	}
	return p, nil
}
