package memory

import (
	"context"
	"reflect"
	"sort"
	"testing"
)

func seedFact(t *testing.T, s *Store, scope Ref, body string) {
	t.Helper()
	w := Writer{Author: "ada", Role: "developer", Scope: scope}
	if _, err := s.Remember(context.Background(), w, Note{Kind: KindFact, Body: body}); err != nil {
		t.Fatal(err)
	}
}

func sortedHitIDs(hits []Match) []int64 {
	out := make([]int64, len(hits))
	for i, h := range hits {
		out[i] = h.ID
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func TestSearch_StemsWords(t *testing.T) {
	s, _ := openTest(t)
	shelf := Ref{Kind: ScopeProject, ID: "1"}
	seedFact(t, s, shelf, "the client will retry on timeout")
	seedFact(t, s, shelf, "use sqlite for storage")

	hits, err := s.Search(context.Background(), "retries?", Filter{Scopes: []Ref{shelf}})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].ID != 1 || hits[0].Status != StatusAccepted || hits[0].Score <= 0 {
		t.Fatalf("hits = %+v, want only id 1, accepted, positive score", hits)
	}
}

func TestSearch_OnlyAllowedScopes(t *testing.T) {
	s, _ := openTest(t)
	a := Ref{Kind: ScopeProject, ID: "1"}
	b := Ref{Kind: ScopeProject, ID: "2"}
	seedFact(t, s, a, "cache the token")
	seedFact(t, s, b, "cache the session")
	seedFact(t, s, Ref{Kind: ScopeTeam, ID: "1"}, "cache the page")
	seedFact(t, s, Ref{Kind: ScopeProject}, "cache the row")

	for _, tc := range []struct {
		name   string
		scopes []Ref
		want   []int64
	}{
		{"one shelf", []Ref{a}, []int64{1}},
		{"two shelves", []Ref{a, b}, []int64{1, 2}},
		{"empty id is not a wildcard", []Ref{{Kind: ScopeProject}}, []int64{4}},
		{"no shelves", nil, []int64{}},
	} {
		hits, err := s.Search(context.Background(), "cache", Filter{Scopes: tc.scopes})
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got := sortedHitIDs(hits); !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("%s: ids = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestSearch_RetiredAndCandidates(t *testing.T) {
	s, db := openTest(t)
	ctx := context.Background()
	shelf := Ref{Kind: ScopeProject, ID: "1"}
	seedFact(t, s, shelf, "deploy on friday")
	seedFact(t, s, shelf, "deploy on monday")
	seedFact(t, s, shelf, "deploy on tuesday")
	if err := s.Retire(ctx, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE memory SET status = 'candidate' WHERE id = 3`); err != nil {
		t.Fatal(err)
	}

	hits, err := s.Search(ctx, "deploy", Filter{Scopes: []Ref{shelf}})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].ID != 1 {
		t.Fatalf("default hits = %+v, want only id 1", hits)
	}

	hits, err = s.Search(ctx, "deploy", Filter{Scopes: []Ref{shelf}, IncludeRetired: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 || hits[0].ID != 2 || hits[0].Status != StatusRetired || hits[1].ID != 1 {
		t.Fatalf("with retired hits = %+v, want ids 2 (retired), 1 (accepted)", hits)
	}
}

func TestSearch_FindsLaterWrites(t *testing.T) {
	s, db := openTest(t)
	ctx := context.Background()
	shelf := Ref{Kind: ScopeProject, ID: "1"}
	f := Filter{Scopes: []Ref{shelf}}
	seedFact(t, s, shelf, "alpha")

	if hits, _ := s.Search(ctx, "beta", f); len(hits) != 0 {
		t.Fatalf("hits before write = %+v", hits)
	}
	seedFact(t, s, shelf, "beta")
	if hits, _ := s.Search(ctx, "beta", f); len(hits) != 1 || hits[0].ID != 2 {
		t.Fatalf("hits after insert = %+v, want id 2", hits)
	}

	if _, err := db.Exec(`UPDATE memory SET body = 'gamma' WHERE id = 2`); err != nil {
		t.Fatal(err)
	}
	if hits, _ := s.Search(ctx, "beta", f); len(hits) != 0 {
		t.Fatalf("hits for old body = %+v", hits)
	}
	if hits, _ := s.Search(ctx, "gamma", f); len(hits) != 1 || hits[0].ID != 2 {
		t.Fatalf("hits for new body = %+v, want id 2", hits)
	}

	if _, err := db.Exec(`DELETE FROM memory WHERE id = 2`); err != nil {
		t.Fatal(err)
	}
	if hits, _ := s.Search(ctx, "gamma", f); len(hits) != 0 {
		t.Fatalf("hits after delete = %+v", hits)
	}
}

func TestSearch_QueryNeverBreaksSyntax(t *testing.T) {
	s, _ := openTest(t)
	shelf := Ref{Kind: ScopeProject, ID: "1"}
	f := Filter{Scopes: []Ref{shelf}}
	seedFact(t, s, shelf, "foo bar")

	for _, q := range []string{`foo"bar`, `foo AND`, `(foo`, `NEAR(foo bar)`, `foo*`, `col:foo`, `-foo`} {
		if _, err := s.Search(context.Background(), q, f); err != nil {
			t.Fatalf("Search(%q): %v", q, err)
		}
	}
	for _, q := range []string{"", "  ", `"*?`} {
		hits, err := s.Search(context.Background(), q, f)
		if err != nil || hits != nil {
			t.Fatalf("Search(%q) = %v, %v; want nil, nil", q, hits, err)
		}
	}
}
