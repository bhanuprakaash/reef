package memory

import (
	"context"
	"testing"
)

func TestRemember_SharedReplacementWaitsForApproval(t *testing.T) {
	s, _ := openTest(t)
	ctx := context.Background()
	user := Writer{Author: "user", Scope: Ref{Kind: ScopeProject, ID: "1"}}
	bot := Writer{Author: "Prakash", Role: "Prakash", Scope: user.Scope}
	own := Writer{Author: "Prakash", Role: "Prakash", Scope: Ref{Kind: ScopeBot, ID: "9"}}

	old, err := s.Remember(ctx, user, Note{Kind: KindFact, Body: "uses postgres"})
	if err != nil {
		t.Fatal(err)
	}
	repl, err := s.Remember(ctx, bot, Note{Kind: KindFact, Body: "uses sqlite", Replaces: []int64{old.Item.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Read(ctx, old.Item.ID); repl.Item.Status != StatusCandidate || got.Status != StatusAccepted {
		t.Fatalf("replacement %q, old %q; want candidate, accepted", repl.Item.Status, got.Status)
	}

	for range 2 {
		if err := s.Accept(ctx, repl.Item.ID); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := s.Read(ctx, old.Item.ID)
	if got.Status != StatusRetired || got.ReplacedBy == nil || *got.ReplacedBy != repl.Item.ID {
		t.Fatalf("old after approval = %+v", got)
	}
	if n := count(t, s.db, `SELECT COUNT(*) FROM memory WHERE status = 'accepted'`); n != 1 {
		t.Fatalf("accepted items = %d, want 1", n)
	}

	next, err := s.Remember(ctx, bot, Note{Kind: KindFact, Body: "uses duckdb", Replaces: []int64{repl.Item.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Reject(ctx, next.Item.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Read(ctx, repl.Item.ID); got.Status != StatusAccepted {
		t.Fatalf("replaced item after reject = %q, want accepted", got.Status)
	}

	mine, err := s.Remember(ctx, own, Note{Kind: KindConvention, Body: "be brief"})
	if err != nil {
		t.Fatal(err)
	}
	mine2, err := s.Remember(ctx, own, Note{Kind: KindConvention, Body: "be terse", Replaces: []int64{mine.Item.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Read(ctx, mine.Item.ID); mine2.Item.Status != StatusAccepted || got.Status != StatusRetired {
		t.Fatalf("own scope: new %q, old %q; want accepted, retired", mine2.Item.Status, got.Status)
	}
}
