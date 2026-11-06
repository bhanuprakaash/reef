package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestSubmitForReview(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.CreateProject(ctx, Project{Name: "reef", Path: "/tmp/reef"}); err != nil {
		t.Fatal(err)
	}
	lines := []AcceptanceLine{{Text: "wrong password is refused"}, {Text: "session lasts a day"}, {Text: "logout clears it"}}
	if _, err := s.CreateWorkItem(ctx, WorkItem{ProjectID: 1, Title: "Login", Status: _inProgress, AcceptanceLines: lines}); err != nil {
		t.Fatal(err)
	}
	parent := int64(1)
	if _, err := s.CreateWorkItem(ctx, WorkItem{ProjectID: 1, ParentID: &parent, Title: "Form", Status: _inProgress}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateSpace(ctx, Space{ProjectID: 1, WorkItemID: &parent, Name: "login"}); err != nil {
		t.Fatal(err)
	}
	itemID := int64(1)
	wk, err := s.EnsureWorkspace(ctx, Workspace{ProjectID: 1, WorkItemID: &itemID, Dir: "/data/worktrees/item-1/1-reef", Branch: "work-item/1"})
	if err != nil {
		t.Fatal(err)
	}
	proof := Proof{
		SpaceID: 1,
		Commands: []ProofCommand{
			{Command: "go test ./...", Output: "ok  reef/server 1.2s", Project: "reef", ProjectID: 1},
			{Command: "make lint", Output: "clean"},
		},
		Repositories: []ProofRepository{{
			WorkspaceID: wk.ID, ProjectID: 1, Branch: "work-item/1", Head: "abc123", BaseRef: "refs/remotes/origin/main",
			Dir: "/data/worktrees/item-1/1-reef", Dirty: true,
		}},
	}

	_, err = s.SubmitForReview(ctx, 1, []LineTick{{Position: 1, Checked: true}}, proof)
	if !errors.Is(err, ErrInvalid) || err.Error() != `invalid: work item 1 cannot go to review while sub-item 2 "Form" is in_progress` {
		t.Errorf("submit with an open sub-item = %v, want ErrInvalid naming sub-item 2", err)
	}
	if got, err := s.WorkItem(ctx, 1); err != nil || got.Status != _inProgress || got.AcceptanceLines[0].Checked {
		t.Errorf("after the refusal item 1 = %+v, %v, want in_progress with line 1 unticked", got, err)
	}
	if _, err := s.Proof(ctx, 1); !errors.Is(err, ErrNotFound) {
		t.Errorf("proof after the refusal = %v, want ErrNotFound", err)
	}
	if _, err := s.MoveWorkItem(ctx, 2, _done, ByOwner); err != nil {
		t.Fatal(err)
	}

	_, err = s.SubmitForReview(ctx, 1, []LineTick{{Position: 4, Checked: true}}, proof)
	if !errors.Is(err, ErrInvalid) || err.Error() != "invalid: work item 1 has no acceptance line 4" {
		t.Errorf("submit with line 4 = %v, want ErrInvalid naming line 4", err)
	}
	_, err = s.SubmitForReview(ctx, 2, nil, proof)
	if !errors.Is(err, ErrInvalid) || err.Error() != "invalid: work item 2 is a sub-item, submit its parent 1" {
		t.Errorf("submit a sub-item = %v, want ErrInvalid naming parent 1", err)
	}

	ticks := []LineTick{{Position: 1, Checked: true, Note: "401 on a bad password"}, {Position: 2, Checked: true}}
	w, err := s.SubmitForReview(ctx, 1, ticks, proof)
	if err != nil || w.Status != _review {
		t.Fatalf("submit = %+v, %v, want review", w, err)
	}
	got, err := s.Proof(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.SubmittedAt.IsZero() {
		t.Error("proof has no submittedAt")
	}
	got.SubmittedAt = time.Time{}
	b, _ := json.Marshal(got)
	want := `{"spaceId":1,"acceptanceLines":[` +
		`{"position":1,"text":"wrong password is refused","checked":true,"note":"401 on a bad password"},` +
		`{"position":2,"text":"session lasts a day","checked":true,"note":""},` +
		`{"position":3,"text":"logout clears it","checked":false,"note":""}],` +
		`"commands":[{"command":"make lint","output":"clean"}],` +
		`"repositories":[{"workspaceId":1,"projectId":1,"projectName":"reef","branch":"work-item/1","head":"abc123",` +
		`"baseRef":"refs/remotes/origin/main","dir":"/data/worktrees/item-1/1-reef","dirty":true,` +
		`"checks":[{"command":"go test ./...","output":"ok  reef/server 1.2s","project":"reef","projectId":1}]}],` +
		`"screenshots":[],"submittedAt":"0001-01-01T00:00:00Z"}`
	if string(b) != want {
		t.Errorf("proof = %s, want %s", b, want)
	}

	if claimed, err := s.ClaimWrap(ctx, 1); err != nil || !claimed {
		t.Errorf("first ClaimWrap = %v, %v, want true", claimed, err)
	}
	if claimed, err := s.ClaimWrap(ctx, 1); err != nil || claimed {
		t.Errorf("second ClaimWrap = %v, %v, want false", claimed, err)
	}
}

func TestProofKeepsRepositoryWhenProjectIsDeleted(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	home, err := s.CreateProject(ctx, Project{Name: "app", Path: "/tmp/app"})
	if err != nil {
		t.Fatal(err)
	}
	target, err := s.CreateProject(ctx, Project{Name: "library", Path: "/tmp/library"})
	if err != nil {
		t.Fatal(err)
	}
	item, err := s.CreateWorkItem(ctx, WorkItem{
		ProjectID: home.ID, Title: "Use library", Status: _inProgress,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddWorkItemTarget(ctx, item.ID, target.ID, "user"); err != nil {
		t.Fatal(err)
	}
	space, err := s.CreateSpace(ctx, Space{
		ProjectID: home.ID, WorkItemID: &item.ID, Name: item.Title,
	})
	if err != nil {
		t.Fatal(err)
	}
	homeWorkspace, err := s.EnsureWorkspace(ctx, Workspace{
		ProjectID: home.ID, WorkItemID: &item.ID, Dir: "/tmp/app-worktree", Branch: "work-item/1",
	})
	if err != nil {
		t.Fatal(err)
	}
	targetWorkspace, err := s.EnsureWorkspace(ctx, Workspace{
		ProjectID: target.ID, WorkItemID: &item.ID, Dir: "/tmp/library-worktree", Branch: "work-item/1",
	})
	if err != nil {
		t.Fatal(err)
	}
	proof := Proof{
		SpaceID: space.ID,
		Repositories: []ProofRepository{
			{WorkspaceID: homeWorkspace.ID, ProjectID: home.ID, Branch: "work-item/1", Dir: homeWorkspace.Dir},
			{WorkspaceID: targetWorkspace.ID, ProjectID: target.ID, Branch: "work-item/1", Dir: targetWorkspace.Dir},
		},
	}
	if _, err := s.SubmitForReview(ctx, item.ID, nil, proof); err != nil {
		t.Fatal(err)
	}

	if _, err := s.MarkProjectDeleted(ctx, target.ID); err != nil {
		t.Fatal(err)
	}

	got, err := s.Proof(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Repositories) != 2 {
		t.Fatalf("proof repositories = %+v, want home and deleted target", got.Repositories)
	}
	deleted := got.Repositories[1]
	if deleted.ProjectID != target.ID || deleted.ProjectName != "library" ||
		deleted.WorkspaceID != targetWorkspace.ID || deleted.Dir != targetWorkspace.Dir {
		t.Errorf("deleted project proof = %+v, want the retained row of library", deleted)
	}
	workItem, err := s.WorkItem(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(workItem.Targets) != 2 || workItem.Targets[1].ProjectName != "library" {
		t.Errorf("targets after project deletion = %+v, want the library target kept", workItem.Targets)
	}
}
