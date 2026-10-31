package store

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestWorkItemKeepsTargetsAndParticipantsOnOneRow(t *testing.T) {
	ctx := context.Background()
	s, _ := queueFixture(t, 0)
	api, err := s.CreateProject(ctx, Project{Name: "api", Path: "/tmp/api"})
	if err != nil {
		t.Fatal(err)
	}
	var bots []int64
	for _, name := range []string{"bea", "cal", "dan"} {
		b, err := s.CreateBot(ctx, Bot{Name: name, Backend: "claude"})
		if err != nil {
			t.Fatal(err)
		}
		bots = append(bots, b.ID)
	}
	item, err := s.CreateWorkItem(ctx, WorkItem{ProjectID: 1, Title: "Rename the field in both repos"})
	if err != nil {
		t.Fatal(err)
	}
	if item, err = s.SetWorkItemParticipants(ctx, item.ID, &bots[0], []int64{bots[1], bots[2], bots[1]}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddWorkItemTarget(ctx, item.ID, api.ID, "user"); err != nil {
		t.Fatal(err)
	}
	sub, err := s.CreateWorkItem(ctx, WorkItem{ProjectID: 1, ParentID: &item.ID, Title: "api side"})
	if err != nil {
		t.Fatal(err)
	}

	var rows int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM work_item WHERE parent_id IS NULL").Scan(&rows); err != nil || rows != 1 {
		t.Errorf("top-level rows = %d, %v, want 1", rows, err)
	}
	got, err := s.WorkItem(ctx, sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.OwnerBotID == nil || *got.OwnerBotID != bots[0] || !reflect.DeepEqual(got.ParticipantBotIDs, bots) {
		t.Errorf("sub-item owner %v participants %v, want %d and %v", got.OwnerBotID, got.ParticipantBotIDs, bots[0], bots)
	}
	var targets []string
	for _, tg := range got.Targets {
		targets = append(targets, tg.ProjectName+"/"+tg.AuthorizedBy)
	}
	if want := []string{"reef/user", "api/user"}; !reflect.DeepEqual(targets, want) {
		t.Errorf("sub-item targets = %v, want %v", targets, want)
	}

	if _, err := s.AddWorkItemTarget(ctx, item.ID, api.ID, "user"); !errors.Is(err, ErrConflict) {
		t.Errorf("duplicate target = %v, want ErrConflict", err)
	}
	if _, err := s.AddWorkItemTarget(ctx, sub.ID, api.ID, "user"); !errors.Is(err, ErrInvalid) {
		t.Errorf("target on a sub-item = %v, want ErrInvalid", err)
	}
	if _, err := s.CreateWorkItem(ctx, WorkItem{ProjectID: 1, ParentID: &item.ID, Title: "x", OwnerBotID: &bots[1]}); !errors.Is(err, ErrInvalid) {
		t.Errorf("sub-item with its own owner = %v, want ErrInvalid", err)
	}
}

func TestAssociationIsNotATarget(t *testing.T) {
	ctx := context.Background()
	s, _ := queueFixture(t, 1)
	other, err := s.CreateProject(ctx, Project{Name: "api", Path: "/tmp/api"})
	if err != nil {
		t.Fatal(err)
	}
	space, err := s.CreateSpace(ctx, Space{ProjectID: 1, Name: "item"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddSpaceProject(ctx, space.ID, other.ID, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AttachSpace(ctx, space.ID, 1); err != nil {
		t.Fatal(err)
	}
	w, err := s.WorkItem(ctx, 1)
	if err != nil || len(w.Targets) != 1 || w.Targets[0].ProjectID != 1 {
		t.Errorf("targets = %+v, %v, want only the home project", w.Targets, err)
	}
}

func TestOwnerAndParticipantsChangeOnlyInBacklog(t *testing.T) {
	ctx := context.Background()
	s, bot := queueFixture(t, 1)
	if _, err := s.UpdateWorkItem(ctx, WorkItem{ID: 1, Title: "renamed", OwnerBotID: &bot}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApproveWorkItem(ctx, 1, nil, nil, nil, "k"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetWorkItemParticipants(ctx, 1, nil, nil); !errors.Is(err, ErrInvalid) {
		t.Errorf("owner change after start = %v, want ErrInvalid", err)
	}
	if _, err := s.UpdateWorkItem(ctx, WorkItem{ID: 1, Title: "again", OwnerBotID: &bot, ParticipantBotIDs: []int64{bot}}); err != nil {
		t.Errorf("update with unchanged owner = %v, want nil", err)
	}
}
