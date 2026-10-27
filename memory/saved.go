package memory

import (
	"context"
	"fmt"
)

func ValidKind(kind string) bool { return _kinds[kind] }

// Saved reports whether scope holds an accepted or candidate item whose body is exactly body.
func (s *Store) Saved(ctx context.Context, scope Ref, body string) (bool, error) {
	var found bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM memory
		 WHERE scope_kind = ? AND scope_id = ? AND body = ? AND status IN (?, ?))`,
		scope.Kind, scope.ID, body, StatusAccepted, StatusCandidate).Scan(&found)
	if err != nil {
		return false, fmt.Errorf("check saved: %w", err)
	}
	return found, nil
}
