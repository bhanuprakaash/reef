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
		if _, err := db.Exec(`UPDATE memory SET created_at = ?1, accepted_at = ?1 WHERE id = ?2`, created, r.Item.ID); err != nil {
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

func TestChanged_CandidateAcceptedAfterSinceIsAdded(t *testing.T) {
	s, db := openTest(t)
	ctx := context.Background()
	w := writer()
	old, err := s.Remember(ctx, w, Note{Kind: KindDecision, Body: "use cookies"})
	if err != nil {
		t.Fatal(err)
	}
	cand, err := s.Remember(ctx, w, Note{
		Kind: KindDecision, Body: "use sessions", Replaces: []int64{old.Item.ID}, NeedsApproval: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	hourAgo := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
	if _, err := db.Exec(`UPDATE memory SET created_at = ?1, accepted_at = ?1 WHERE id = ?2`, hourAgo, old.Item.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE memory SET created_at = ? WHERE id = ?`, hourAgo, cand.Item.ID); err != nil {
		t.Fatal(err)
	}
	since := time.Now().Add(-time.Minute)
	if added, retired, err := s.Changed(ctx, []Ref{w.Scope}, since); err != nil || added != nil || retired != nil {
		t.Fatalf("Changed before accepting = %v, %v, %v, want nothing", added, retired, err)
	}

	if err := s.Accept(ctx, cand.Item.ID); err != nil {
		t.Fatal(err)
	}
	added, retired, err := s.Changed(ctx, []Ref{w.Scope}, since)
	if err != nil {
		t.Fatal(err)
	}
	if len(added) != 1 || added[0].ID != cand.Item.ID {
		t.Errorf("added = %+v, want only item %d", added, cand.Item.ID)
	}
	if len(retired) != 1 || retired[0].ID != old.Item.ID ||
		retired[0].ReplacedBy == nil || *retired[0].ReplacedBy != cand.Item.ID {
		t.Errorf("retired = %+v, want item %d replaced by %d", retired, old.Item.ID, cand.Item.ID)
	}
}
