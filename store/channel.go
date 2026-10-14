package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// TokenBudget is the default channel budget and what continuing over budget leaves free.
const TokenBudget = 1_000_000

// HopCap is how many turns woken by agent messages a channel runs between user messages.
const HopCap = 4

const _channelCols = "id, project_id, feature_id, name, role, backend, permission_mode, " +
	"paused, token_budget, created_at, (SELECT name FROM team WHERE id = channel.team_id)"

// Channel with a nil FeatureID is a quick session.
type Channel struct {
	ID             int64     `json:"id"`
	ProjectID      int64     `json:"projectId"`
	FeatureID      *int64    `json:"featureId"`
	Name           string    `json:"name"`
	Role           string    `json:"role"`
	Backend        string    `json:"backend"`
	PermissionMode string    `json:"permissionMode"`
	Paused         string    `json:"paused"`
	TokenBudget    int64     `json:"tokenBudget"`
	Team           string    `json:"team"`
	CreatedAt      time.Time `json:"createdAt"`
}

// ChannelFilter narrows Channels to one project and/or one feature when set.
type ChannelFilter struct {
	ProjectID *int64
	FeatureID *int64
}

// CreateChannel adds c in team c.Team (solo when empty), which must exist. A set
// FeatureID must be a feature of c.ProjectID.
func (s *Store) CreateChannel(ctx context.Context, c Channel) (Channel, error) {
	if err := c.validate(); err != nil {
		return Channel{}, err
	}
	if err := s.checkFeatureInProject(ctx, c.FeatureID, c.ProjectID); err != nil {
		return Channel{}, err
	}
	if c.Team == "" {
		c.Team = SoloTeam
	}
	var teamID int64
	err := s.db.QueryRowContext(ctx, "SELECT id FROM team WHERE name = ?", c.Team).Scan(&teamID)
	if errors.Is(err, sql.ErrNoRows) {
		return Channel{}, fmt.Errorf("%w: unknown team %s", ErrInvalid, c.Team)
	}
	if err != nil {
		return Channel{}, fmt.Errorf("read team: %w", err)
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO channel (project_id, feature_id, name, role, backend, permission_mode, "+
			"token_budget, created_at, team_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		c.ProjectID, c.FeatureID, c.Name, c.Role, c.Backend, c.PermissionMode, TokenBudget,
		now.Format(time.RFC3339Nano), teamID)
	if err != nil {
		return Channel{}, s.writeErr(ctx, "insert channel", err, ErrInvalid,
			parent{table: "project", id: &c.ProjectID}, parent{table: "feature", id: c.FeatureID})
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Channel{}, fmt.Errorf("insert channel: %w", err)
	}
	c.ID, c.Paused, c.TokenBudget, c.CreatedAt = id, "", TokenBudget, now
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

// UpdateChannel sets the name and permission mode of c.ID. A channel never changes
// project, and its feature changes only from nil, through PromoteChannel.
func (s *Store) UpdateChannel(ctx context.Context, c Channel) (Channel, error) {
	if err := c.validate(); err != nil {
		return Channel{}, err
	}
	res, err := s.db.ExecContext(ctx,
		"UPDATE channel SET name = ?, permission_mode = ? WHERE id = ?",
		c.Name, c.PermissionMode, c.ID)
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

// PromoteChannel attaches quick session id to feature f: the existing one when f.ID
// is set, else a new one. A channel that already has a feature is ErrInvalid.
func (s *Store) PromoteChannel(ctx context.Context, id int64, f Feature) (Channel, error) {
	tx, err := s.promote(ctx, id, f)
	if err != nil {
		return Channel{}, err
	}
	if err := tx.Commit(); err != nil {
		return Channel{}, fmt.Errorf("commit promote: %w", err)
	}
	return s.Channel(ctx, id)
}

// CheckPromote returns the error PromoteChannel would return, and changes nothing.
func (s *Store) CheckPromote(ctx context.Context, id int64, f Feature) error {
	tx, err := s.promote(ctx, id, f)
	if err != nil {
		return err
	}
	return tx.Rollback()
}

// promote does the work of PromoteChannel in a transaction it leaves open for the
// caller to commit or roll back. On error it has rolled the transaction back.
func (s *Store) promote(ctx context.Context, id int64, f Feature) (_ *sql.Tx, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin promote: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	var projectID int64
	var featureID *int64
	err = tx.QueryRowContext(ctx, "SELECT project_id, feature_id FROM channel WHERE id = ?", id).
		Scan(&projectID, &featureID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("channel %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("read channel %d: %w", id, err)
	}
	if featureID != nil {
		return nil, fmt.Errorf("%w: channel %d already has feature %d", ErrInvalid, id, *featureID)
	}
	if f.ID != 0 {
		var inProject bool
		err = tx.QueryRowContext(ctx,
			"SELECT EXISTS (SELECT 1 FROM feature WHERE id = ? AND project_id = ?)",
			f.ID, projectID).Scan(&inProject)
		if err != nil {
			return nil, fmt.Errorf("check feature: %w", err)
		}
		if !inProject {
			return nil, fmt.Errorf("%w: feature %d is not in project %d", ErrInvalid, f.ID, projectID)
		}
	} else {
		if err := f.validateSlug(); err != nil {
			return nil, err
		}
		if err := f.validate(); err != nil {
			return nil, err
		}
		f.ProjectID = projectID
		if f, err = insertFeature(ctx, tx, f); err != nil {
			return nil, err
		}
	}
	if _, err := tx.ExecContext(ctx,
		"UPDATE channel SET feature_id = ? WHERE id = ?", f.ID, id); err != nil {
		return nil, fmt.Errorf("update channel: %w", err)
	}
	return tx, nil
}

// PauseChannel pauses the channel; the reason must not be empty.
func (s *Store) PauseChannel(ctx context.Context, id int64, reason string) error {
	if reason == "" {
		return fmt.Errorf("%w: pause reason is required", ErrInvalid)
	}
	return s.setChannel(ctx, id, "paused = ?", reason)
}

// ContinueChannel clears the pause, drops the pending holds it left and raises the budget to
// at least budget.
func (s *Store) ContinueChannel(ctx context.Context, id, budget int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin continue: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx,
		"UPDATE channel SET paused = '', token_budget = MAX(token_budget, ?) WHERE id = ? AND paused <> ''",
		budget, id)
	if err != nil {
		return fmt.Errorf("update channel: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update channel: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("channel %d is not paused: %w", id, ErrConflict)
	}
	if _, err := tx.ExecContext(ctx,
		"UPDATE approval SET status = ? WHERE channel_id = ? AND status = ? AND kind = ?",
		ApprovalRejected, id, ApprovalPending, ApprovalHold); err != nil {
		return fmt.Errorf("drop holds: %w", err)
	}
	return tx.Commit()
}

// ChannelTokens sums input, output and cache write tokens; cache reads are not counted.
func (s *Store) ChannelTokens(ctx context.Context, id int64) (int64, error) {
	var n int64
	err := s.db.QueryRowContext(ctx,
		"SELECT COALESCE(SUM(input_tokens + output_tokens + cache_write_tokens), 0) FROM turn WHERE channel_id = ?",
		id).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("sum channel tokens: %w", err)
	}
	return n, nil
}

func (s *Store) setChannel(ctx context.Context, id int64, set string, args ...any) error {
	res, err := s.db.ExecContext(ctx, "UPDATE channel SET "+set+" WHERE id = ?", append(args, id)...)
	if err != nil {
		return fmt.Errorf("update channel: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update channel: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("channel %d: %w", id, ErrNotFound)
	}
	return nil
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
	if err := r.Scan(&c.ID, &c.ProjectID, &c.FeatureID, &c.Name, &c.Role, &c.Backend,
		&c.PermissionMode, &c.Paused, &c.TokenBudget, &created, &c.Team); err != nil {
		return Channel{}, err
	}
	t, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return Channel{}, fmt.Errorf("channel %d: bad created_at: %w", c.ID, err)
	}
	c.CreatedAt = t
	return c, nil
}
