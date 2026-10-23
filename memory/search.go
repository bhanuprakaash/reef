package memory

import (
	"context"
	"fmt"
	"strings"
	"unicode"
)

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

	where, args := f.where()
	rows, err := s.db.QueryContext(ctx,
		`SELECT m.id, -bm25(memory_fts), m.status
		 FROM memory_fts JOIN memory m ON m.id = memory_fts.rowid
		 WHERE memory_fts MATCH ? AND `+where+`
		 ORDER BY bm25(memory_fts), m.id`,
		append([]any{strings.Join(terms, " OR ")}, args...)...)
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
