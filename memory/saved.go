package memory

import (
	"context"
	"fmt"
	"strings"
)

func ValidKind(kind string) bool { return _kinds[kind] }

// Saved reports whether scope holds an accepted or candidate item whose body is exactly body,
// not counting the items with the ids in except.
func (s *Store) Saved(
	ctx context.Context, scope Ref, body string, except []int64,
) (bool, error) {
	args := []any{scope.Kind, scope.ID, body, StatusAccepted, StatusCandidate}
	skip := ""
	if len(except) > 0 {
		skip = ` AND id NOT IN (?` + strings.Repeat(", ?", len(except)-1) + `)`
		for _, id := range except {
			args = append(args, id)
		}
	}
	var found bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM memory
		 WHERE scope_kind = ? AND scope_id = ? AND body = ? AND status IN (?, ?)`+skip+`)`,
		args...).Scan(&found)
	if err != nil {
		return false, fmt.Errorf("check saved: %w", err)
	}
	return found, nil
}
