package store

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

const _notLive9 = "invalid: decision 9 is not a live decision of work item 1"

func TestCreateDecisionReplaces(t *testing.T) {
	ctx := context.Background()
	s := proposalFixture(t)

	d, err := s.CreateDecision(ctx, Decision{
		WorkItemID: 1, Body: "use rust", Author: "user", Replaces: []int64{1},
	})
	if err != nil {
		t.Fatal(err)
	}
	old, err := s.Decision(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if old.RetiredAt == nil || old.ReplacedBy == nil || *old.ReplacedBy != d.ID {
		t.Errorf("decision 1 = retired %v replacedBy %v, want retired and replaced by %d",
			old.RetiredAt, old.ReplacedBy, d.ID)
	}
	if old.Body != "use go" {
		t.Errorf("decision 1 body = %q, want it unchanged", old.Body)
	}

	_, err = s.CreateDecision(ctx, Decision{
		WorkItemID: 1, Body: "use zig", Author: "user", Replaces: []int64{1},
	})
	want := "invalid: decision 1 is not a live decision of work item 1"
	if !errors.Is(err, ErrInvalid) || err.Error() != want {
		t.Errorf("replace a retired decision error = %v, want %s", err, want)
	}
}

func TestCreateDecisionBadReplacesWritesNothing(t *testing.T) {
	ctx := context.Background()
	s := proposalFixture(t)

	_, err := s.CreateDecision(ctx, Decision{
		WorkItemID: 1, Body: "use rust", Author: "user", Replaces: []int64{1, 9},
	})
	if !errors.Is(err, ErrInvalid) || err.Error() != _notLive9 {
		t.Errorf("error = %v, want %s", err, _notLive9)
	}
	ds, err := s.Decisions(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(ds) != 1 || ds[0].RetiredAt != nil {
		t.Errorf("decisions = %+v, want only a live decision 1", ds)
	}
}

func TestCreateDecisionCannotReplaceItself(t *testing.T) {
	ctx := context.Background()
	s := proposalFixture(t)

	_, err := s.CreateDecision(ctx, Decision{
		WorkItemID: 1, Body: "use rust", Author: "user", Replaces: []int64{2},
	})
	if want := "invalid: decision 2 is not a live decision of work item 1"; err == nil || err.Error() != want {
		t.Errorf("error = %v, want %s", err, want)
	}
	if ds, err := s.Decisions(ctx, 1); err != nil || len(ds) != 1 {
		t.Errorf("decisions = %+v, %v, want only decision 1", ds, err)
	}
}

func TestCreateProposalDedupesRetiredIDs(t *testing.T) {
	ctx := context.Background()
	s := proposalFixture(t)

	p, err := s.CreateProposal(ctx, Proposal{
		SpaceID: 1, Role: "architect", Brief: "v3", Retire: []int64{1, 2, 3, 3},
		Items: []ProposedItem{
			{Kind: "decision", Body: "use postgres", Replaces: []int64{1, 2}},
			{Kind: "decision", Body: "use yaml", Replaces: []int64{2, 3}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	want := []ProposedItem{
		{Kind: "decision", Body: "use postgres", Replaces: []int64{1, 2}},
		{Kind: "decision", Body: "use yaml", Replaces: []int64{3}},
	}
	if !reflect.DeepEqual(p.Items, want) || !reflect.DeepEqual(p.Retire, []int64{}) {
		t.Errorf("proposal = items %+v retire %v, want %+v and []", p.Items, p.Retire, want)
	}
}
