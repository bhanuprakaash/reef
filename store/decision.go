package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const _insertDecision = "INSERT INTO decision " +
	"(feature_id, message_id, body, why, author, created_at) VALUES (?, ?, ?, ?, ?, ?)"

const _decisionCols = "id, feature_id, message_id, body, why, author, created_at, " +
	"retired_at, replaced_by"

// Decision is never edited or deleted, only retired: RetiredAt is set when it is stale or
// replaced, and ReplacedBy is the decision that replaced it, if one did. Replaces is input
// to CreateDecision only: the live decisions the new one retires.
type Decision struct {
	ID         int64      `json:"id"`
	FeatureID  int64      `json:"featureId"`
	MessageID  *int64     `json:"messageId"`
	Body       string     `json:"body"`
	Why        string     `json:"why"`
	Author     string     `json:"author"`
	CreatedAt  time.Time  `json:"createdAt"`
	RetiredAt  *time.Time `json:"retiredAt"`
	ReplacedBy *int64     `json:"replacedBy"`
	Replaces   []int64    `json:"replaces,omitempty"`
}

// CreateDecision adds d and retires the decisions in d.Replaces, in one transaction. A set
// MessageID must be a message in a channel of d.FeatureID; each Replaces id must be a live
// decision of d.FeatureID, or nothing is written.
func (s *Store) CreateDecision(ctx context.Context, d Decision) (Decision, error) {
	if d.Body == "" {
		return Decision{}, fmt.Errorf("%w: decision body is required", ErrInvalid)
	}
	if d.Author == "" {
		return Decision{}, fmt.Errorf("%w: decision author is required", ErrInvalid)
	}
	if d.FeatureID == 0 {
		return Decision{}, fmt.Errorf("%w: feature is required", ErrInvalid)
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
			return Decision{}, fmt.Errorf("%w: message %d is not in a channel of feature %d",
				ErrInvalid, *d.MessageID, d.FeatureID)
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Decision{}, fmt.Errorf("begin decision: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := requireLiveDecisions(ctx, tx, d.FeatureID, d.Replaces); err != nil {
		return Decision{}, err
	}
	now := time.Now().UTC()
	at := now.Format(time.RFC3339Nano)
	res, err := tx.ExecContext(ctx, _insertDecision,
		d.FeatureID, d.MessageID, d.Body, d.Why, d.Author, at)
	if err != nil {
		return Decision{}, s.writeErr(ctx, "insert decision", err, ErrInvalid,
			parent{table: "feature", id: &d.FeatureID}, parent{table: "message", id: d.MessageID})
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Decision{}, fmt.Errorf("insert decision: %w", err)
	}
	if err := retireDecisions(ctx, tx, d.FeatureID, d.Replaces, &id, at); err != nil {
		return Decision{}, err
	}
	if err := tx.Commit(); err != nil {
		return Decision{}, fmt.Errorf("commit decision: %w", err)
	}
	d.ID, d.CreatedAt = id, now
	return d, nil
}

// requireLiveDecisions returns ErrInvalid for an id that is not a live decision of the feature.
func requireLiveDecisions(
	ctx context.Context, q interface {
		QueryRowContext(context.Context, string, ...any) *sql.Row
	}, featureID int64, ids []int64,
) error {
	for _, id := range ids {
		var ok bool
		err := q.QueryRowContext(ctx,
			"SELECT EXISTS (SELECT 1 FROM decision WHERE id = ? AND feature_id = ? AND retired_at IS NULL)",
			id, featureID).Scan(&ok)
		if err != nil {
			return fmt.Errorf("check decision %d: %w", id, err)
		}
		if !ok {
			return fmt.Errorf("%w: decision %d is not a live decision of feature %d",
				ErrInvalid, id, featureID)
		}
	}
	return nil
}

// retireDecisions retires the live ones among ids and skips a decision of the feature that is
// already retired. It returns ErrInvalid for an id that is not a decision of the feature.
func retireDecisions(
	ctx context.Context, tx *sql.Tx, featureID int64, ids []int64, replacedBy *int64, at string,
) error {
	for _, id := range ids {
		res, err := tx.ExecContext(ctx,
			"UPDATE decision SET retired_at = ?, replaced_by = ? "+
				"WHERE id = ? AND feature_id = ? AND retired_at IS NULL",
			at, replacedBy, id, featureID)
		if err != nil {
			return fmt.Errorf("retire decision %d: %w", id, err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("retire decision %d: %w", id, err)
		}
		if n > 0 {
			continue
		}
		var exists bool
		err = tx.QueryRowContext(ctx,
			"SELECT EXISTS (SELECT 1 FROM decision WHERE id = ? AND feature_id = ?)",
			id, featureID).Scan(&exists)
		if err != nil {
			return fmt.Errorf("check decision %d: %w", id, err)
		}
		if !exists {
			return fmt.Errorf("%w: decision %d is not a live decision of feature %d",
				ErrInvalid, id, featureID)
		}
	}
	return nil
}

// Decision returns the decision with the id, or ErrNotFound.
func (s *Store) Decision(ctx context.Context, id int64) (Decision, error) {
	row := s.db.QueryRowContext(ctx,
		"SELECT "+_decisionCols+" FROM decision WHERE id = ?", id)
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
		"SELECT "+_decisionCols+" FROM decision WHERE feature_id = ? ORDER BY id",
		featureID)
	if err != nil {
		return nil, fmt.Errorf("list decisions: %w", err)
	}
	return scanAll(rows, scanDecision)
}

func scanDecision(r interface{ Scan(...any) error }) (Decision, error) {
	var d Decision
	var created string
	var retired sql.NullString
	if err := r.Scan(
		&d.ID, &d.FeatureID, &d.MessageID, &d.Body, &d.Why, &d.Author, &created,
		&retired, &d.ReplacedBy,
	); err != nil {
		return Decision{}, err
	}
	t, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return Decision{}, fmt.Errorf("decision %d: bad created_at: %w", d.ID, err)
	}
	d.CreatedAt = t
	if retired.Valid {
		rt, err := time.Parse(time.RFC3339Nano, retired.String)
		if err != nil {
			return Decision{}, fmt.Errorf("decision %d: bad retired_at: %w", d.ID, err)
		}
		d.RetiredAt = &rt
	}
	return d, nil
}
