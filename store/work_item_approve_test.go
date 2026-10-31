package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
)

func TestApproveQueuesOnceAndKeepsItsIdentity(t *testing.T) {
	ctx := context.Background()
	s, ann := queueFixture(t, 2)
	bob, err := s.CreateBot(ctx, Bot{Name: "bob", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	cy, err := s.CreateBot(ctx, Bot{Name: "cy", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.MoveWorkItem(ctx, 1, _queued, ByUser); !errors.Is(err, ErrInvalid) {
		t.Fatalf("user backlog to queued = %v, want ErrInvalid", err)
	}
	if _, err := s.SetWorkItemParticipants(ctx, 1, &ann, []int64{bob.ID}); err != nil {
		t.Fatal(err)
	}

	w, err := s.ApproveWorkItem(ctx, 1, &bob.ID, []int64{cy.ID}, nil, "k1")
	if err != nil {
		t.Fatal(err)
	}
	if w.Status != _inProgress || *w.OwnerBotID != bob.ID || w.ApprovedAt == nil || *w.ApprovalKey != "k1" ||
		!slices.Equal(w.ParticipantBotIDs, []int64{bob.ID, cy.ID}) {
		t.Errorf("approved = %+v, want in_progress, owner bob, participants [bob cy], key k1", w)
	}
	again, err := s.ApproveWorkItem(ctx, 1, &ann, nil, nil, "k1")
	if err != nil || *again.OwnerBotID != bob.ID || !again.ApprovedAt.Equal(*w.ApprovedAt) {
		t.Errorf("same key again = %+v, %v, want the item unchanged", again, err)
	}
	if _, err := s.ApproveWorkItem(ctx, 1, &bob.ID, nil, nil, "k2"); !errors.Is(err, ErrConflict) {
		t.Errorf("another key = %v, want ErrConflict", err)
	}
	if _, err := s.ApproveWorkItem(ctx, 2, &ann, nil, nil, "k1"); !errors.Is(err, ErrConflict) {
		t.Errorf("a key of another item = %v, want ErrConflict", err)
	}
	if got, want := queueState(t, s, 1, 2), "1:in_progress:- 2:backlog:-"; got != want {
		t.Errorf("state = %q, want %q", got, want)
	}
}

func TestQueuedItemMovedBackIsNoLongerApproved(t *testing.T) {
	ctx := context.Background()
	s, ann := queueFixture(t, 1)
	waiting(t, s, 1)
	if _, err := s.db.Exec("UPDATE work_item SET approved_at = '2026-01-01T00:00:00Z', approval_key = 'k' WHERE id = 1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MoveWorkItem(ctx, 1, _backlog, ByUser); err != nil {
		t.Fatal(err)
	}
	w, err := s.ApproveWorkItem(ctx, 1, &ann, nil, nil, "k")
	if err != nil || w.Status != _inProgress {
		t.Errorf("approve again with the old key = %+v, %v, want a new approval that starts", w, err)
	}
}

func TestAddWorkItemParticipantIsIdempotentAndUsesTheParent(t *testing.T) {
	ctx := context.Background()
	s, ann := queueFixture(t, 1)
	bob, err := s.CreateBot(ctx, Bot{Name: "bob", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	parent := int64(1)
	if _, err := s.CreateWorkItem(ctx, WorkItem{ProjectID: 1, ParentID: &parent, Title: "sub"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApproveWorkItem(ctx, 1, &ann, nil, nil, "k"); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := s.AddWorkItemParticipant(ctx, 2, bob.ID); err != nil {
			t.Fatal(err)
		}
	}
	w, err := s.WorkItem(ctx, 1)
	if err != nil || !slices.Equal(w.ParticipantBotIDs, []int64{ann, bob.ID}) {
		t.Errorf("participants = %v, %v, want [ann bob]", w.ParticipantBotIDs, err)
	}
}

func TestApproveDecidesTheRepositoriesAndKeepsWhoProposedThem(t *testing.T) {
	ctx := context.Background()
	s, ann := queueFixture(t, 0)
	for _, name := range []string{"infra", "docs", "web"} {
		if _, err := s.CreateProject(ctx, Project{Name: name, Path: "/tmp/" + name}); err != nil {
			t.Fatal(err)
		}
	}
	targets := func(id int64) string {
		t.Helper()
		w, err := s.WorkItem(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, tg := range w.Targets {
			out = append(out, fmt.Sprintf("%s:proposed=%q:authorized=%q", tg.ProjectName, tg.ProposedBy, tg.AuthorizedBy))
		}
		return strings.Join(out, " ")
	}
	proposal := func(title string) int64 {
		t.Helper()
		w, err := s.CreateWorkItem(ctx, WorkItem{
			ProjectID: 1, Title: title, ProposedBy: "ann", OwnerBotID: &ann,
			Targets: []Target{{ProjectID: 2}, {ProjectID: 3}},
		})
		if err != nil {
			t.Fatal(err)
		}
		return w.ID
	}

	a := proposal("a")
	if got, want := targets(a), `reef:proposed="ann":authorized="" infra:proposed="ann":authorized="" docs:proposed="ann":authorized=""`; got != want {
		t.Errorf("proposed targets = %s, want %s", got, want)
	}
	if _, err := s.ApproveWorkItem(ctx, a, &ann, nil, []int64{3, 4}, "ka"); err != nil {
		t.Fatal(err)
	}
	if got, want := targets(a), `reef:proposed="ann":authorized="user" docs:proposed="ann":authorized="user" web:proposed="":authorized="user"`; got != want {
		t.Errorf("targets after approving [docs web] = %s, want %s", got, want)
	}

	b := proposal("b")
	if _, err := s.ApproveWorkItem(ctx, b, &ann, nil, nil, "kb"); err != nil {
		t.Fatal(err)
	}
	if got, want := targets(b), `reef:proposed="ann":authorized="user" infra:proposed="ann":authorized="user" docs:proposed="ann":authorized="user"`; got != want {
		t.Errorf("targets after approving with no list = %s, want %s", got, want)
	}

	c := proposal("c")
	unknown := int64(99)
	if _, err := s.ApproveWorkItem(ctx, c, &unknown, nil, nil, "kc"); !errors.Is(err, ErrInvalid) {
		t.Errorf("approve with an unknown owner = %v, want ErrInvalid", err)
	}
	if err := s.RemoveWorkItemTarget(ctx, c, 2); err != nil {
		t.Errorf("remove a proposed target from a backlog item = %v, want nil", err)
	}
	if err := s.RemoveWorkItemTarget(ctx, c, 1); !errors.Is(err, ErrInvalid) {
		t.Errorf("remove the home target = %v, want ErrInvalid", err)
	}
	if err := s.RemoveWorkItemTarget(ctx, b, 2); !errors.Is(err, ErrConflict) {
		t.Errorf("remove a target of a started item = %v, want ErrConflict", err)
	}
}
