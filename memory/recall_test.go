package memory

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
)

func put(t *testing.T, s *Store, scope Ref, n Note) int64 {
	t.Helper()
	w := Writer{Author: "ada", Role: "developer", Scope: scope}
	res, err := s.Remember(context.Background(), w, Note{
		Kind: KindFinding, Body: n.Body, About: n.About, Replaces: n.Replaces,
	})
	if err != nil {
		t.Fatal(err)
	}
	return res.Item.ID
}

func recall(t *testing.T, s *Store, q Query) []Hit {
	t.Helper()
	hits, err := s.Recall(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	return hits
}

func vias(hits []Hit) []string {
	out := make([]string, len(hits))
	for i, h := range hits {
		out[i] = h.Via
	}
	return out
}

func TestRecall_NoneEmbedderIsSearchPlusExpansion(t *testing.T) {
	s, _ := openTest(t)
	shelf := Ref{Kind: ScopeProject, ID: "1"}
	elsewhere := Ref{Kind: ScopeProject, ID: "2"}
	task := Ref{Kind: ScopeWorkItem, ID: "12"}
	put(t, s, shelf, Note{Body: "cache the token", About: []Ref{task}})
	put(t, s, shelf, Note{Body: "refresh before expiry", About: []Ref{task}})
	put(t, s, shelf, Note{Body: "cache the session"})
	put(t, s, shelf, Note{Body: "unrelated deploy note", About: []Ref{{Kind: ScopeWorkItem, ID: "99"}}})
	put(t, s, elsewhere, Note{Body: "rotate keys", About: []Ref{task}})

	scopes := []Ref{shelf}
	words, err := s.Search(context.Background(), "cache", Filter{Scopes: scopes, IncludeRetired: true})
	if err != nil {
		t.Fatal(err)
	}
	want := append(ids(words), 2)
	hits := recall(t, s, Query{Scopes: scopes, Text: "cache"})
	if got := hitIDs(hits); !reflect.DeepEqual(got, want) || !reflect.DeepEqual(want, []int64{3, 1, 2}) {
		t.Fatalf("ids = %v, want search ids + expansion %v", got, want)
	}
	if got, want := vias(hits), []string{"direct", "direct", "about work_item:12"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("via = %v, want %v", got, want)
	}
	if h := hits[2]; h.Kind != KindFinding || h.Body != "refresh before expiry" ||
		h.Scope != shelf || h.Status != StatusAccepted || h.Score <= 0 {
		t.Fatalf("expanded hit = %+v", h)
	}
}

func hitIDs(hits []Hit) []int64 {
	out := make([]int64, len(hits))
	for i, h := range hits {
		out[i] = h.ID
	}
	return out
}

func TestRecall_ExpansionReachesReplacedItem(t *testing.T) {
	s, _ := openTest(t)
	shelf := Ref{Kind: ScopeProject, ID: "1"}
	put(t, s, shelf, Note{Body: "retry three times on timeout"})
	put(t, s, shelf, Note{Body: "retry five times on failure", Replaces: []int64{1}})
	scopes := []Ref{shelf}

	hits := recall(t, s, Query{Scopes: scopes, Text: "failure"})
	if got, want := hitIDs(hits), []int64{2, 1}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ids = %v, want %v", got, want)
	}
	if got, want := vias(hits), []string{"direct", "replaces memory:1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("via = %v, want %v", got, want)
	}
	if hits[1].Status != StatusRetired {
		t.Fatalf("status = %q, want retired", hits[1].Status)
	}

	hits = recall(t, s, Query{Scopes: scopes, Text: "timeout"})
	if got, want := hitIDs(hits), []int64{1, 2}; !reflect.DeepEqual(got, want) {
		t.Fatalf("from the old item: ids = %v, want %v", got, want)
	}
	if got, want := vias(hits), []string{"direct", "replaces memory:1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("from the old item: via = %v, want %v", got, want)
	}
	if hits[0].Status != StatusRetired || hits[1].Status != StatusAccepted {
		t.Fatalf("statuses = %q, %q", hits[0].Status, hits[1].Status)
	}
}

func TestRecall_ExpansionSkipsHubs(t *testing.T) {
	s, _ := openTest(t)
	shelf := Ref{Kind: ScopeProject, ID: "1"}
	for i := 1; i <= 50; i++ {
		put(t, s, shelf, Note{
			Body:  fmt.Sprintf("entry %d", i),
			About: []Ref{shelf, {Kind: ScopeWorkItem, ID: fmt.Sprint(i)}},
		})
	}
	put(t, s, shelf, Note{
		Body: "unrelated words", About: []Ref{{Kind: ScopeWorkItem, ID: "7"}},
	})

	hits := recall(t, s, Query{Scopes: []Ref{shelf}, Text: "7", Limit: 100})
	if got, want := hitIDs(hits), []int64{7, 51}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ids = %v, want %v", got, want)
	}
	if got, want := vias(hits), []string{"direct", "about work_item:7"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("via = %v, want %v", got, want)
	}
}

