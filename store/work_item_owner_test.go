package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
)

func TestOwnerTransfer_SurvivesARestartAndHandsOverOnce(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateProject(ctx, Project{Name: "reef", Path: "/tmp/reef"}); err != nil {
		t.Fatal(err)
	}
	var bots []int64
	for _, name := range []string{"ann", "bea"} {
		b, err := s.CreateBot(ctx, Bot{Name: name, Backend: "claude"})
		if err != nil {
			t.Fatal(err)
		}
		bots = append(bots, b.ID)
	}
	item, err := s.CreateWorkItem(ctx, WorkItem{ProjectID: 1, Title: "Login", OwnerBotID: &bots[0]})
	if err != nil {
		t.Fatal(err)
	}
	sp, err := s.CreateSpaceOf(ctx, Space{ProjectID: 1, WorkItemID: &item.ID, Name: "Login"}, bots[:1], 0)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.RequestOwnerTransfer(ctx, item.ID, bots[1]); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if s, err = Open(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	if got, err := s.PendingOwnerTransfers(ctx, 0); err != nil || !slices.Equal(got, []int64{item.ID}) {
		t.Fatalf("pending after restart = %v, %v, want [%d]", got, err, item.ID)
	}
	dm, made, err := s.ApplyOwnerTransfer(ctx, item.ID)
	if err != nil || !made || dm.State != DirectWaiting || *dm.ToBotID != bots[1] ||
		*dm.RequestKey != fmt.Sprintf("owner:1:%d:0", bots[1]) {
		t.Fatalf("apply = %+v, %v, %v, want one waiting handover to bea with key owner:1:<bea>:0", dm, made, err)
	}
	if _, made, err := s.ApplyOwnerTransfer(ctx, item.ID); err != nil || made {
		t.Errorf("second apply = %v, %v, want nothing", made, err)
	}

	got, err := s.WorkItem(ctx, item.ID)
	if err != nil || *got.OwnerBotID != bots[1] || got.PendingOwnerBotID != nil ||
		!slices.Equal(got.ParticipantBotIDs, []int64{bots[0], bots[1]}) {
		t.Errorf("item = owner %v pending %v participants %v, %v, want bea owning, ann and bea participating",
			got.OwnerBotID, got.PendingOwnerBotID, got.ParticipantBotIDs, err)
	}
	team, err := s.SpaceTeam(ctx, sp.ID)
	if err != nil || len(team.Roles) != 2 || team.Roles[0].Name != "bea" || !team.Roles[0].Lead || team.Roles[1].Lead {
		t.Errorf("Space roles = %+v, %v, want bea leading, ann a member", team.Roles, err)
	}
	var handovers int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM direct_message WHERE request_key LIKE 'owner:%'").
		Scan(&handovers); err != nil || handovers != 1 {
		t.Errorf("handover exchanges = %d, %v, want 1", handovers, err)
	}

	if _, err := s.RequestOwnerTransfer(ctx, item.ID, bots[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RequestOwnerTransfer(ctx, item.ID, bots[1]); err != nil {
		t.Fatal(err)
	}
	if got, err := s.PendingOwnerTransfers(ctx, 0); err != nil || len(got) != 0 {
		t.Errorf("pending after a request for the current owner = %v, %v, want none", got, err)
	}
	if _, err := s.SetSpaceCoordinator(ctx, sp.ID, bots[0]); err != nil {
		t.Fatal(err)
	}
	if got, err := s.WorkItem(ctx, item.ID); err != nil || *got.OwnerBotID != bots[1] {
		t.Errorf("owner after a coordinator change = %v, %v, want bea", got.OwnerBotID, err)
	}
}

func TestOwnerTransfer_RefusesSubItemsClosedItemsAndUnknownBots(t *testing.T) {
	ctx := context.Background()
	s, bot := queueFixture(t, 1)
	parent := int64(1)
	sub, err := s.CreateWorkItem(ctx, WorkItem{ProjectID: 1, ParentID: &parent, Title: "part"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RequestOwnerTransfer(ctx, sub.ID, bot); !errors.Is(err, ErrInvalid) {
		t.Errorf("sub-item = %v, want ErrInvalid", err)
	}
	if _, err := s.RequestOwnerTransfer(ctx, 1, 99); !errors.Is(err, ErrInvalid) {
		t.Errorf("unknown Bot = %v, want ErrInvalid", err)
	}
	if _, err := s.db.Exec("UPDATE work_item SET status = 'dropped' WHERE id = 1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RequestOwnerTransfer(ctx, 1, bot); !errors.Is(err, ErrInvalid) {
		t.Errorf("dropped item = %v, want ErrInvalid", err)
	}
}
