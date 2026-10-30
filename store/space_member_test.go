package store

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func spaceMemberStore(t *testing.T) *Store {
	t.Helper()
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if _, err := s.CreateProject(ctx, Project{Name: "shop", Path: "/tmp/shop"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyTeam(ctx, Team{Name: "core", Roles: []Role{
		{Name: "boss", Lead: true, Backend: "claude"},
		{Name: "dev", Backend: "claude", Write: true, Tools: []string{"Read"}},
	}}); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestCreateSpace_RefusesSecondActiveSpaceOfWorkItem(t *testing.T) {
	ctx := context.Background()
	s := spaceMemberStore(t)
	w, err := s.CreateWorkItem(ctx, WorkItem{ProjectID: 1, Title: "Login"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.CreateSpace(ctx, Space{ProjectID: 1, WorkItemID: &w.ID, Name: "one", Team: "core"})
	if err != nil || first.Kind != KindWorkItem {
		t.Fatalf("first Space = %+v, %v, want kind work_item", first, err)
	}

	_, err = s.CreateSpace(ctx, Space{ProjectID: 1, WorkItemID: &w.ID, Name: "two"})
	if !errors.Is(err, ErrConflict) || !strings.Contains(err.Error(), "work item 1 already has an active Space") {
		t.Fatalf("second active Space = %v, want ErrConflict naming work item 1", err)
	}

	if _, err := s.db.ExecContext(ctx,
		"UPDATE space SET closed_at = '2026-01-01T00:00:00Z' WHERE id = ?", first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateSpace(ctx, Space{ProjectID: 1, WorkItemID: &w.ID, Name: "two"}); err != nil {
		t.Errorf("Space after the first closed = %v, want nil", err)
	}
	if err := s.ReopenSpace(ctx, first.ID); !errors.Is(err, ErrConflict) {
		t.Errorf("reopen the first while the second is active = %v, want ErrConflict", err)
	}
}

func TestSpaceTeam_UsesProfileConfiguration(t *testing.T) {
	ctx := context.Background()
	s := spaceMemberStore(t)
	one, err := s.CreateSpace(ctx, Space{ProjectID: 1, Name: "one", Team: "core"})
	if err != nil {
		t.Fatal(err)
	}
	two, err := s.CreateSpace(ctx, Space{ProjectID: 1, Name: "two", Team: "core"})
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.Team(ctx, "core")
	if err != nil {
		t.Fatal(err)
	}
	bot, err := s.Bot(ctx, team.Roles[1].BotID)
	if err != nil {
		t.Fatal(err)
	}
	bot.Tools = []string{"Read", "Grep"}
	bot.Write = false
	if _, err := s.UpdateBot(ctx, bot); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{one.ID, two.ID} {
		got, err := s.SpaceTeam(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if d := got.Roles[1]; d.Write || !reflect.DeepEqual(d.Tools, []string{"Read", "Grep"}) {
			t.Errorf("Space %d member = %+v, want profile settings", id, d)
		}
	}
	team, err = s.Team(ctx, "core")
	if err != nil {
		t.Fatal(err)
	}
	if d := team.Roles[1]; d.Write || !reflect.DeepEqual(d.Tools, []string{"Read", "Grep"}) {
		t.Errorf("team member = %+v, want profile settings", d)
	}
}

func TestBotConversations_NewestActivityFirstAndOnlyOwnConversations(t *testing.T) {
	ctx := context.Background()
	s := spaceMemberStore(t)
	sam, err := s.CreateBot(ctx, Bot{Name: "sam", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	kim, err := s.CreateBot(ctx, Bot{Name: "kim", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	start := func(name string, with Bot, kind string) Space {
		c, err := s.CreateBotSpace(ctx,
			Space{ProjectID: 1, Name: name, Kind: kind, Role: with.Name, Backend: "claude"}, with.ID)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	old := start("old", sam, KindConversation)
	start("new", sam, KindConversation)
	start("kims", kim, KindConversation)
	start("free", sam, KindFree)
	if _, err := s.CreateMessage(ctx, Message{SpaceID: old.ID, Author: "user", Body: "back again"}); err != nil {
		t.Fatal(err)
	}

	got, err := s.BotConversations(ctx, sam.ID)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, c := range got {
		names = append(names, c.Name)
	}
	if want := []string{"old", "new"}; !reflect.DeepEqual(names, want) {
		t.Errorf("sam's conversations = %v, want %v", names, want)
	}
}

func TestBotSpace_HasOneLeadAndTakesMoreOneByOne(t *testing.T) {
	ctx := context.Background()
	s := spaceMemberStore(t)
	sam, err := s.CreateBot(ctx, Bot{Name: "sam", Backend: "claude", Tools: []string{"Read"}})
	if err != nil {
		t.Fatal(err)
	}
	kim, err := s.CreateBot(ctx, Bot{Name: "kim", Backend: "claude", Write: true})
	if err != nil {
		t.Fatal(err)
	}
	chat, err := s.CreateBotSpace(ctx,
		Space{ProjectID: 1, Name: "chat", Kind: KindConversation, Role: "sam", Backend: "claude"}, sam.ID)
	if err != nil || chat.Kind != KindConversation || chat.Team != "" {
		t.Fatalf("conversation = %+v, %v, want kind conversation and no team", chat, err)
	}
	if _, err := s.AddSpaceMember(ctx, chat.ID, kim.ID); !errors.Is(err, ErrInvalid) {
		t.Errorf("add to a conversation = %v, want ErrInvalid", err)
	}

	free, err := s.CreateBotSpace(ctx,
		Space{ProjectID: 1, Name: "free", Role: "sam", Backend: "claude"}, sam.ID)
	if err != nil || free.Kind != KindFree {
		t.Fatalf("free Space = %+v, %v, want kind free", free, err)
	}
	added, err := s.AddSpaceMember(ctx, free.ID, kim.ID)
	if err != nil || added.Name != "kim" || added.Lead || !added.Write {
		t.Fatalf("added = %+v, %v, want kim, not lead, write", added, err)
	}
	if _, err := s.AddSpaceMember(ctx, free.ID, kim.ID); !errors.Is(err, ErrConflict) {
		t.Errorf("add kim twice = %v, want ErrConflict", err)
	}
	team, err := s.SpaceTeam(ctx, free.ID)
	if err != nil || team.Name != "" || len(team.Roles) != 2 || !team.Roles[0].Lead ||
		!reflect.DeepEqual(team.Roles[0].Tools, []string{"Read"}) {
		t.Errorf("SpaceTeam = %+v, %v, want no team, sam (lead, tools [Read]) then kim", team, err)
	}
	if err := s.DeleteBot(ctx, sam.ID); !errors.Is(err, ErrInvalid) {
		t.Errorf("delete a bot that is in Spaces = %v, want ErrInvalid", err)
	}
}

func TestAttachSpace(t *testing.T) {
	ctx := context.Background()
	s := spaceMemberStore(t)
	w, err := s.CreateWorkItem(ctx, WorkItem{ProjectID: 1, Title: "Login"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateSpace(ctx, Space{ProjectID: 1, WorkItemID: &w.ID, Name: "item"}); err != nil {
		t.Fatal(err)
	}
	free, err := s.CreateSpace(ctx, Space{ProjectID: 1, Name: "free", Team: "core"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.AttachSpace(ctx, free.ID, w.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("attach to an item with an active Space = %v, want ErrConflict", err)
	}
	if got, err := s.Space(ctx, free.ID); err != nil || got.WorkItemID != nil || got.Kind != KindFree {
		t.Fatalf("Space after refused attach = %+v, %v, want still free", got, err)
	}

	if _, err := s.db.ExecContext(ctx,
		"UPDATE space SET closed_at = '2026-01-01T00:00:00Z' WHERE id = 1"); err != nil {
		t.Fatal(err)
	}
	got, err := s.AttachSpace(ctx, free.ID, w.ID)
	if err != nil || got.WorkItemID == nil || *got.WorkItemID != w.ID || got.Kind != KindWorkItem {
		t.Fatalf("AttachSpace = %+v, %v, want work item %d and kind work_item", got, err, w.ID)
	}
	if _, err := s.AttachSpace(ctx, free.ID, w.ID); !errors.Is(err, ErrInvalid) {
		t.Errorf("attach an attached Space = %v, want ErrInvalid", err)
	}
	spaces, err := s.Spaces(ctx, SpaceFilter{WorkItemID: &w.ID})
	if err != nil || len(spaces) != 2 {
		t.Errorf("Spaces of the item = %+v, %v, want 2", spaces, err)
	}
}

func TestMoveConversation_FreeSpaceWithBothBotsAndTheConversationMarked(t *testing.T) {
	ctx := context.Background()
	s := spaceMemberStore(t)
	sam, err := s.CreateBot(ctx, Bot{Name: "sam", Backend: "claude", Tools: []string{"Read"}})
	if err != nil {
		t.Fatal(err)
	}
	kim, err := s.CreateBot(ctx, Bot{Name: "kim", Backend: "claude", Write: true})
	if err != nil {
		t.Fatal(err)
	}
	conv, err := s.CreateBotSpace(ctx,
		Space{ProjectID: 1, Name: "talk", Kind: KindConversation, Role: "sam", Backend: "claude"}, sam.ID)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.MoveConversation(ctx, conv.ID, sam.ID); !errors.Is(err, ErrInvalid) {
		t.Errorf("move to its own Bot = %v, want ErrInvalid", err)
	}
	if _, err := s.MoveConversation(ctx, conv.ID, 99); !errors.Is(err, ErrInvalid) {
		t.Errorf("move to an unknown Bot = %v, want ErrInvalid", err)
	}
	moved, err := s.MoveConversation(ctx, conv.ID, kim.ID)
	if err != nil {
		t.Fatal(err)
	}
	if moved.Kind != KindFree || moved.Team != "" || moved.Role != "sam" || moved.Name != "talk" {
		t.Errorf("new Space = %+v, want a free Space named talk led by sam with no Team", moved)
	}
	team, err := s.SpaceTeam(ctx, moved.ID)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, r := range team.Roles {
		got = append(got, fmt.Sprintf("%s lead=%v write=%v tools=%v", r.Name, r.Lead, r.Write, r.Tools))
	}
	want := []string{"sam lead=true write=false tools=[Read]", "kim lead=false write=true tools=[]"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("members = %v, want %v", got, want)
	}
	if c, err := s.Space(ctx, conv.ID); err != nil || c.MovedTo == nil || *c.MovedTo != moved.ID {
		t.Errorf("conversation = %+v, %v, want moved to Space %d", c, err, moved.ID)
	}
	if _, err := s.MoveConversation(ctx, conv.ID, kim.ID); !errors.Is(err, ErrConflict) {
		t.Errorf("second move = %v, want ErrConflict", err)
	}
}
