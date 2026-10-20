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
		ChannelID: 1, Role: "architect", Brief: "v3", Retire: []int64{1},
		Decisions: []ProposedDecision{{Body: "use rust"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE proposal SET retire = '[1, 9]' WHERE id = ?`, p.ID); err != nil {
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

func TestCreateDecisionCannotReplaceItself(t *testing.T) {
	ctx := context.Background()
	s := proposalFixture(t)

	_, err := s.CreateDecision(ctx, Decision{
		FeatureID: 1, Body: "use rust", Author: "user", Replaces: []int64{2},
	})
	if want := "invalid: decision 2 is not a live decision of feature 1"; err == nil || err.Error() != want {
		t.Errorf("error = %v, want %s", err, want)
	}
	if ds, err := s.Decisions(ctx, 1); err != nil || len(ds) != 1 {
		t.Errorf("decisions = %+v, %v, want only decision 1", ds, err)
	}
}

func TestCreateProposalChecksRetiredIDs(t *testing.T) {
	ctx := context.Background()
	s := proposalFixture(t)
	if _, err := s.CreateFeature(ctx, Feature{ProjectID: 1, Slug: "billing", Name: "Billing"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateDecision(ctx, Decision{FeatureID: 2, Body: "use stripe", Author: "user"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateDecision(ctx, Decision{
		FeatureID: 1, Body: "use rust", Author: "user", Replaces: []int64{1},
	}); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		give Proposal
		want string
	}{
		{"retire unknown", Proposal{Retire: []int64{9}}, _notLive9},
		{"replace unknown", Proposal{Decisions: []ProposedDecision{{Body: "x", Replaces: []int64{9}}}}, _notLive9},
		{"retire other feature", Proposal{Retire: []int64{2}},
			"invalid: decision 2 is not a live decision of feature 1"},
		{"retire already retired", Proposal{Retire: []int64{1}},
			"invalid: decision 1 is not a live decision of feature 1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.give.ChannelID, tt.give.Role, tt.give.Brief = 1, "architect", "v3"
			_, err := s.CreateProposal(ctx, tt.give)
			if !errors.Is(err, ErrInvalid) || err.Error() != tt.want {
				t.Errorf("error = %v, want %s", err, tt.want)
			}
		})
	}
	if p, err := s.PendingProposal(ctx, 1); err != nil || p.ID != 1 {
		t.Errorf("pending = %d, %v, want the earlier proposal 1 untouched", p.ID, err)
	}
}

func TestCreateProposalDedupesRetiredIDs(t *testing.T) {
	ctx := context.Background()
	s := proposalFixture(t)
	for _, body := range []string{"use sqlite", "use json"} {
		if _, err := s.CreateDecision(ctx, Decision{FeatureID: 1, Body: body, Author: "user"}); err != nil {
			t.Fatal(err)
		}
	}

	p, err := s.CreateProposal(ctx, Proposal{
		ChannelID: 1, Role: "architect", Brief: "v3", Retire: []int64{1, 2, 3, 3},
		Decisions: []ProposedDecision{
			{Body: "use postgres", Replaces: []int64{1, 2}},
			{Body: "use yaml", Replaces: []int64{2, 3}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	want := []ProposedDecision{
		{Body: "use postgres", Replaces: []int64{1, 2}},
		{Body: "use yaml", Replaces: []int64{3}},
	}
	if !reflect.DeepEqual(p.Decisions, want) || !reflect.DeepEqual(p.Retire, []int64{}) {
		t.Errorf("proposal = decisions %+v retire %v, want %+v and []", p.Decisions, p.Retire, want)
	}
}

func TestApproveProposalSkipsAlreadyRetired(t *testing.T) {
	ctx := context.Background()
	s := proposalFixture(t)
	p, err := s.CreateProposal(ctx, Proposal{
		ChannelID: 1, Role: "architect", Brief: "v3", Retire: []int64{1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateDecision(ctx, Decision{
		FeatureID: 1, Body: "use rust", Author: "user", Replaces: []int64{1},
	}); err != nil {
		t.Fatal(err)
	}

	if _, _, err := s.ApproveProposal(ctx, p.ID); err != nil {
		t.Fatalf("approve with a decision retired meanwhile = %v, want nil", err)
	}
	d, err := s.Decision(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if d.ReplacedBy == nil || *d.ReplacedBy != 2 {
		t.Errorf("decision 1 replacedBy = %v, want 2 kept from the first retire", d.ReplacedBy)
	}
}
