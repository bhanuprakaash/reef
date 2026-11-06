package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// RoleNotes is what one role keeps on a work item. There is one row per work item
// and role.
type RoleNotes struct {
	WorkItemID int64     `json:"workItemId"`
	Role       string    `json:"role"`
	Body       string    `json:"body"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

const _upsertRoleNotes = `INSERT INTO role_notes (work_item_id, role, body, updated_at)
	VALUES (?, ?, ?, ?)
	ON CONFLICT (work_item_id, role)
	DO UPDATE SET body = excluded.body, updated_at = excluded.updated_at`

// PutRoleNotes sets the notes of one role on a work item, replacing any before.
func (s *Store) PutRoleNotes(ctx context.Context, n RoleNotes) (RoleNotes, error) {
	if n.Role == "" {
		return RoleNotes{}, fmt.Errorf("%w: role is required", ErrInvalid)
	}
	n.UpdatedAt = time.Now().UTC()
	_, err := s.db.ExecContext(ctx, _upsertRoleNotes,
		n.WorkItemID, n.Role, n.Body, n.UpdatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return RoleNotes{}, s.writeErr(ctx, "upsert role notes", err, ErrNotFound,
			parent{table: "work_item", id: &n.WorkItemID})
	}
	return n, nil
}

// DeleteRoleNotes removes the notes of one role on a work item.
func (s *Store) DeleteRoleNotes(ctx context.Context, workItemID int64, role string) error {
	res, err := s.db.ExecContext(ctx,
		"DELETE FROM role_notes WHERE work_item_id = ? AND role = ?", workItemID, role)
	if err != nil {
		return fmt.Errorf("delete role notes: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete role notes: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("notes of %s on work item %d: %w", role, workItemID, ErrNotFound)
	}
	return nil
}

// RoleNotes returns the notes of one role on a work item, or ErrNotFound.
func (s *Store) RoleNotes(
	ctx context.Context, workItemID int64, role string,
) (RoleNotes, error) {
	n := RoleNotes{WorkItemID: workItemID, Role: role}
	var updated string
	err := s.db.QueryRowContext(ctx,
		"SELECT body, updated_at FROM role_notes WHERE work_item_id = ? AND role = ?",
		workItemID, role).Scan(&n.Body, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return RoleNotes{}, fmt.Errorf("notes of %s on work item %d: %w", role, workItemID,
			ErrNotFound)
	}
	if err != nil {
		return RoleNotes{}, fmt.Errorf("read role notes: %w", err)
	}
	if n.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated); err != nil {
		return RoleNotes{}, fmt.Errorf("notes of %s on work item %d: bad updated_at: %w",
			role, workItemID, err)
	}
	return n, nil
}
