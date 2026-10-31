package store

import (
	"context"
	"errors"
	"slices"
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

	w, err := s.ApproveWorkItem(ctx, 1, &bob.ID, []int64{cy.ID}, "k1")
	if err != nil {
		t.Fatal(err)
	}
	if w.Status != _inProgress || *w.OwnerBotID != bob.ID || w.ApprovedAt == nil || *w.ApprovalKey != "k1" ||
		!slices.Equal(w.ParticipantBotIDs, []int64{bob.ID, cy.ID}) {
		t.Errorf("approved = %+v, want in_progress, owner bob, participants [bob cy], key k1", w)
	}
	again, err := s.ApproveWorkItem(ctx, 1, &ann, nil, "k1")
	if err != nil || *again.OwnerBotID != bob.ID || !again.ApprovedAt.Equal(*w.ApprovedAt) {
		t.Errorf("same key again = %+v, %v, want the item unchanged", again, err)
	}
	if _, err := s.ApproveWorkItem(ctx, 1, &bob.ID, nil, "k2"); !errors.Is(err, ErrConflict) {
		t.Errorf("another key = %v, want ErrConflict", err)
	}
	if _, err := s.ApproveWorkItem(ctx, 2, &ann, nil, "k1"); !errors.Is(err, ErrConflict) {
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
	w, err := s.ApproveWorkItem(ctx, 1, &ann, nil, "k")
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
	if _, err := s.ApproveWorkItem(ctx, 1, &ann, nil, "k"); err != nil {
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
