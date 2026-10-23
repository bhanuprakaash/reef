package memory

import (
	"context"
	"fmt"
	"strings"
	"unicode"
)

// Filter limits what Search returns. Only items in one of Scopes are
// returned; an empty Scopes returns nothing.
type Filter struct {
	Scopes         []Ref
	IncludeRetired bool
}

// Hit is one search result. A higher Score is a better match.
type Hit struct {
	ID     int64
	Score  float64
	Status string
}

// Search returns accepted items that match any word of query, best first.
// Retired items are included, marked by Status, when f.IncludeRetired is set.
// Candidates are never returned.
func (s *Store) Search(ctx context.Context, query string, f Filter) ([]Hit, error) {
	terms := strings.FieldsFunc(strings.ToLower(query), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	if len(terms) == 0 || len(f.Scopes) == 0 {
		return nil, nil
	}
	for i, t := range terms {
		terms[i] = `"` + t + `"`
	}

	statuses := []any{StatusAccepted}
	if f.IncludeRetired {
		statuses = append(statuses, StatusRetired)
	}
	args := []any{strings.Join(terms, " OR ")}
	args = append(args, statuses...)

	scopes := make([]string, len(f.Scopes))
	for i, r := range f.Scopes {
		scopes[i] = `(m.scope_kind = ? AND m.scope_id = ?)`
		args = append(args, r.Kind, r.ID)
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT m.id, -bm25(memory_fts), m.status
		 FROM memory_fts JOIN memory m ON m.id = memory_fts.rowid
		 WHERE memory_fts MATCH ?
		   AND m.status IN (?`+strings.Repeat(", ?", len(statuses)-1)+`)
		   AND (`+strings.Join(scopes, " OR ")+`)
		 ORDER BY bm25(memory_fts), m.id`, args...)
	if err != nil {
		return nil, fmt.Errorf("search memory: %w", err)
	}
	defer rows.Close()
	var hits []Hit
	for rows.Next() {
		var h Hit
		if err := rows.Scan(&h.ID, &h.Score, &h.Status); err != nil {
			return nil, fmt.Errorf("scan hit: %w", err)
		}
		hits = append(hits, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read hits: %w", err)
	}
	return hits, nil
}
