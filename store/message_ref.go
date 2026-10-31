package store

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strconv"
)

// _spaceRef matches the token the composer writes for a selected Space: #[Name](space:ID).
// The id is the identity; the name is only what the user saw.
var _spaceRef = regexp.MustCompile(`#\[[^\]\n]*\]\(space:(\d+)\)`)

// SpaceRef is a Space a message refers to. Available is false when the Space has since been
// deleted.
type SpaceRef struct {
	SpaceID   int64  `json:"spaceId"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Closed    bool   `json:"closed"`
	Available bool   `json:"available"`
}

// ParseSpaceRefs returns the Space ids that body refers to, each once, in order.
func ParseSpaceRefs(body string) []int64 {
	var ids []int64
	seen := map[int64]bool{}
	for _, m := range _spaceRef.FindAllStringSubmatch(body, -1) {
		id, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids
}

// SaveMessageRefs records that the message refers to the Spaces. A reference survives the
// deletion of the Space it names. CreateMessage does it for a user message.
func (s *Store) SaveMessageRefs(ctx context.Context, messageID int64, spaceIDs []int64) error {
	return saveMessageRefs(ctx, s.db, messageID, spaceIDs)
}

func saveMessageRefs(ctx context.Context, db execer, messageID int64, spaceIDs []int64) error {
	for _, id := range spaceIDs {
		if _, err := db.ExecContext(ctx,
			"INSERT OR IGNORE INTO message_ref (message_id, space_id) VALUES (?, ?)",
			messageID, id); err != nil {
			return fmt.Errorf("save message ref: %w", err)
		}
	}
	return nil
}

// MessageRefs lists the Spaces the message refers to, by Space id.
func (s *Store) MessageRefs(ctx context.Context, messageID int64) ([]SpaceRef, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT r.space_id, sp.name, sp.kind, sp.closed_at IS NOT NULL, sp.id IS NOT NULL "+
			"FROM message_ref r LEFT JOIN space sp ON sp.id = r.space_id "+
			"WHERE r.message_id = ? ORDER BY r.space_id", messageID)
	if err != nil {
		return nil, fmt.Errorf("list message refs: %w", err)
	}
	return scanAll(rows, func(r interface{ Scan(...any) error }) (SpaceRef, error) {
		var ref SpaceRef
		var name, kind sql.NullString
		err := r.Scan(&ref.SpaceID, &name, &kind, &ref.Closed, &ref.Available)
		ref.Name, ref.Kind = name.String, kind.String
		return ref, err
	})
}

// SpaceReferenced reports whether a message of the Space in refers to the Space ref.
func (s *Store) SpaceReferenced(ctx context.Context, in, ref int64) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM message_ref r JOIN message m ON m.id = r.message_id "+
			"WHERE m.space_id = ? AND r.space_id = ?", in, ref).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("check space ref: %w", err)
	}
	return n > 0, nil
}
