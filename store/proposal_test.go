package store

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

// proposalFixture has work item 1 with Space 1, a brief "v1", architect notes
// "old" and one decision; Space 2 is a quick session. Proposal 1 is pending.
func proposalFixture(t *testing.T, items ...ProposedItem) *Store {
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
			_, err := s.CreateWorkItem(ctx, WorkItem{ProjectID: 1, Title: "Auth"})
			return err
		},
		func() error {
			_, err := s.CreateSpace(ctx, Space{ProjectID: 1, WorkItemID: &one, Name: "architect"})
			return err
		},
		func() error { _, err := s.CreateSpace(ctx, Space{ProjectID: 1, Name: "quick"}); return err },
		func() error { _, err := s.PutBrief(ctx, Brief{WorkItemID: 1, Body: "v1", Author: "user"}); return err },
		func() error {
			_, err := s.PutRoleNotes(ctx, RoleNotes{WorkItemID: 1, Role: "architect", Body: "old"})
			return err
		},
		func() error {
			_, err := s.CreateDecision(ctx, Decision{WorkItemID: 1, Body: "use go", Author: "user"})
			return err
		},
		func() error {
			_, err := s.CreateProposal(ctx, Proposal{
				SpaceID: 1, Role: "architect", Brief: "v2", Items: items,
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

func TestApproveBriefProposal(t *testing.T) {
	ctx := context.Background()
	s := proposalFixture(t)
	if _, err := s.RejectProposal(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateProposal(ctx, Proposal{
		SpaceID: 1, Role: "lead", Kind: ProposalBrief, Brief: "v3",
	}); err != nil {
		t.Fatal(err)
	}
	before := countRows(t, s)

	got, err := s.ApproveProposal(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != ProposalBrief || got.Status != ProposalApproved {
		t.Errorf("ApproveProposal = kind %q status %q, want brief approved", got.Kind, got.Status)
	}
	if after := countRows(t, s); after != (counts{before.Briefs + 1, before.Decisions}) {
		t.Errorf("counts after approve = %+v, want one more brief than %+v", after, before)
	}
	b, err := s.Brief(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if b.Body != "v3" || b.Author != "lead" {
		t.Errorf("Brief = %q by %q, want %q by lead", b.Body, b.Author, "v3")
	}
	c, err := s.Space(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if c.ClosedAt != nil {
		t.Errorf("space closedAt = %v, want nil", c.ClosedAt)
	}
}

func TestCreateBriefProposalRejectsOlderBriefProposals(t *testing.T) {
	ctx := context.Background()
	s := proposalFixture(t)
	if _, err := s.RejectProposal(ctx, 1); err != nil {
		t.Fatal(err)
	}
	for _, brief := range []string{"v3", "v4"} {
		if _, err := s.CreateProposal(ctx, Proposal{
			SpaceID: 1, Role: "lead", Kind: ProposalBrief, Brief: brief,
		}); err != nil {
			t.Fatal(err)
		}
	}

	ps, err := s.PendingProposals(ctx, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, p := range ps {
		got = append(got, p.Kind+":"+p.Brief)
	}
	if want := []string{"brief:v4"}; !reflect.DeepEqual(got, want) {
		t.Errorf("pending = %v, want %v", got, want)
	}
}

func TestDecideProposalTwice(t *testing.T) {
	ctx := context.Background()
	s := proposalFixture(t)
	if _, err := s.ApproveProposal(ctx, 1); err != nil {
		t.Fatal(err)
	}
	before := countRows(t, s)

	if _, err := s.ApproveProposal(ctx, 1); !errors.Is(err, ErrInvalid) {
		t.Errorf("second approve = %v, want ErrInvalid", err)
	}
	if _, err := s.RejectProposal(ctx, 1); !errors.Is(err, ErrInvalid) {
		t.Errorf("reject after approve = %v, want ErrInvalid", err)
	}
	if after := countRows(t, s); after != before {
		t.Errorf("counts after second approve = %+v, want %+v", after, before)
	}
}

func TestPendingProposals(t *testing.T) {
	ctx := context.Background()
	s := proposalFixture(t)
	if _, err := s.CreateWorkItem(ctx, WorkItem{ProjectID: 1, Title: "Billing"}); err != nil {
		t.Fatal(err)
	}
	one, two := int64(1), int64(2)
	if _, err := s.db.ExecContext(ctx,
		"UPDATE space SET closed_at = '2026-01-01T00:00:00Z' WHERE id = 1"); err != nil {
		t.Fatal(err)
	}
	for _, c := range []Space{
		{ProjectID: 1, WorkItemID: &two, Name: "dev"},
		{ProjectID: 1, WorkItemID: &one, Name: "review"},
	} {
		if _, err := s.CreateSpace(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []Proposal{
		{SpaceID: 3, Role: "developer", Brief: "b2"},
		{SpaceID: 4, Role: "reviewer", Brief: "b3"},
	} {
		if _, err := s.CreateProposal(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.RejectProposal(ctx, 3); err != nil {
		t.Fatal(err)
	}

	ids := func(workItemID *int64) []int64 {
		ps, err := s.PendingProposals(ctx, workItemID, nil)
		if err != nil {
			t.Fatal(err)
		}
		var got []int64
		for _, p := range ps {
			got = append(got, p.ID)
		}
		return got
	}
	if got, want := ids(nil), []int64{1, 2}; !reflect.DeepEqual(got, want) {
		t.Errorf("all pending = %v, want %v", got, want)
	}
	if got, want := ids(&one), []int64{1}; !reflect.DeepEqual(got, want) {
		t.Errorf("pending of work item 1 = %v, want %v", got, want)
	}
	if got, want := ids(&two), []int64{2}; !reflect.DeepEqual(got, want) {
		t.Errorf("pending of work item 2 = %v, want %v", got, want)
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
		{"quick session", Proposal{SpaceID: 2, Role: "architect", Brief: "b"}, ErrInvalid},
		{"empty brief", Proposal{SpaceID: 1, Role: "architect"}, ErrInvalid},
		{
			"empty item body",
			Proposal{SpaceID: 1, Role: "architect", Brief: "b", Items: []ProposedItem{{}}},
			ErrInvalid,
		},
		{"unknown space", Proposal{SpaceID: 9, Role: "architect", Brief: "b"}, ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := s.CreateProposal(ctx, tt.give); !errors.Is(err, tt.want) {
				t.Errorf("CreateProposal = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestApproveProposalClosesSpaceAndReopenClearsIt(t *testing.T) {
	ctx := context.Background()
	s := proposalFixture(t)
	if _, err := s.ApproveProposal(ctx, 1); err != nil {
		t.Fatal(err)
	}

	c, err := s.Space(ctx, 1)
	if err != nil || c.ClosedAt == nil {
		t.Fatalf("space 1 closedAt after approve = %v, err = %v, want set", c.ClosedAt, err)
	}
	if q, err := s.Space(ctx, 2); err != nil || q.ClosedAt != nil {
		t.Errorf("space 2 closedAt = %v, err = %v, want nil", q.ClosedAt, err)
	}

	if err := s.ReopenSpace(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if c, err = s.Space(ctx, 1); err != nil || c.ClosedAt != nil {
		t.Errorf("space 1 closedAt after reopen = %v, err = %v, want nil", c.ClosedAt, err)
	}
	if err := s.ReopenSpace(ctx, 99); !errors.Is(err, ErrNotFound) {
		t.Errorf("reopen Space 99 = %v, want ErrNotFound", err)
	}
}

func TestCreateBriefProposalRefusedWhileWrapPending(t *testing.T) {
	ctx := context.Background()
	s := proposalFixture(t)

	_, err := s.CreateProposal(ctx, Proposal{
		SpaceID: 1, Role: "lead", Kind: ProposalBrief, Brief: "v3",
	})
	want := "invalid: Space 1 has a wrap-up waiting for approval"
	if !errors.Is(err, ErrInvalid) || err.Error() != want {
		t.Errorf("CreateProposal brief = %v, want %s", err, want)
	}
	p, err := s.PendingProposal(ctx, 1)
	if err != nil || p.ID != 1 || p.Kind != ProposalWrap {
		t.Errorf("pending = %d %q, err %v, want wrap 1", p.ID, p.Kind, err)
	}
}

func TestCreateProposalUnknownKindMessage(t *testing.T) {
	s := proposalFixture(t)

	_, err := s.CreateProposal(context.Background(), Proposal{
		SpaceID: 1, Role: "lead", Kind: "x", Brief: "b",
	})
	if want := `invalid: unknown proposal kind "x"`; err == nil || err.Error() != want {
		t.Errorf("error = %v, want %s", err, want)
	}
}

func TestStaleBriefProposalCannotOverwriteWrap(t *testing.T) {
	ctx := context.Background()
	s := proposalFixture(t)
	if _, err := s.RejectProposal(ctx, 1); err != nil {
		t.Fatal(err)
	}
	for _, p := range []Proposal{
		{SpaceID: 1, Role: "lead", Kind: ProposalBrief, Brief: "stale"},
		{SpaceID: 1, Role: "lead", Brief: "final"},
	} {
		if _, err := s.CreateProposal(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.ApproveProposal(ctx, 3); err != nil {
		t.Fatal(err)
	}

	if _, err := s.ApproveProposal(ctx, 2); !errors.Is(err, ErrInvalid) {
		t.Errorf("approve stale brief = %v, want ErrInvalid", err)
	}
	b, err := s.Brief(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if b.Body != "final" {
		t.Errorf("Brief = %q, want %q", b.Body, "final")
	}
	got, err := s.Proposal(ctx, 2)
	if err != nil || got.Status != ProposalRejected {
		t.Errorf("proposal 2 = %q, err %v, want rejected", got.Status, err)
	}
}

func TestCreateWrapProposalRejectsOlderPending(t *testing.T) {
	ctx := context.Background()
	s := proposalFixture(t)
	if _, err := s.CreateProposal(ctx, Proposal{SpaceID: 1, Role: "lead", Brief: "v3"}); err != nil {
		t.Fatal(err)
	}

	ps, err := s.PendingProposals(ctx, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 1 || ps[0].ID != 2 || ps[0].Brief != "v3" {
		t.Errorf("pending = %+v, want only proposal 2", ps)
	}
	if old, err := s.Proposal(ctx, 1); err != nil || old.Status != ProposalRejected {
		t.Errorf("proposal 1 = %q, err %v, want rejected", old.Status, err)
	}
}

func TestApproveBriefProposalOnClosedSpace(t *testing.T) {
	ctx := context.Background()
	s := proposalFixture(t)
	if _, err := s.RejectProposal(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateProposal(ctx, Proposal{
		SpaceID: 1, Role: "lead", Kind: ProposalBrief, Brief: "v3",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MoveWorkItem(ctx, 1, "dropped", ByUser); err != nil {
		t.Fatal(err)
	}

	_, err := s.ApproveProposal(ctx, 2)
	if want := "invalid: Space 1 is closed"; err == nil || err.Error() != want {
		t.Errorf("approve = %v, want %s", err, want)
	}
	if b, err := s.Brief(ctx, 1); err != nil || b.Body != "v1" {
		t.Errorf("Brief = %q, err %v, want v1", b.Body, err)
	}
}

func TestApproveProposal(t *testing.T) {
	ctx := context.Background()
	want := []ProposedItem{
		{Kind: "decision", Body: "cookie sessions", Why: "simple", Scope: "work_item"},
		{Kind: "lesson", Body: "24h expiry", Scope: "team", Replaces: []int64{3}},
	}
	s := proposalFixture(t, want...)

	pending, err := s.PendingProposal(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(pending.Items, want) {
		t.Errorf("pending items = %+v, want %+v", pending.Items, want)
	}
	got, err := s.ApproveProposal(ctx, 1)
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
	if _, err := s.PendingProposal(ctx, 1); !errors.Is(err, ErrNotFound) {
		t.Errorf("PendingProposal after approve = %v, want ErrNotFound", err)
	}
}

func TestRejectProposal(t *testing.T) {
	ctx := context.Background()
	s := proposalFixture(t, ProposedItem{Kind: "decision", Body: "cookie sessions"})
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
}

func TestApproveProposalFailureAppliesNothing(t *testing.T) {
	ctx := context.Background()
	s := proposalFixture(t)
	if _, err := s.db.Exec(`CREATE TRIGGER space_boom BEFORE UPDATE OF closed_at ON space
		BEGIN SELECT RAISE(ABORT, 'boom'); END`); err != nil {
		t.Fatal(err)
	}
	before := countRows(t, s)

	if _, err := s.ApproveProposal(ctx, 1); err == nil {
		t.Fatal("ApproveProposal succeeded, want the Space close to fail")
	}
	if after := countRows(t, s); after != before {
		t.Errorf("counts after failed approve = %+v, want %+v", after, before)
	}
	p, err := s.PendingProposal(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if p.Status != ProposalPending {
		t.Errorf("status = %q, want pending", p.Status)
	}
}
