package memory

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func TestChanged_AcceptedAddedAndRetiredAfterSince(t *testing.T) {
	s, db := openTest(t)
	ctx := context.Background()
	w := writer()
	other := w
	other.Scope = Ref{Kind: ScopeWorkItem, ID: "8"}
	add := func(w Writer, n Note, created string) int64 {
		t.Helper()
		r, err := s.Remember(ctx, w, n)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`UPDATE memory SET created_at = ? WHERE id = ?`, created, r.Item.ID); err != nil {
			t.Fatal(err)
		}
		return r.Item.ID
	}
	since := time.Date(2026, 10, 27, 10, 0, 0, 500_000_000, time.UTC)
	old := add(w, Note{Kind: KindDecision, Body: "old"}, "2026-10-27T09:00:00Z")
	retiredBefore := add(w, Note{Kind: KindFact, Body: "gone before"}, "2026-10-27T09:00:00Z")
	added := add(w, Note{Kind: KindFinding, Body: "new"}, "2026-10-27T10:00:00.6Z")
	add(w, Note{Kind: KindFinding, Body: "at since"}, "2026-10-27T10:00:00.5Z")
	addedRetired := add(w, Note{Kind: KindFact, Body: "new, then gone"}, "2026-10-27T10:00:00.7Z")
	add(w, Note{Kind: KindFact, Body: "maybe", NeedsApproval: true}, "2026-10-27T10:00:01Z")
	add(other, Note{Kind: KindFact, Body: "elsewhere"}, "2026-10-27T10:00:01Z")
	replacement := add(w, Note{
		Kind: KindDecision, Body: "replaces old", Replaces: []int64{old},
	}, "2026-10-27T10:00:02Z")
	if err := s.Retire(ctx, retiredBefore); err != nil {
		t.Fatal(err)
	}
	if err := s.Retire(ctx, addedRetired); err != nil {
		t.Fatal(err)
	}
	setRetired := func(id int64, at string) {
		t.Helper()
		if _, err := db.Exec(`UPDATE memory SET retired_at = ? WHERE id = ?`, at, id); err != nil {
			t.Fatal(err)
		}
	}
	setRetired(retiredBefore, "2026-10-27T09:30:00Z")
	setRetired(addedRetired, "2026-10-27T10:00:03Z")
	setRetired(old, "2026-10-27T10:00:02Z")

	gotAdded, gotRetired, err := s.Changed(ctx, []Ref{w.Scope}, since)
	if err != nil {
		t.Fatal(err)
	}
	ids := func(items []Item) []int64 {
		var out []int64
		for _, it := range items {
			out = append(out, it.ID)
		}
		return out
	}
	if want := []int64{added, replacement}; !reflect.DeepEqual(ids(gotAdded), want) {
		t.Errorf("added = %v, want %v", ids(gotAdded), want)
	}
	if want := []int64{old, addedRetired}; !reflect.DeepEqual(ids(gotRetired), want) {
		t.Errorf("retired = %v, want %v", ids(gotRetired), want)
	}
	if r := gotRetired[0]; r.ReplacedBy == nil || *r.ReplacedBy != replacement {
		t.Errorf("retired %d ReplacedBy = %v, want %d", r.ID, r.ReplacedBy, replacement)
	}

	a, r, err := s.Changed(ctx, nil, since)
	if err != nil || a != nil || r != nil {
		t.Errorf("Changed with no scopes = %v, %v, %v, want nothing", a, r, err)
	}
}
