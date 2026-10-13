package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const _channelCols = "id, project_id, feature_id, name, created_at"

// Channel with a nil FeatureID is a quick session.
type Channel struct {
	ID        int64     `json:"id"`
	ProjectID int64     `json:"projectId"`
	FeatureID *int64    `json:"featureId"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}

// ChannelFilter narrows Channels to one project and/or one feature when set.
type ChannelFilter struct {
	ProjectID *int64
	FeatureID *int64
}

// CreateChannel adds c. A set FeatureID must be a feature of c.ProjectID.
func (s *Store) CreateChannel(ctx context.Context, c Channel) (Channel, error) {
	if err := c.validate(); err != nil {
		return Channel{}, err
	}
	if err := s.checkFeatureInProject(ctx, c.FeatureID, c.ProjectID); err != nil {
		return Channel{}, err
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO channel (project_id, feature_id, name, created_at) VALUES (?, ?, ?, ?)",
		c.ProjectID, c.FeatureID, c.Name, now.Format(time.RFC3339Nano))
	if err != nil {
		return Channel{}, s.writeErr(ctx, "insert channel", err, ErrInvalid,
			parent{table: "project", id: &c.ProjectID}, parent{table: "feature", id: c.FeatureID})
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Channel{}, fmt.Errorf("insert channel: %w", err)
	}
	c.ID, c.CreatedAt = id, now
	return c, nil
}

// Channel returns the channel with the id, or ErrNotFound.
func (s *Store) Channel(ctx context.Context, id int64) (Channel, error) {
	row := s.db.QueryRowContext(ctx, "SELECT "+_channelCols+" FROM channel WHERE id = ?", id)
	c, err := scanChannel(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Channel{}, fmt.Errorf("channel %d: %w", id, ErrNotFound)
	}
	return c, err
}

// Channels lists channels by id, narrowed by the set fields of f.
func (s *Store) Channels(ctx context.Context, f ChannelFilter) ([]Channel, error) {
	q, args := "SELECT "+_channelCols+" FROM channel WHERE 1 = 1", []any{}
	if f.ProjectID != nil {
		q, args = q+" AND project_id = ?", append(args, *f.ProjectID)
	}
	if f.FeatureID != nil {
		q, args = q+" AND feature_id = ?", append(args, *f.FeatureID)
	}
	rows, err := s.db.QueryContext(ctx, q+" ORDER BY id", args...)
	if err != nil {
		return nil, fmt.Errorf("list channels: %w", err)
	}
	return scanAll(rows, scanChannel)
}

// UpdateChannel sets the name of c.ID. A channel never changes project or
// feature.
func (s *Store) UpdateChannel(ctx context.Context, c Channel) (Channel, error) {
	if err := c.validate(); err != nil {
		return Channel{}, err
	}
	res, err := s.db.ExecContext(ctx, "UPDATE channel SET name = ? WHERE id = ?", c.Name, c.ID)
	if err != nil {
		return Channel{}, fmt.Errorf("update channel: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return Channel{}, fmt.Errorf("update channel: %w", err)
	}
	if n == 0 {
		return Channel{}, fmt.Errorf("channel %d: %w", c.ID, ErrNotFound)
	}
	return s.Channel(ctx, c.ID)
}

// DeleteChannel removes the channel and, by cascade, its messages and turns.
func (s *Store) DeleteChannel(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM channel WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete channel: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete channel: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("channel %d: %w", id, ErrNotFound)
	}
	return nil
}

func (c Channel) validate() error {
	if c.Name == "" {
		return fmt.Errorf("%w: channel name is required", ErrInvalid)
	}
	return nil
}

func scanChannel(r interface{ Scan(...any) error }) (Channel, error) {
	var c Channel
	var created string
	if err := r.Scan(&c.ID, &c.ProjectID, &c.FeatureID, &c.Name, &created); err != nil {
		return Channel{}, err
	}
	t, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return Channel{}, fmt.Errorf("channel %d: bad created_at: %w", c.ID, err)
	}
	c.CreatedAt = t
	return c, nil
}
