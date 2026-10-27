package memory

import (
	"context"
	"reflect"
	"testing"
)

func TestNewest_AcceptedItemsOfTheScopeLatestAcceptedFirst(t *testing.T) {
	s, _ := openTest(t)
	ctx := context.Background()
	add := func(w Writer, n Note) int64 {
		t.Helper()
		r, err := s.Remember(ctx, w, n)
		if err != nil {
			t.Fatal(err)
		}
		return r.Item.ID
	}
	w := writer()
	other := w
	other.Scope = Ref{Kind: ScopeWorkItem, ID: "8"}
	first := add(w, Note{Kind: KindFinding, Body: "one"})
	retired := add(w, Note{Kind: KindFact, Body: "two"})
	third := add(w, Note{Kind: KindDecision, Body: "three"})
	late := add(w, Note{Kind: KindFact, Body: "four", NeedsApproval: true})
	fifth := add(w, Note{Kind: KindFact, Body: "five"})
	add(w, Note{Kind: KindFact, Body: "six", NeedsApproval: true})
	add(other, Note{Kind: KindFact, Body: "elsewhere"})
	if err := s.Retire(ctx, retired); err != nil {
		t.Fatal(err)
	}
	if err := s.Accept(ctx, late); err != nil {
		t.Fatal(err)
	}

	ids := func(items []Item) []int64 {
		var out []int64
		for _, it := range items {
			out = append(out, it.ID)
		}
		return out
	}
	type query struct {
		kinds []string
		limit int
	}
	type result struct {
		ids   []int64
		total int
	}
	tests := []struct {
		name string
		give query
		want result
	}{
		{"all kinds", query{limit: 0}, result{ids: []int64{late, fifth, third, first}, total: 4}},
		{"limited", query{limit: 2}, result{ids: []int64{late, fifth}, total: 4}},
		{
			"some kinds",
			query{kinds: []string{KindFact, KindFinding}, limit: 5},
			result{ids: []int64{late, fifth, first}, total: 3},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, total, err := s.Newest(ctx, w.Scope, tt.give.kinds, tt.give.limit)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(ids(got), tt.want.ids) || total != tt.want.total {
				t.Fatalf("Newest = %v of %d, want %v of %d", ids(got), total, tt.want.ids, tt.want.total)
			}
		})
	}
}

func TestNewest_OrdersFractionsOfOneSecondByValue(t *testing.T) {
	s, db := openTest(t)
	ctx := context.Background()
	w := writer()
	for _, at := range []string{
		"2026-10-27T10:00:00.123Z", "2026-10-27T10:00:00.12Z", "2026-10-27T10:00:00Z",
	} {
		r, err := s.Remember(ctx, w, Note{Kind: KindFact, Body: at})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`UPDATE memory SET accepted_at = ? WHERE id = ?`, at, r.Item.ID); err != nil {
			t.Fatal(err)
		}
	}

	got, _, err := s.Newest(ctx, w.Scope, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Body != "2026-10-27T10:00:00.123Z" {
		t.Fatalf("Newest(limit 1) = %v, want the item accepted at 10:00:00.123Z", got)
	}
}
