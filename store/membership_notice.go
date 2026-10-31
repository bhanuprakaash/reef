package store

import (
	"context"
	"fmt"
	"time"
)

// The notices Reef posts in a Space when a Bot joins or leaves it, after the Bot's name.
const (
	JoinedSuffix = " joined the Space"
	LeftSuffix   = " left the Space"
)

// MembershipNotices returns the bodies of the join and leave notices of Space id posted after
// since, oldest first.
func (s *Store) MembershipNotices(ctx context.Context, id int64, since time.Time) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT body FROM message WHERE space_id = ? AND author = ? AND created_at > ? "+
			"AND (body LIKE '%' || ? OR body LIKE '%' || ?) ORDER BY id",
		id, ReefAuthor, since.UTC().Format(time.RFC3339Nano), JoinedSuffix, LeftSuffix)
	if err != nil {
		return nil, fmt.Errorf("list membership notices: %w", err)
	}
	return scanAll(rows, func(r interface{ Scan(...any) error }) (body string, err error) {
		return body, r.Scan(&body)
	})
}
