package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Setting is the value stored under key, or "" when none is set.
func (s *Store) Setting(ctx context.Context, key string) (string, error) {
	var v string
	err := s.db.QueryRowContext(ctx, "SELECT value FROM setting WHERE key = ?", key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read setting %s: %w", key, err)
	}
	return v, nil
}

// SetSettings stores every key and value in one transaction, replacing any
// before: all of them are saved or none.
func (s *Store) SetSettings(ctx context.Context, settings map[string]string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin settings: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for key, value := range settings {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO setting (key, value) VALUES (?, ?) "+
				"ON CONFLICT(key) DO UPDATE SET value = excluded.value", key, value); err != nil {
			return fmt.Errorf("write setting %s: %w", key, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit settings: %w", err)
	}
	return nil
}