func TestRecall_NewestAcceptedConflictFirst(t *testing.T) {
	s, _ := openTest(t)
	shelf := Ref{Kind: ScopeProject, ID: "1"}
	put(t, s, shelf, Note{Body: "the deploy window is monday"})
	put(t, s, shelf, Note{Body: "the deploy window is friday"})

	hits := recall(t, s, Query{Scopes: []Ref{shelf}, Text: "deploy window"})
	if got, want := hitIDs(hits), []int64{2, 1}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ids = %v, want %v", got, want)
	}
}

func TestRecall_LiveBeforeNewerRetiredOnEqualScore(t *testing.T) {
	s, _ := openTest(t)
	shelf := Ref{Kind: ScopeProject, ID: "1"}
	live := put(t, s, shelf, Note{Body: "Deploys go out on Tuesdays"})
	stale := put(t, s, shelf, Note{Body: "Deploys go out on Fridays"})
	if err := s.Retire(context.Background(), stale); err != nil {
		t.Fatal(err)
	}

	hits := recall(t, s, Query{Scopes: []Ref{shelf}, Text: "deploys"})
	if got, want := hitIDs(hits), []int64{live, stale}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ids = %v, want %v", got, want)
	}
}

func TestRecall_NeverExceedsLimit(t *testing.T) {
	s, _ := openTest(t)
	shelf := Ref{Kind: ScopeProject, ID: "1"}
	task := Ref{Kind: ScopeWorkItem, ID: "1"}
	for i := 1; i <= 15; i++ {
		put(t, s, shelf, Note{Body: fmt.Sprintf("cache entry %d", i), About: []Ref{task}})
	}
	put(t, s, shelf, Note{Body: "other", About: []Ref{task}})

	tests := []struct {
		name string
		give int
		want []int64
	}{
		{"set", 3, []int64{15, 14, 13}},
		{"default", 0, []int64{15, 14, 13, 12, 11, 10, 9, 8, 7, 6}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hits := recall(t, s, Query{Scopes: []Ref{shelf}, Text: "cache", Limit: tt.give})
			if got := hitIDs(hits); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ids = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRecall_FusesMeaningMatches(t *testing.T) {
	s, _ := openTest(t)
	s.embed = &fakeEmbedder{vectors: map[string][]float32{
		"cache the token":         {1, 0},
		"refresh the credentials": {0.9, 0.1},
		"deploy on friday":        {0, 1},
		"cache":                   {1, 0},
	}}
	shelf := Ref{Kind: ScopeProject, ID: "1"}
	put(t, s, shelf, Note{Body: "cache the token"})
	put(t, s, shelf, Note{Body: "refresh the credentials"})
	put(t, s, shelf, Note{Body: "deploy on friday"})

	hits := recall(t, s, Query{Scopes: []Ref{shelf}, Text: "cache"})
	if got, want := hitIDs(hits), []int64{1, 2, 3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ids = %v, want %v", got, want)
	}
	if got, want := vias(hits), []string{"direct", "direct", "direct"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("via = %v, want %v", got, want)
	}
}

func TestRecall_EmbedFailureFallsBackToWords(t *testing.T) {
	s, _ := openTest(t)
	s.embed = &fakeEmbedder{err: errors.New("down")}
	shelf := Ref{Kind: ScopeProject, ID: "1"}
	put(t, s, shelf, Note{Body: "cache the token"})

	hits := recall(t, s, Query{Scopes: []Ref{shelf}, Text: "cache"})
	if got, want := hitIDs(hits), []int64{1}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ids = %v, want %v", got, want)
	}
}

func TestRecall_ScopeRestriction(t *testing.T) {
	s, _ := openTest(t)
	a := Ref{Kind: ScopeProject, ID: "1"}
	b := Ref{Kind: ScopeProject, ID: "2"}
	hidden := Ref{Kind: ScopeProject, ID: "3"}
	put(t, s, a, Note{Body: "cache the token"})
	put(t, s, b, Note{Body: "cache the session"})
	put(t, s, hidden, Note{Body: "cache the page"})

	tests := []struct {
		name string
		give Query
		want []int64
	}{
		{"all visible", Query{Scopes: []Ref{a, b}, Text: "cache"}, []int64{2, 1}},
		{"restricted", Query{Scopes: []Ref{a, b}, Text: "cache", Scope: &b}, []int64{2}},
		{"restricted to hidden", Query{Scopes: []Ref{a, b}, Text: "cache", Scope: &hidden}, []int64{}},
		{"none visible", Query{Text: "cache"}, []int64{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hitIDs(recall(t, s, tt.give)); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ids = %v, want %v", got, tt.want)
			}
		})
	}
}
