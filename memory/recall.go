package memory

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
)

const (
	_defaultLimit = 10
	_candidates   = 50
	_rrfK         = 60
	_viaDirect    = "direct"
)

// Query is what a caller asks Recall. Scope, when set, narrows to one of Scopes; if it is
// not one of them, nothing matches. A Limit of zero or less means 10.
type Query struct {
	Scopes []Ref
	Text   string
	Limit  int
	Scope  *Ref
}

// Hit is one recalled item. Via is "direct", or the link it was reached through, such as
// "replaces memory:4".
type Hit struct {
	ID         int64
	Kind, Body string
	Scope      Ref
	Status     string
	Score      float64
	Via        string
}

type neighbour struct {
	id  int64
	via string
}

// Recall fuses word and meaning matches by reciprocal rank fusion, then adds items one
// link away from the best, each ranked one place below the hit it came from. Links to a
// project or team are never followed: they reach everything on the shelf.
func (s *Store) Recall(ctx context.Context, q Query) ([]Hit, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = _defaultLimit
	}
	scopes := q.Scopes
	if q.Scope != nil {
		if !slices.Contains(scopes, *q.Scope) {
			return nil, nil
		}
		scopes = []Ref{*q.Scope}
	}
	f := Filter{Scopes: scopes, IncludeRetired: true}

	words, err := s.Search(ctx, q.Text, f)
	if err != nil {
		return nil, err
	}
	// A failing embedder must not fail recall: words alone answer.
	meaning, _ := s.Nearest(ctx, q.Text, f, _candidates)

	found := make(map[int64]*Hit)
	for _, list := range [][]Match{words, meaning} {
		for i, m := range list[:min(len(list), _candidates)] {
			c := found[m.ID]
			if c == nil {
				c = &Hit{ID: m.ID, Status: m.Status, Via: _viaDirect}
				found[m.ID] = c
			}
			c.Score += 1 / float64(_rrfK+i+1)
		}
	}
	if len(found) == 0 {
		return nil, nil
	}

	direct := make([]*Hit, 0, len(found))
	for _, c := range found {
		direct = append(direct, c)
	}
	sort.Slice(direct, func(i, j int) bool { return before(*direct[i], *direct[j]) })
	for _, p := range direct[:min(limit, len(direct))] {
		ns, err := s.neighbours(ctx, p.ID)
		if err != nil {
			return nil, err
		}
		below := p.Score / (1 + p.Score)
		for _, n := range ns {
			if c, ok := found[n.id]; ok && c.Score >= below {
				continue
			}
			found[n.id] = &Hit{ID: n.id, Score: below, Via: n.via}
		}
	}

	hits, err := s.load(ctx, f, found)
	if err != nil {
		return nil, err
	}
	sort.Slice(hits, func(i, j int) bool { return before(hits[i], hits[j]) })
	return hits[:min(limit, len(hits))], nil
}

func (s *Store) neighbours(ctx context.Context, id int64) ([]neighbour, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT CAST(to_id AS INTEGER), rel || ' memory:' || to_id
		 FROM memory_link WHERE from_id = ? AND to_kind = 'memory'
		 UNION ALL
		 SELECT from_id, rel || ' memory:' || to_id
		 FROM memory_link WHERE to_kind = 'memory' AND to_id = ?
		 UNION ALL
		 SELECT o.from_id, p.rel || ' ' || p.to_kind || ':' || p.to_id
		 FROM memory_link p JOIN memory_link o ON o.to_kind = p.to_kind AND o.to_id = p.to_id
		 WHERE p.from_id = ? AND o.from_id != p.from_id AND p.to_kind NOT IN ('memory', ?, ?)`,
		id, strconv.FormatInt(id, 10), id, ScopeProject, ScopeTeam)
	if err != nil {
		return nil, fmt.Errorf("query neighbours of %d: %w", id, err)
	}
	defer rows.Close()
	var out []neighbour
	for rows.Next() {
		var n neighbour
		if err := rows.Scan(&n.id, &n.via); err != nil {
			return nil, fmt.Errorf("scan neighbour: %w", err)
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read neighbours of %d: %w", id, err)
	}
	return out, nil
}

func (s *Store) load(ctx context.Context, f Filter, found map[int64]*Hit) ([]Hit, error) {
	where, args := f.where()
	marks := make([]string, 0, len(found))
	idArgs := make([]any, 0, len(found)+len(args))
	for id := range found {
		marks = append(marks, "?")
		idArgs = append(idArgs, id)
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, kind, body, scope_kind, scope_id, status FROM memory
		 WHERE id IN (`+strings.Join(marks, ",")+`) AND `+where,
		append(idArgs, args...)...)
	if err != nil {
		return nil, fmt.Errorf("load hits: %w", err)
	}
	defer rows.Close()
	hits := make([]Hit, 0, len(found))
	for rows.Next() {
		var h Hit
		if err := rows.Scan(&h.ID, &h.Kind, &h.Body, &h.Scope.Kind, &h.Scope.ID, &h.Status); err != nil {
			return nil, fmt.Errorf("scan hit: %w", err)
		}
		h.Score, h.Via = found[h.ID].Score, found[h.ID].Via
		hits = append(hits, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read hits: %w", err)
	}
	return hits, nil
}

func before(a, b Hit) bool {
	if a.Score != b.Score {
		return a.Score > b.Score
	}
	if (a.Status == StatusAccepted) != (b.Status == StatusAccepted) {
		return a.Status == StatusAccepted
	}
	return a.ID > b.ID
}
