package memory

import (
	"context"
	"fmt"
	"strings"
	"unicode"
)

// Search returns items that match any word of query, best first; ties go to accepted
// before retired, then newer. Retired items appear only when f.IncludeRetired is set.
func (s *Store) Search(ctx context.Context, query string, f Filter) ([]Match, error) {
	words := terms(query)
	if len(words) == 0 || len(f.Scopes) == 0 {
		return nil, nil
	}
	for i, t := range words {
		words[i] = `"` + t + `"`
	}

	where, args := f.where()
	rows, err := s.db.QueryContext(ctx,
		`SELECT m.id, -bm25(memory_fts), m.status
		 FROM memory_fts JOIN memory m ON m.id = memory_fts.rowid
		 WHERE memory_fts MATCH ? AND `+where+`
		 ORDER BY bm25(memory_fts), m.status = ? DESC, m.id DESC`,
		append(append([]any{strings.Join(words, " OR ")}, args...), StatusAccepted)...)
	if err != nil {
		return nil, fmt.Errorf("search memory: %w", err)
	}
	defer rows.Close()
	var hits []Match
	for rows.Next() {
		var h Match
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

func terms(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}
