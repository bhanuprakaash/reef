package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Decision is append-only: there is no update or delete.
type Decision struct {
	ID        int64     `json:"id"`
	FeatureID int64     `json:"featureId"`
	MessageID *int64    `json:"messageId"`
	Body      string    `json:"body"`
	Why       string    `json:"why"`
	Author    string    `json:"author"`
	CreatedAt time.Time `json:"createdAt"`
}

func (s *Store) CreateDecision(ctx context.Context, d Decision) (Decision, error) {
	if d.Body == "" {
		return Decision{}, fmt.Errorf("%w: decision body is required", ErrInvalid)
	}
	if d.Author == "" {
		return Decision{}, fmt.Errorf("%w: decision author is required", ErrInvalid)
	}
	if d.MessageID != nil {
		var ok bool
		err := s.db.QueryRowContext(ctx,
			`SELECT EXISTS (SELECT 1 FROM message m JOIN channel c ON c.id = m.channel_id
			 WHERE m.id = ? AND c.feature_id = ?)`,
			*d.MessageID, d.FeatureID).Scan(&ok)
		if err != nil {
			return Decision{}, fmt.Errorf("check message: %w", err)
		}
		if !ok {
			return Decision{}, fmt.Errorf("%w: message %d is not in a channel of feature %d", ErrInvalid, *d.MessageID, d.FeatureID)
		}
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO decision (feature_id, message_id, body, why, author, created_at) VALUES (?, ?, ?, ?, ?, ?)",
		d.FeatureID, d.MessageID, d.Body, d.Why, d.Author, now.Format(time.RFC3339Nano))
	if isForeignKey(err) {
		return Decision{}, fmt.Errorf("%w: unknown feature %d", ErrInvalid, d.FeatureID)
	}
	if err != nil {
		return Decision{}, fmt.Errorf("insert decision: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Decision{}, fmt.Errorf("insert decision: %w", err)
	}
	d.ID, d.CreatedAt = id, now
	return d, nil
}

func (s *Store) Decision(ctx context.Context, id int64) (Decision, error) {
	row := s.db.QueryRowContext(ctx,
		"SELECT id, feature_id, message_id, body, why, author, created_at FROM decision WHERE id = ?", id)
	d, err := scanDecision(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Decision{}, fmt.Errorf("decision %d: %w", id, ErrNotFound)
	}
	return d, err
}

// Decisions lists the feature's decisions, oldest first.
func (s *Store) Decisions(ctx context.Context, featureID int64) ([]Decision, error) {
	if err := s.requireFeature(ctx, featureID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, feature_id, message_id, body, why, author, created_at FROM decision WHERE feature_id = ? ORDER BY id",
		featureID)
	if err != nil {
		return nil, fmt.Errorf("list decisions: %w", err)
	}
	defer rows.Close()
	var ds []Decision
	for rows.Next() {
		d, err := scanDecision(rows)
		if err != nil {
			return nil, err
		}
		ds = append(ds, d)
	}
	return ds, rows.Err()
}

func scanDecision(r interface{ Scan(...any) error }) (Decision, error) {
	var d Decision
	var created string
	if err := r.Scan(&d.ID, &d.FeatureID, &d.MessageID, &d.Body, &d.Why, &d.Author, &created); err != nil {
		return Decision{}, err
	}
	t, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return Decision{}, fmt.Errorf("decision %d: bad created_at: %w", d.ID, err)
	}
	d.CreatedAt = t
	return d, nil
}
