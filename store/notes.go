package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type RoleNotes struct {
	FeatureID int64     `json:"featureId"`
	Role      string    `json:"role"`
	Body      string    `json:"body"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// PutRoleNotes sets the notes of one role on a feature, replacing any before.
func (s *Store) PutRoleNotes(ctx context.Context, n RoleNotes) (RoleNotes, error) {
	if n.Role == "" {
		return RoleNotes{}, fmt.Errorf("%w: role is required", ErrInvalid)
	}
	if err := s.requireFeature(ctx, n.FeatureID); err != nil {
		return RoleNotes{}, err
	}
	n.UpdatedAt = time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO role_notes (feature_id, role, body, updated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT (feature_id, role) DO UPDATE SET body = excluded.body, updated_at = excluded.updated_at`,
		n.FeatureID, n.Role, n.Body, n.UpdatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return RoleNotes{}, fmt.Errorf("upsert role notes: %w", err)
	}
	return n, nil
}

func (s *Store) RoleNotes(ctx context.Context, featureID int64, role string) (RoleNotes, error) {
	n := RoleNotes{FeatureID: featureID, Role: role}
	var updated string
	err := s.db.QueryRowContext(ctx,
		"SELECT body, updated_at FROM role_notes WHERE feature_id = ? AND role = ?",
		featureID, role).Scan(&n.Body, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return RoleNotes{}, fmt.Errorf("notes of %s on feature %d: %w", role, featureID, ErrNotFound)
	}
	if err != nil {
		return RoleNotes{}, fmt.Errorf("read role notes: %w", err)
	}
	if n.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated); err != nil {
		return RoleNotes{}, fmt.Errorf("notes of %s on feature %d: bad updated_at: %w", role, featureID, err)
	}
	return n, nil
}
