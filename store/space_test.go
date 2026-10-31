package store

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/bhanuprakaash/reef/agent"
)

func TestSpace_NilTeamIsNoTeam(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	project, err := s.CreateProject(ctx, Project{Name: "shop", Path: "/tmp/shop"})
	if err != nil {
		t.Fatal(err)
	}
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO space (project_id, name, created_at, team_id) VALUES (?, ?, ?, NULL)",
		project.ID, "chat", time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		t.Fatal(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddSpaceProject(ctx, id, project.ID); err != nil {
		t.Fatal(err)
	}

	ch, err := s.Space(ctx, id)
	if err != nil || ch.Team != "" {
		t.Errorf("Space(%d) = %+v, %v, want no team", id, ch, err)
	}
	spaces, err := s.Spaces(ctx, SpaceFilter{ProjectID: &project.ID})
	if err != nil || len(spaces) != 1 || spaces[0].Team != "" {
		t.Errorf("Spaces() = %+v, %v, want one Space with no team", spaces, err)
	}
}

func TestSpace_SubItemHasNone(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.CreateProject(ctx, Project{Name: "shop", Path: "/tmp/shop"}); err != nil {
		t.Fatal(err)
	}
	parent, err := s.CreateWorkItem(ctx, WorkItem{ProjectID: 1, Title: "login"})
	if err != nil {
		t.Fatal(err)
	}
	sub, err := s.CreateWorkItem(ctx, WorkItem{ProjectID: 1, ParentID: &parent.ID, Title: "form"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateSpace(ctx, Space{ProjectID: 1, WorkItemID: &sub.ID, Name: "form"}); !errors.Is(err, ErrInvalid) {
		t.Errorf("CreateSpace for a sub-item = %v, want ErrInvalid", err)
	}
	free, err := s.CreateSpace(ctx, Space{ProjectID: 1, Name: "free"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AttachSpace(ctx, free.ID, sub.ID); !errors.Is(err, ErrInvalid) {
		t.Errorf("AttachSpace to a sub-item = %v, want ErrInvalid", err)
	}
}

func TestSpaceTokens_ExcludesCacheReads(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.CreateProject(ctx, Project{Name: "shop", Path: "/tmp/shop"}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a", "b"} {
		if _, err := s.CreateSpace(ctx, Space{ProjectID: 1, Name: name}); err != nil {
			t.Fatal(err)
		}
	}
	turns := []struct {
		spaceID int64
		usage   agent.Usage
	}{
		{1, agent.Usage{InputTokens: 10, OutputTokens: 5, CacheReadTokens: 1000}},
		{1, agent.Usage{InputTokens: 20, OutputTokens: 7, CacheWriteTokens: 500}},
		{2, agent.Usage{InputTokens: 99, OutputTokens: 99}},
	}
	for _, u := range turns {
		tr, err := s.CreateTurn(ctx, Turn{SpaceID: u.spaceID, Role: "lead", Backend: "claude"})
		if err != nil {
			t.Fatal(err)
		}
		tr.Status, tr.Usage = "done", u.usage
		if _, err := s.EndTurn(ctx, tr); err != nil {
			t.Fatal(err)
		}
	}

	if got, err := s.SpaceTokens(ctx, 1); err != nil || got != 542 {
		t.Errorf("SpaceTokens(1) = %d, %v, want 542", got, err)
	}
}

func TestSpaceBudget_SharedByTheWorkItem(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.CreateProject(ctx, Project{Name: "shop", Path: "/tmp/shop"}); err != nil {
		t.Fatal(err)
	}
	item, err := s.CreateWorkItem(ctx, WorkItem{ProjectID: 1, Title: "login"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.CreateSpace(ctx, Space{ProjectID: 1, WorkItemID: &item.ID, Name: "first"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, "UPDATE space SET closed_at = ? WHERE id = ?",
		time.Now().UTC().Format(time.RFC3339Nano), first.ID); err != nil {
		t.Fatal(err)
	}
	second, err := s.CreateSpace(ctx, Space{ProjectID: 1, WorkItemID: &item.ID, Name: "second"})
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range []struct {
		spaceID int64
		tokens  int64
	}{{first.ID, 600_000}, {second.ID, 500_000}} {
		tr, err := s.CreateTurn(ctx, Turn{SpaceID: u.spaceID, Role: "lead", Backend: "claude"})
		if err != nil {
			t.Fatal(err)
		}
		tr.Status, tr.Usage = "done", agent.Usage{InputTokens: u.tokens}
		if _, err := s.EndTurn(ctx, tr); err != nil {
			t.Fatal(err)
		}
	}

	if got, err := s.SpaceTokens(ctx, second.ID); err != nil || got != 1_100_000 {
		t.Errorf("SpaceTokens(second) = %d, %v, want 1100000: both Spaces count", got, err)
	}
	if err := s.PauseSpace(ctx, second.ID, "capped"); err != nil {
		t.Fatal(err)
	}
	if err := s.ContinueSpace(ctx, second.ID, 2_100_000); err != nil {
		t.Fatal(err)
	}
	if got, err := s.WorkItem(ctx, item.ID); err != nil || got.TokenBudget != 2_100_000 {
		t.Errorf("work item budget = %d, %v, want 2100000", got.TokenBudget, err)
	}
	if got, err := s.Space(ctx, second.ID); err != nil || got.TokenBudget != 2_100_000 {
		t.Errorf("Space budget = %d, %v, want the work item's 2100000", got.TokenBudget, err)
	}
}

func TestPauseAndContinueSpace(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.CreateProject(ctx, Project{Name: "shop", Path: "/tmp/shop"}); err != nil {
		t.Fatal(err)
	}
	ch, err := s.CreateSpace(ctx, Space{ProjectID: 1, Name: "chat"})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := s.Space(ctx, ch.ID); err != nil || got.Paused != "" || got.TokenBudget != 1000000 {
		t.Fatalf("new Space = %+v, %v, want not paused with budget 1000000", got, err)
	}

	if err := s.PauseSpace(ctx, ch.ID, "capped"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Space(ctx, ch.ID); got.Paused != "capped" {
		t.Errorf("Paused = %q, want capped", got.Paused)
	}
	if err := s.ContinueSpace(ctx, ch.ID, 1000500); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Space(ctx, ch.ID); got.Paused != "" || got.TokenBudget != 1000500 {
		t.Errorf("after continue = paused %q budget %d, want not paused, 1000500",
			got.Paused, got.TokenBudget)
	}
	if err := s.PauseSpace(ctx, ch.ID, ""); !errors.Is(err, ErrInvalid) {
		t.Errorf("PauseSpace with empty reason = %v, want ErrInvalid", err)
	}
	if err := s.PauseSpace(ctx, 99, "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("PauseSpace on unknown Space = %v, want ErrNotFound", err)
	}
}

func TestSpace_NameSteps(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	p, err := s.CreateProject(ctx, Project{Name: "shop", Path: "/tmp/shop"})
	if err != nil {
		t.Fatal(err)
	}
	mk := func(name string) int64 {
		t.Helper()
		c, err := s.CreateSpace(ctx, Space{ProjectID: p.ID, Name: name})
		if err != nil {
			t.Fatal(err)
		}
		return c.ID
	}

	typed := mk("typed")
	if ok, err := s.ClaimName(ctx, typed); err != nil || ok {
		t.Errorf("ClaimName on a user-named Space = %v, %v, want false", ok, err)
	}

	pending := mk("claude 02:32 PM")
	if err := s.MarkNamePending(ctx, pending); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.ClaimName(ctx, pending); err != nil || !ok {
		t.Fatalf("first ClaimName = %v, %v, want true", ok, err)
	}
	if ok, err := s.ClaimName(ctx, pending); err != nil || ok {
		t.Errorf("second ClaimName = %v, %v, want false", ok, err)
	}
	if ok, err := s.FinishName(ctx, pending, "login redirect"); err != nil || !ok {
		t.Fatalf("FinishName = %v, %v, want true", ok, err)
	}
	if c, err := s.Space(ctx, pending); err != nil || c.Name != "login redirect" {
		t.Errorf("name = %q, %v, want login redirect", c.Name, err)
	}
	if ok, err := s.FinishName(ctx, pending, "again"); err != nil || ok {
		t.Errorf("second FinishName = %v, %v, want false", ok, err)
	}

	failed := mk("claude 02:33 PM")
	if err := s.MarkNamePending(ctx, failed); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimName(ctx, failed); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.FinishName(ctx, failed, ""); err != nil || !ok {
		t.Fatalf("FinishName with no name = %v, %v, want true", ok, err)
	}
	if c, err := s.Space(ctx, failed); err != nil || c.Name != "claude 02:33 PM" {
		t.Errorf("name = %q, %v, want the default name kept", c.Name, err)
	}

	renamed := mk("claude 02:34 PM")
	if err := s.MarkNamePending(ctx, renamed); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimName(ctx, renamed); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateSpace(ctx, Space{ID: renamed, Name: "mine"}); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.FinishName(ctx, renamed, "proposed"); err != nil || ok {
		t.Errorf("FinishName after a rename = %v, %v, want false", ok, err)
	}
	if c, err := s.Space(ctx, renamed); err != nil || c.Name != "mine" {
		t.Errorf("name = %q, %v, want mine", c.Name, err)
	}

	same := mk("claude 02:35 PM")
	if err := s.MarkNamePending(ctx, same); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateSpace(ctx, Space{ID: same, Name: "claude 02:35 PM"}); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.ClaimName(ctx, same); err != nil || !ok {
		t.Errorf("ClaimName after an update that kept the name = %v, %v, want true", ok, err)
	}
}

func TestSpace_RoleFollowsLeadBotRename(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	project, err := s.CreateProject(ctx, Project{Name: "shop", Path: "/tmp/shop"})
	if err != nil {
		t.Fatal(err)
	}
	sam, err := s.CreateBot(ctx, Bot{Name: "sam", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.CreateBotSpace(ctx, Space{ProjectID: project.ID, Name: "chat", Role: "sam"}, sam.ID)
	if err != nil {
		t.Fatal(err)
	}
	if c.BotID == nil || *c.BotID != sam.ID || c.Role != "sam" {
		t.Fatalf("created Space botId = %v, role = %q", c.BotID, c.Role)
	}
	sam.Name = "sally"
	if _, err = s.UpdateBot(ctx, sam); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreateBot(ctx, Bot{Name: "sam", Backend: "claude"}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Space(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Role != "sally" || got.BotID == nil || *got.BotID != sam.ID {
		t.Fatalf("renamed Space role = %q, botId = %v, want sally, %d", got.Role, got.BotID, sam.ID)
	}
	previews, err := s.SpacePreviews(ctx, SpaceFilter{})
	if err != nil || len(previews) != 1 || previews[0].Role != "sally" || *previews[0].BotID != sam.ID {
		t.Fatalf("previews = %+v, %v", previews, err)
	}
}

func TestSpace_WithoutProjectSurvivesReopen(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	bot, err := s.CreateBot(ctx, Bot{Name: "ada", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	chat, err := s.CreateBotSpace(ctx, Space{Name: "ideas", Kind: KindConversation}, bot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateBotSpace(ctx, Space{Name: "scratch"}, bot.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateMessage(ctx, Message{SpaceID: chat.ID, Author: "user", Body: "hello"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	spaces, err := s.Spaces(ctx, SpaceFilter{})
	if err != nil || len(spaces) != 2 ||
		spaces[0].ID != 1 || spaces[0].Name != "ideas" || spaces[0].Kind != KindConversation || spaces[0].ProjectID != 0 ||
		spaces[1].ID != 2 || spaces[1].Name != "scratch" || spaces[1].Kind != KindFree || spaces[1].ProjectID != 0 {
		t.Fatalf("Spaces() = %+v, %v, want conversation 1 and free Space 2, both without a project", spaces, err)
	}
	msgs, err := s.Messages(ctx, chat.ID, 0)
	if err != nil || len(msgs) != 1 || msgs[0].Body != "hello" {
		t.Errorf("Messages(%d) = %+v, %v, want the one message hello", chat.ID, msgs, err)
	}
}

func TestSpace_FilterByProjectUsesAssociation(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	for _, name := range []string{"shop", "blog"} {
		if _, err := s.CreateProject(ctx, Project{Name: name, Path: "/tmp/" + name}); err != nil {
			t.Fatal(err)
		}
	}
	bot, err := s.CreateBot(ctx, Bot{Name: "ada", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []Space{
		{Name: "global", Kind: KindConversation},
		{Name: "in shop", Kind: KindConversation, ProjectID: 1},
		{Name: "in blog", Kind: KindFree, ProjectID: 2},
	} {
		if _, err := s.CreateBotSpace(ctx, c, bot.ID); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.AddSpaceProject(ctx, 1, 2); err != nil {
		t.Fatal(err)
	}
	if err := s.AddSpaceProject(ctx, 1, 2); err != nil {
		t.Fatalf("adding the same project twice: %v", err)
	}

	projectNames := func(projectID int64) []string {
		spaces, err := s.Spaces(ctx, SpaceFilter{ProjectID: &projectID})
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, c := range spaces {
			names = append(names, c.Name)
		}
		return names
	}
	if got, want := projectNames(1), []string{"in shop"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Spaces of project 1 = %v, want %v", got, want)
	}
	if got, want := projectNames(2), []string{"global", "in blog"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Spaces of project 2 = %v, want %v", got, want)
	}

	if err := s.RemoveSpaceProject(ctx, 1, 2); err != nil {
		t.Fatal(err)
	}
	if got := projectNames(2); !reflect.DeepEqual(got, []string{"in blog"}) {
		t.Errorf("Spaces of project 2 after remove = %v, want [in blog]", got)
	}
	global, err := s.Space(ctx, 1)
	if err != nil || global.ProjectID != 0 {
		t.Errorf("Space 1 after remove = %+v, %v, want no project", global, err)
	}
}

func TestSpace_WorkItemSpaceKeepsItsProject(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.CreateProject(ctx, Project{Name: "shop", Path: "/tmp/shop"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateWorkItem(ctx, WorkItem{ProjectID: 1, Title: "login"}); err != nil {
		t.Fatal(err)
	}
	one := int64(1)
	if _, err := s.CreateSpace(ctx, Space{Name: "login", WorkItemID: &one}); !errors.Is(err, ErrInvalid) {
		t.Errorf("work item Space with no project: err = %v, want ErrInvalid", err)
	}
	c, err := s.CreateSpace(ctx, Space{Name: "login", ProjectID: 1, WorkItemID: &one})
	if err != nil || c.ProjectID != 1 {
		t.Fatalf("CreateSpace = %+v, %v, want project 1", c, err)
	}
	if err := s.RemoveSpaceProject(ctx, c.ID, 1); !errors.Is(err, ErrInvalid) {
		t.Errorf("RemoveSpaceProject on a work item Space: err = %v, want ErrInvalid", err)
	}
}
