package store

import (
	"context"
	"fmt"
	"slices"
	"testing"
)

func TestSendBackKeepsTheAssignmentAndItsProof(t *testing.T) {
	ctx := context.Background()
	s, ann := queueFixture(t, 1)
	bob, err := s.CreateBot(ctx, Bot{Name: "bob", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateProject(ctx, Project{Name: "docs", Path: "/tmp/docs"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApproveWorkItem(ctx, 1, &ann, []int64{bob.ID}, nil, "k1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddWorkItemTarget(ctx, 1, 2, "user"); err != nil {
		t.Fatal(err)
	}
	one := int64(1)
	var repos []ProofRepository
	for _, p := range []int64{1, 2} {
		wk, err := s.EnsureWorkspace(ctx, Workspace{
			ProjectID: p, WorkItemID: &one, Dir: fmt.Sprint("/data/item-1/", p), Branch: "work-item/1",
		})
		if err != nil {
			t.Fatal(err)
		}
		repos = append(repos, ProofRepository{WorkspaceID: wk.ID, ProjectID: p, Branch: "work-item/1", Head: "abc", Dir: wk.Dir})
	}
	if _, err := s.CreateSpace(ctx, Space{ProjectID: 1, WorkItemID: &one, Name: "item"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SubmitForReview(ctx, 1, nil, Proof{SpaceID: 1, Repositories: repos}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MoveWorkItem(ctx, 1, _done, ByOwner); err == nil {
		t.Error("the owner accepted Done, want only the user")
	}

	w, err := s.SendBack(ctx, 1, "docs page is missing", ByUser)
	if err != nil {
		t.Fatal(err)
	}
	if w.Status != _inProgress || *w.OwnerBotID != ann || !slices.Equal(w.ParticipantBotIDs, []int64{ann, bob.ID}) || len(w.Targets) != 2 {
		t.Errorf("sent back = %+v, want in_progress with owner ann, participants [ann bob], 2 targets", w)
	}
	ws, err := s.WorkItemWorkspaces(ctx, 1)
	if err != nil || len(ws) != 2 || ws[0].ID != repos[0].WorkspaceID || ws[1].ID != repos[1].WorkspaceID {
		t.Errorf("workspaces after send back = %+v, %v, want the same two rows", ws, err)
	}
	p, err := s.Proof(ctx, 1)
	if err != nil || len(p.Repositories) != 2 || p.Repositories[1].Head != "abc" {
		t.Errorf("proof after send back = %+v, %v, want the two repositories kept", p, err)
	}
	if claimed, err := s.ClaimWrap(ctx, 1); err != nil || claimed {
		t.Errorf("ClaimWrap after send back = %v, %v, want false", claimed, err)
	}
}
