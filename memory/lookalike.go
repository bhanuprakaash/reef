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

func (s *Store) lookalikes(ctx context.Context, w Writer, n Note, vec []float32) ([]Similar, error) {
	f := Filter{Scopes: []Ref{w.Scope}}
	var out []Similar
	if vec != nil {
		hits, err := s.scoreVectors(ctx, vec, f)
		if err != nil {
			return nil, err
		}
		for _, h := range hits {
			if h.Score < _similarCosine {
				break
			}
			if slices.Contains(n.Replaces, h.ID) {
				continue
			}
			body, _, err := s.body(ctx, h.ID)
			if err != nil {
				return nil, err
			}
			out = append(out, Similar{ID: h.ID, Body: body})
		}
		where, args := f.where()
		var pending bool
		err = s.db.QueryRowContext(ctx,
			`SELECT EXISTS (SELECT 1 FROM memory WHERE vector IS NULL AND `+where+`)`, args...).
			Scan(&pending)
		if err != nil {
			return nil, fmt.Errorf("query unembedded in scope: %w", err)
		}
		if !pending {
			return out, nil
		}
	}

	hits, err := s.Search(ctx, n.Body, f)
	if err != nil {
		return nil, err
	}
	want := slices.Compact(slices.Sorted(slices.Values(terms(n.Body))))
	for _, h := range hits {
		if slices.Contains(n.Replaces, h.ID) {
			continue
		}
		body, hasVector, err := s.body(ctx, h.ID)
		if err != nil {
			return nil, err
		}
		if vec != nil && hasVector {
			continue
		}
		have := terms(body)
		found := 0
		for _, t := range want {
			if slices.Contains(have, t) {
				found++
			}
		}
		if float64(found)/float64(len(want)) >= _similarTermShare {
			out = append(out, Similar{ID: h.ID, Body: body})
		}
		break
	}
	return out, nil
}

func (s *Store) body(ctx context.Context, id int64) (string, bool, error) {
	var (
		body      string
		hasVector bool
	)
	err := s.db.QueryRowContext(ctx, `SELECT body, vector IS NOT NULL FROM memory WHERE id = ?`, id).
		Scan(&body, &hasVector)
	if err != nil {
		return "", false, fmt.Errorf("read body of %d: %w", id, err)
	}
	return body, hasVector, nil
}
