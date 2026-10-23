package memory

import (
	"context"
	"fmt"
	"slices"
)

const (
	_similarCosine    = 0.92
	_similarTermShare = 0.7
)

// Similar is a stored item that looks like a note being written.
type Similar struct {
	ID   int64
	Body string
}

// lookalikes returns the accepted items in w's scope that look like n, best
// first, never ones n replaces. With a vector it is every item at or above the
// cosine threshold. Without one it is the top word match, when it holds enough
// of n's words.
func (s *Store) lookalikes(ctx context.Context, w Writer, n Note, vec []float32) ([]Similar, error) {
	f := Filter{Scopes: []Ref{w.Scope}}
	if vec != nil {
		hits, err := s.scoreVectors(ctx, vec, f)
		if err != nil {
			return nil, err
		}
		var out []Similar
		for _, h := range hits {
			if h.Score < _similarCosine {
				break
			}
			if slices.Contains(n.Replaces, h.ID) {
				continue
			}
			body, err := s.body(ctx, h.ID)
			if err != nil {
				return nil, err
			}
			out = append(out, Similar{ID: h.ID, Body: body})
		}
		return out, nil
	}

	hits, err := s.Search(ctx, n.Body, f)
	if err != nil {
		return nil, err
	}
	hits = slices.DeleteFunc(hits, func(h Match) bool { return slices.Contains(n.Replaces, h.ID) })
	if len(hits) == 0 {
		return nil, nil
	}
	body, err := s.body(ctx, hits[0].ID)
	if err != nil {
		return nil, err
	}
	have := terms(body)
	want := slices.Compact(slices.Sorted(slices.Values(terms(n.Body))))
	found := 0
	for _, t := range want {
		if slices.Contains(have, t) {
			found++
		}
	}
	if float64(found)/float64(len(want)) < _similarTermShare {
		return nil, nil
	}
	return []Similar{{ID: hits[0].ID, Body: body}}, nil
}

func (s *Store) body(ctx context.Context, id int64) (string, error) {
	var body string
	err := s.db.QueryRowContext(ctx, `SELECT body FROM memory WHERE id = ?`, id).Scan(&body)
	if err != nil {
		return "", fmt.Errorf("read body of %d: %w", id, err)
	}
	return body, nil
}
