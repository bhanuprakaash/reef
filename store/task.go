package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const (
	_defaultTaskStatus = "todo"
	_taskCols          = "id, project_id, feature_id, title, body, status, created_at, updated_at"
)

// Task is a to-do in a project, optionally tied to one of its features.
type Task struct {
	ID        int64     `json:"id"`
	ProjectID int64     `json:"projectId"`
	FeatureID *int64    `json:"featureId"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// TaskFilter narrows Tasks. A nil id or empty Status matches everything.
type TaskFilter struct {
	ProjectID *int64
	FeatureID *int64
	Status    string
}

// CreateTask adds t. A set FeatureID must be a feature of t.ProjectID.
func (s *Store) CreateTask(ctx context.Context, t Task) (Task, error) {
	if err := t.validate(); err != nil {
		return Task{}, err
	}
	if err := s.checkFeatureInProject(ctx, t.FeatureID, t.ProjectID); err != nil {
		return Task{}, err
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO task (project_id, feature_id, title, body, status, created_at, updated_at) "+
			"VALUES (?, ?, ?, ?, ?, ?, ?)",
		t.ProjectID, t.FeatureID, t.Title, t.Body, t.Status,
		now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err != nil {
		return Task{}, s.writeErr(ctx, "insert task", err, ErrInvalid,
			parent{"project", &t.ProjectID}, parent{"feature", t.FeatureID})
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Task{}, fmt.Errorf("insert task: %w", err)
	}
	t.ID, t.CreatedAt, t.UpdatedAt = id, now, now
	return t, nil
}

// Task returns the task with the id, or ErrNotFound.
func (s *Store) Task(ctx context.Context, id int64) (Task, error) {
	row := s.db.QueryRowContext(ctx, "SELECT "+_taskCols+" FROM task WHERE id = ?", id)
	t, err := scanTask(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Task{}, fmt.Errorf("task %d: %w", id, ErrNotFound)
	}
	return t, err
}

// Tasks lists tasks by id across all projects, narrowed by f.
func (s *Store) Tasks(ctx context.Context, f TaskFilter) ([]Task, error) {
	if f.Status != "" && !isTaskStatus(f.Status) {
		return nil, fmt.Errorf("%w: unknown status %q", ErrInvalid, f.Status)
	}
	q, args := "SELECT "+_taskCols+" FROM task WHERE 1 = 1", []any{}
	if f.ProjectID != nil {
		q, args = q+" AND project_id = ?", append(args, *f.ProjectID)
	}
	if f.FeatureID != nil {
		q, args = q+" AND feature_id = ?", append(args, *f.FeatureID)
	}
	if f.Status != "" {
		q, args = q+" AND status = ?", append(args, f.Status)
	}
	rows, err := s.db.QueryContext(ctx, q+" ORDER BY id", args...)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	return scanAll(rows, scanTask)
}

// UpdateTask sets the title, body, status and feature of t.ID. A task never
// changes project, so its feature must belong to the project it already has.
func (s *Store) UpdateTask(ctx context.Context, t Task) (Task, error) {
	if err := t.validate(); err != nil {
		return Task{}, err
	}
	old, err := s.Task(ctx, t.ID)
	if err != nil {
		return Task{}, err
	}
	t.ProjectID = old.ProjectID
	if err := s.checkFeatureInProject(ctx, t.FeatureID, t.ProjectID); err != nil {
		return Task{}, err
	}
	_, err = s.db.ExecContext(ctx,
		"UPDATE task SET title = ?, body = ?, status = ?, feature_id = ?, updated_at = ? "+
			"WHERE id = ?",
		t.Title, t.Body, t.Status, t.FeatureID, time.Now().UTC().Format(time.RFC3339Nano), t.ID)
	if err != nil {
		return Task{}, s.writeErr(ctx, "update task", err, ErrInvalid,
			parent{"feature", t.FeatureID})
	}
	return s.Task(ctx, t.ID)
}

// DeleteTask removes the task.
func (s *Store) DeleteTask(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM task WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete task: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete task: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("task %d: %w", id, ErrNotFound)
	}
	return nil
}

// validate also fills the default status.
func (t *Task) validate() error {
	if t.Title == "" {
		return fmt.Errorf("%w: task title is required", ErrInvalid)
	}
	if t.Status == "" {
		t.Status = _defaultTaskStatus
	}
	if !isTaskStatus(t.Status) {
		return fmt.Errorf("%w: unknown status %q", ErrInvalid, t.Status)
	}
	return nil
}

func isTaskStatus(s string) bool {
	switch s {
	case "todo", "doing", "done":
		return true
	}
	return false
}

func scanTask(r interface{ Scan(...any) error }) (Task, error) {
	var t Task
	var created, updated string
	if err := r.Scan(
		&t.ID, &t.ProjectID, &t.FeatureID, &t.Title, &t.Body, &t.Status, &created, &updated,
	); err != nil {
		return Task{}, err
	}
	var err error
	if t.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return Task{}, fmt.Errorf("task %d: bad created_at: %w", t.ID, err)
	}
	if t.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated); err != nil {
		return Task{}, fmt.Errorf("task %d: bad updated_at: %w", t.ID, err)
	}
	return t, nil
}
