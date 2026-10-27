package memory

import (
	"context"
	"reflect"
	"testing"
)

func TestNewest_AcceptedItemsOfTheScopeNewestFirst(t *testing.T) {
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
	fourth := add(w, Note{Kind: KindFact, Body: "four"})
	add(w, Note{Kind: KindFact, Body: "five", NeedsApproval: true})
	add(other, Note{Kind: KindFact, Body: "elsewhere"})
	if err := s.Retire(ctx, retired); err != nil {
		t.Fatal(err)
	}

	ids := func(items []Item) []int64 {
		var out []int64
		for _, it := range items {
			out = append(out, it.ID)
		}
		return out
	}
	tests := []struct {
		name      string
		kinds     []string
		limit     int
		wantIDs   []int64
		wantTotal int
	}{
		{"all kinds", nil, 0, []int64{fourth, third, first}, 3},
		{"limited", nil, 2, []int64{fourth, third}, 3},
		{"some kinds", []string{KindFact, KindFinding}, 5, []int64{fourth, first}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, total, err := s.Newest(ctx, w.Scope, tt.kinds, tt.limit)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(ids(got), tt.wantIDs) || total != tt.wantTotal {
				t.Fatalf("Newest = %v of %d, want %v of %d", ids(got), total, tt.wantIDs, tt.wantTotal)
			}
		})
	}
}
