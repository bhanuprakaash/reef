package store

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

const _notLive9 = "invalid: decision 9 is not a live decision of feature 1"

func TestCreateDecisionReplaces(t *testing.T) {
	ctx := context.Background()
	s := proposalFixture(t)

	d, err := s.CreateDecision(ctx, Decision{
		FeatureID: 1, Body: "use rust", Author: "user", Replaces: []int64{1},
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
		FeatureID: 1, Body: "use zig", Author: "user", Replaces: []int64{1},
	})
	want := "invalid: decision 1 is not a live decision of feature 1"
	if !errors.Is(err, ErrInvalid) || err.Error() != want {
		t.Errorf("replace a retired decision error = %v, want %s", err, want)
	}
}

func TestCreateDecisionBadReplacesWritesNothing(t *testing.T) {
	ctx := context.Background()
	s := proposalFixture(t)

	_, err := s.CreateDecision(ctx, Decision{
		FeatureID: 1, Body: "use rust", Author: "user", Replaces: []int64{1, 9},
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

func TestApproveProposalRetiresAndReplaces(t *testing.T) {
	ctx := context.Background()
	s := proposalFixture(t)
	for _, body := range []string{"use sqlite", "use json"} {
		if _, err := s.CreateDecision(ctx, Decision{FeatureID: 1, Body: body, Author: "user"}); err != nil {
			t.Fatal(err)
		}
	}
	p, err := s.CreateProposal(ctx, Proposal{
		ChannelID: 1, Role: "architect", Brief: "v3", Retire: []int64{1},
		Decisions: []ProposedDecision{{Body: "use postgres", Replaces: []int64{2}}},
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err := s.ApproveProposal(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	ds, err := s.Decisions(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	type state struct {
		Retired    bool
		ReplacedBy int64
	}
	got := map[int64]state{}
	for _, d := range ds {
		st := state{Retired: d.RetiredAt != nil}
		if d.ReplacedBy != nil {
			st.ReplacedBy = *d.ReplacedBy
		}
		got[d.ID] = st
	}
	want := map[int64]state{1: {true, 0}, 2: {true, 4}, 3: {false, 0}, 4: {false, 0}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("decision states = %+v, want %+v", got, want)
	}
}

func TestApproveProposalBadRetireFailsAll(t *testing.T) {
	ctx := context.Background()
	s := proposalFixture(t)
	p, err := s.CreateProposal(ctx, Proposal{
		ChannelID: 1, Role: "architect", Brief: "v3", Retire: []int64{1, 9},
		Decisions: []ProposedDecision{{Body: "use rust"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	before := countRows(t, s)

	_, _, err = s.ApproveProposal(ctx, p.ID)
	if !errors.Is(err, ErrInvalid) || err.Error() != _notLive9 {
		t.Errorf("error = %v, want %s", err, _notLive9)
	}
	if after := countRows(t, s); after != before {
		t.Errorf("counts after failed approve = %+v, want %+v", after, before)
	}
	d, err := s.Decision(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if d.RetiredAt != nil {
		t.Errorf("decision 1 retired at %v, want live", d.RetiredAt)
	}
	got, err := s.Proposal(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != ProposalPending {
		t.Errorf("proposal status = %q, want pending", got.Status)
	}
}
