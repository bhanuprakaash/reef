package store

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

// proposalFixture has feature 1 with channel 1, a brief "v1", architect notes
// "old" and one decision; channel 2 is a quick session. Proposal 1 is pending.
func proposalFixture(t *testing.T, decisions ...ProposedDecision) *Store {
	t.Helper()
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	one := int64(1)
	steps := []func() error{
		func() error { _, err := s.CreateProject(ctx, Project{Name: "reef", Path: "/tmp/reef"}); return err },
		func() error {
			_, err := s.CreateFeature(ctx, Feature{ProjectID: 1, Slug: "auth", Name: "Auth"})
			return err
		},
		func() error {
			_, err := s.CreateChannel(ctx, Channel{ProjectID: 1, FeatureID: &one, Name: "architect"})
			return err
		},
		func() error { _, err := s.CreateChannel(ctx, Channel{ProjectID: 1, Name: "quick"}); return err },
		func() error { _, err := s.PutBrief(ctx, Brief{FeatureID: 1, Body: "v1", Author: "user"}); return err },
		func() error {
			_, err := s.PutRoleNotes(ctx, RoleNotes{FeatureID: 1, Role: "architect", Body: "old"})
			return err
		},
		func() error {
			_, err := s.CreateDecision(ctx, Decision{FeatureID: 1, Body: "use go", Author: "user"})
			return err
		},
		func() error {
			_, err := s.CreateProposal(ctx, Proposal{
				ChannelID: 1, Role: "architect", Brief: "v2", Notes: "new", Decisions: decisions,
			})
			return err
		},
	}
	for _, step := range steps {
		if err := step(); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

type counts struct{ Briefs, Decisions int }

func countRows(t *testing.T, s *Store) counts {
	t.Helper()
	ctx := context.Background()
	bs, err := s.BriefVersions(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	ds, err := s.Decisions(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	return counts{len(bs), len(ds)}
}

func TestApproveProposal(t *testing.T) {
	ctx := context.Background()
	want := []ProposedDecision{{Body: "cookie sessions", Why: "simple"}, {Body: "24h expiry"}}
	s := proposalFixture(t, want...)

	got, added, err := s.ApproveProposal(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != ProposalApproved || got.DecidedAt == nil {
		t.Errorf("ApproveProposal = status %q decided %v, want approved and a time",
			got.Status, got.DecidedAt)
	}
	b, err := s.Brief(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if b.Body != "v2" || b.Author != "architect" {
		t.Errorf("Brief = %q by %q, want %q by architect", b.Body, b.Author, "v2")
	}
	n, err := s.RoleNotes(ctx, 1, "architect")
	if err != nil {
		t.Fatal(err)
	}
	if n.Body != "new" {
		t.Errorf("notes = %q, want %q", n.Body, "new")
	}
	ds, err := s.Decisions(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	var gotDecisions []ProposedDecision
	for _, d := range ds[1:] {
		if d.Author != "architect" {
			t.Errorf("decision %q author = %q, want architect", d.Body, d.Author)
		}
		gotDecisions = append(gotDecisions, ProposedDecision{Body: d.Body, Why: d.Why})
	}
	if !reflect.DeepEqual(gotDecisions, want) || added != 2 {
		t.Errorf("decisions added = %v (%d), want %v (2)", gotDecisions, added, want)
	}
	if _, err := s.PendingProposal(ctx, 1); !errors.Is(err, ErrNotFound) {
		t.Errorf("PendingProposal after approve = %v, want ErrNotFound", err)
	}
}

func TestApproveProposalSkipsRecordedDecisions(t *testing.T) {
	ctx := context.Background()
	s := proposalFixture(t,
		ProposedDecision{Body: "use go", Why: "again"},
		ProposedDecision{Body: "cookie sessions"},
		ProposedDecision{Body: "cookie sessions"},
		ProposedDecision{Body: "Use Go"})

	_, added, err := s.ApproveProposal(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	ds, err := s.Decisions(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	var bodies []string
	for _, d := range ds {
		bodies = append(bodies, d.Body)
	}
	if want := []string{"use go", "cookie sessions", "Use Go"}; !reflect.DeepEqual(bodies, want) || added != 2 {
		t.Errorf("decisions = %q, added = %d, want %q, 2", bodies, added, want)
	}
}

func TestRejectProposal(t *testing.T) {
	ctx := context.Background()
	s := proposalFixture(t, ProposedDecision{Body: "cookie sessions"})
	before := countRows(t, s)

	got, err := s.RejectProposal(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != ProposalRejected {
		t.Errorf("RejectProposal status = %q, want rejected", got.Status)
	}
	if after := countRows(t, s); after != before {
		t.Errorf("counts after reject = %+v, want %+v", after, before)
	}
	n, err := s.RoleNotes(ctx, 1, "architect")
	if err != nil {
		t.Fatal(err)
	}
	if n.Body != "old" {
		t.Errorf("notes = %q, want %q", n.Body, "old")
	}
}

func TestApproveProposalFailureAppliesNothing(t *testing.T) {
	ctx := context.Background()
	s := proposalFixture(t,
		ProposedDecision{Body: "cookie sessions"}, ProposedDecision{Body: "boom"})
	if _, err := s.db.Exec(`CREATE TRIGGER decision_boom BEFORE INSERT ON decision
		WHEN NEW.body = 'boom' BEGIN SELECT RAISE(ABORT, 'boom'); END`); err != nil {
		t.Fatal(err)
	}
	before := countRows(t, s)

	if _, _, err := s.ApproveProposal(ctx, 1); err == nil {
		t.Fatal("ApproveProposal succeeded, want the decision insert to fail")
	}
	if after := countRows(t, s); after != before {
		t.Errorf("counts after failed approve = %+v, want %+v", after, before)
	}
	n, err := s.RoleNotes(ctx, 1, "architect")
	if err != nil {
		t.Fatal(err)
	}
	if n.Body != "old" {
		t.Errorf("notes = %q, want %q", n.Body, "old")
	}
	p, err := s.PendingProposal(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if p.Status != ProposalPending {
		t.Errorf("status = %q, want pending", p.Status)
	}
}

func TestDecideProposalTwice(t *testing.T) {
	ctx := context.Background()
	s := proposalFixture(t)
	if _, _, err := s.ApproveProposal(ctx, 1); err != nil {
		t.Fatal(err)
	}
	before := countRows(t, s)

	if _, _, err := s.ApproveProposal(ctx, 1); !errors.Is(err, ErrInvalid) {
		t.Errorf("second approve = %v, want ErrInvalid", err)
	}
	if _, err := s.RejectProposal(ctx, 1); !errors.Is(err, ErrInvalid) {
		t.Errorf("reject after approve = %v, want ErrInvalid", err)
	}
	if after := countRows(t, s); after != before {
		t.Errorf("counts after second approve = %+v, want %+v", after, before)
	}
}

func TestCreateProposalInvalid(t *testing.T) {
	ctx := context.Background()
	s := proposalFixture(t)
	tests := []struct {
		name string
		give Proposal
		want error
	}{
		{"quick session", Proposal{ChannelID: 2, Role: "architect", Brief: "b"}, ErrInvalid},
		{"empty brief", Proposal{ChannelID: 1, Role: "architect"}, ErrInvalid},
		{
			"empty decision body",
			Proposal{ChannelID: 1, Role: "architect", Brief: "b", Decisions: []ProposedDecision{{}}},
			ErrInvalid,
		},
		{"unknown channel", Proposal{ChannelID: 9, Role: "architect", Brief: "b"}, ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := s.CreateProposal(ctx, tt.give); !errors.Is(err, tt.want) {
				t.Errorf("CreateProposal = %v, want %v", err, tt.want)
			}
		})
	}
}
