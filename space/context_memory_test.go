package space

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/memory"
	"github.com/bhanuprakaash/reef/store"
)

func TestContext_ProjectShowsTheNewestTen(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ms := testMemory(t, st)
	for i := 1; i <= 12; i++ {
		remember(t, ms, projectScope(f), memory.KindFact, "user", fmt.Sprintf("project fact %d", i))
	}

	got, err := Context(ctx, st, ms, f.ID, "developer")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "10 of 12; recall(..., scope=project)\n") {
		t.Errorf("Context =\n%s\nwant the count line 10 of 12", got)
	}
	if n := strings.Count(got, "\nfact "); n != 10 {
		t.Errorf("project lines = %d, want 10", n)
	}
	for _, want := range []string{"fact 12: project fact 12 (", "fact 3: project fact 3 ("} {
		if !strings.Contains(got, want) {
			t.Errorf("Context does not contain %q:\n%s", want, got)
		}
	}
	for _, gone := range []string{"project fact 1 (", "project fact 2 ("} {
		if strings.Contains(got, gone) {
			t.Errorf("Context lists %q, one of the two oldest:\n%s", gone, got)
		}
	}
}

func TestContext_RoleSeesOnlyItsOwnItems(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ms := testMemory(t, st)
	for i := 1; i <= 6; i++ {
		remember(t, ms, botScope("developer"), memory.KindLesson, "developer",
			fmt.Sprintf("developer lesson %d", i))
		remember(t, ms, botScope("tester"), memory.KindLesson, "tester",
			fmt.Sprintf("tester lesson %d", i))
	}

	got, err := Context(ctx, st, ms, f.ID, "developer")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "tester lesson") {
		t.Errorf("developer's prompt holds a tester item:\n%s", got)
	}
	if n := strings.Count(got, "\nlesson "); n != 5 {
		t.Errorf("developer lines = %d, want 5", n)
	}
	for _, want := range []string{
		"### Your items\nlesson 11: developer lesson 6 (",
		"5 of 6; recall(..., scope=bot)\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Context does not contain %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "developer lesson 1 (") {
		t.Errorf("Context lists the oldest item:\n%s", got)
	}
}

func TestContext_FindingsAndEmptyParts(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ms := testMemory(t, st)
	remember(t, ms, workScope(f), memory.KindFact, "developer", "the API is v2")
	if _, err := st.CreateDecision(ctx, store.Decision{WorkItemID: f.ID, Body: "old table decision", Author: "user"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutRoleNotes(ctx, store.RoleNotes{WorkItemID: f.ID, Role: "developer", Body: "old notes"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutProjectMemory(ctx, store.ProjectMemory{ProjectID: f.ProjectID, Body: "old project memory"}); err != nil {
		t.Fatal(err)
	}

	got, err := Context(ctx, st, ms, f.ID, "developer")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "### Findings\nfact 1: the API is v2 (") ||
		!strings.Contains(got, "1 of 1; recall(..., scope=work_item)\n") {
		t.Errorf("Context =\n%s\nwant a Findings part with the fact", got)
	}
	for _, gone := range []string{
		"### Brief", "### Decisions", "### Project", "### Team", "### Your items",
		"old table decision", "old notes", "old project memory",
	} {
		if strings.Contains(got, gone) {
			t.Errorf("Context contains %q:\n%s", gone, got)
		}
	}
}

func TestFirstPrompt_QuickSessionHasNoFeatureParts(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ms := testMemory(t, st)
	remember(t, ms, workScope(f), memory.KindDecision, "user", "a feature decision")
	remember(t, ms, projectScope(f), memory.KindFact, "user", "a project fact")
	remember(t, ms, botScope("1"), memory.KindLesson, "lead", "a lead lesson")

	got, err := firstPrompt(ctx, st, ms, f.ProjectID, nil, 0, store.Team{}, store.Role{Name: "lead"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"### Project\nfact 2: a project fact (", "### Your items\nlesson 3: a lead lesson ("} {
		if !strings.Contains(got, want) {
			t.Errorf("quick session =\n%s\nwant it to contain %q", got, want)
		}
	}
	for _, gone := range []string{"### Brief", "### Decisions", "### Findings", "a feature decision"} {
		if strings.Contains(got, gone) {
			t.Errorf("quick session contains %q:\n%s", gone, got)
		}
	}
}

func TestFirstPrompt_FreeSpaceAndConversationShelves(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ms := testMemory(t, st)
	newSpace := func(kind string) store.Space {
		c, err := st.CreateBotSpace(ctx, store.Space{
			Kind: kind, ProjectID: f.ProjectID, Name: "chat", Role: "lead",
		}, 1)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	ref := func(kind string, c store.Space) memory.Ref {
		return memory.Ref{Kind: kind, ID: strconv.FormatInt(c.ID, 10)}
	}
	free, other := newSpace(store.KindFree), newSpace(store.KindFree)
	chat, nextChat := newSpace(store.KindConversation), newSpace(store.KindConversation)
	remember(t, ms, ref(memory.ScopeSpace, free), memory.KindFact, "lead", "free space fact")
	remember(t, ms, ref(memory.ScopeSpace, other), memory.KindFact, "lead", "other space fact")
	remember(t, ms, ref(memory.ScopeConversation, chat), memory.KindFact, "lead", "conversation fact")
	remember(t, ms, projectScope(f), memory.KindFact, "user", "a project fact")
	remember(t, ms, memory.Ref{Kind: memory.ScopeBot, ID: "1"}, memory.KindLesson, "lead", "a lead lesson")

	prompt := func(c store.Space) string {
		got, err := firstPrompt(ctx, st, ms, f.ProjectID, nil, c.ID, store.Team{}, store.Role{Name: "lead"})
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	got := prompt(free)
	for _, want := range []string{"### Space\nfact 1: free space fact (", "### Project\nfact 4: a project fact (",
		"### Your items\nlesson 5: a lead lesson ("} {
		if !strings.Contains(got, want) {
			t.Errorf("free Space =\n%s\nwant it to contain %q", got, want)
		}
	}
	for _, gone := range []string{"### Team", "other space fact", "conversation fact"} {
		if strings.Contains(got, gone) {
			t.Errorf("free Space contains %q:\n%s", gone, got)
		}
	}
	for _, c := range []store.Space{chat, nextChat} {
		got = prompt(c)
		if !strings.Contains(got, "### Your items\nlesson 5: a lead lesson (") {
			t.Errorf("conversation %d =\n%s\nwant the Bot's items", c.ID, got)
		}
		for _, want := range []string{"This is a direct conversation with the user", "saves to this conversation by default"} {
			if !strings.Contains(got, want) {
				t.Errorf("conversation %d =\n%s\nwant it to contain %q", c.ID, got, want)
			}
		}
		for _, gone := range []string{"### Space", "### Project", "### Team", "conversation fact"} {
			if strings.Contains(got, gone) {
				t.Errorf("conversation %d contains %q:\n%s", c.ID, gone, got)
			}
		}
	}
}

func TestOpen_FirstPromptIsWhatReefContextPrints(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ms := testMemory(t, st)
	remember(t, ms, workScope(f), memory.KindDecision, "user", "use sessions")
	remember(t, ms, workScope(f), memory.KindFinding, "tester", "login is slow")
	remember(t, ms, projectScope(f), memory.KindFact, "user", "Go 1.24")
	remember(t, ms, botScope("architect"), memory.KindLesson, "architect", "keep it small")
	ch := sessionSpace(t, st, f, false)
	ag := newRecordingAgent()

	s, err := Open(ctx, st, ms, t.TempDir(), "/bin/reef", ch.ID, NewTrees(t.TempDir()), ag.factory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	want, err := Context(ctx, st, ms, f.ID, "architect")
	if err != nil {
		t.Fatal(err)
	}
	if err := sendAs(ctx, st, ch.ID, s.Run, "hi", discard); err != nil {
		t.Fatal(err)
	}

	if len(ag.prompts) != 1 || ag.prompts[0] != want+"\n\nhi" {
		t.Errorf("prompts = %q, want [%q]", ag.prompts, want+"\n\nhi")
	}
	if !strings.Contains(want, "### Decisions\ndecision 1: use sessions (") {
		t.Errorf("Context =\n%s\nwant the decision in it", want)
	}
}

func TestContext_YourItemsAreTheRolesBot(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	if _, err := testApplyTeam(t, st, ctx, store.Team{Name: "core",
		Roles: []store.Role{{Name: "developer", Lead: true, Backend: "claude"}}}); err != nil {
		t.Fatal(err)
	}
	teamID := int64(2)
	g, err := st.CreateWorkItem(ctx, store.WorkItem{ProjectID: f.ProjectID, Title: "Logout", TeamID: &teamID})
	if err != nil {
		t.Fatal(err)
	}
	ms := testMemory(t, st)
	remember(t, ms, memory.Ref{Kind: memory.ScopeBot, ID: "2"}, memory.KindLesson, "developer", "keep it small")
	remember(t, ms, botScope("developer"), memory.KindLesson, "developer", "kept under the role name")

	for name, id := range map[string]int64{"a work item with a Team": g.ID, "one without": f.ID} {
		got, err := Context(ctx, st, ms, id, "developer")
		if err != nil {
			t.Fatal(err)
		}
		want := "### Your items\nlesson 1: keep it small ("

		if !strings.Contains(got, want) {
			t.Errorf("Context of %s =\n%s\nwant it to contain %q", name, got, want)
		}
	}
}

func TestScopes(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	if _, err := testApplyTeam(t, st, ctx, store.Team{Name: "core",
		Roles: []store.Role{{Name: "lead", Lead: true, Backend: "claude"}}}); err != nil {
		t.Fatal(err)
	}
	item := func(t *testing.T, title string) int64 {
		w, err := st.CreateWorkItem(ctx, store.WorkItem{ProjectID: f.ProjectID, Title: title})
		if err != nil {
			t.Fatal(err)
		}
		return w.ID
	}
	save := func(t *testing.T, c store.Space) store.Space {
		c.ProjectID, c.Name, c.Role = f.ProjectID, "chat", "lead"
		c, err := st.CreateSpace(ctx, c)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	tests := []struct {
		name string
		give func(t *testing.T) (store.Space, string)
		want []memory.Ref
	}{
		{
			name: "work item Space of a real team",
			give: func(t *testing.T) (store.Space, string) {
				w := item(t, "core item")
				return save(t, store.Space{WorkItemID: &w, Team: "core"}), strconv.FormatInt(w, 10)
			},
			want: []memory.Ref{
				{Kind: "work_item", ID: "W"}, {Kind: "project", ID: "1"},
				{Kind: "bot", ID: "1"},
			},
		},
		{
			name: "work item Space of the solo team",
			give: func(t *testing.T) (store.Space, string) {
				w := item(t, "solo item")
				return save(t, store.Space{WorkItemID: &w, Team: store.SoloTeam}), strconv.FormatInt(w, 10)
			},
			want: []memory.Ref{
				{Kind: "work_item", ID: "W"}, {Kind: "project", ID: "1"}, {Kind: "bot", ID: "1"},
			},
		},
		{
			name: "free Space of a real team",
			give: func(t *testing.T) (store.Space, string) {
				return save(t, store.Space{Team: "core"}), ""
			},
			want: []memory.Ref{
				{Kind: "space", ID: "S"}, {Kind: "project", ID: "1"},
				{Kind: "bot", ID: "1"},
			},
		},
		{
			name: "direct conversation",
			give: func(t *testing.T) (store.Space, string) {
				c, err := st.CreateBotSpace(ctx, store.Space{
					Kind: store.KindConversation, ProjectID: f.ProjectID, Name: "chat", Role: "lead",
				}, 1)
				if err != nil {
					t.Fatal(err)
				}
				return c, ""
			},
			want: []memory.Ref{
				{Kind: "conversation", ID: "S"}, {Kind: "project", ID: "1"}, {Kind: "bot", ID: "1"},
			},
		},
		{
			name: "Space that is not stored",
			give: func(t *testing.T) (store.Space, string) {
				return store.Space{ProjectID: 3}, ""
			},
			want: []memory.Ref{
				{Kind: "space", ID: "S"}, {Kind: "project", ID: "3"}, {Kind: "bot", ID: "1"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch, workItemID := tt.give(t)
			got, err := Scopes(ctx, st, ch, "lead")
			if err != nil {
				t.Fatal(err)
			}
			want := slices.Clone(tt.want)
			for i, r := range want {
				if r.ID == "W" {
					want[i].ID = workItemID
				}
				if r.ID == "S" {
					want[i].ID = strconv.FormatInt(ch.ID, 10)
				}
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("Scopes = %v, want %v", got, want)
			}
		})
	}
}

func TestFirstPrompt_SoloSpaceMatchesReefContext(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ms := testMemory(t, st)
	remember(t, ms, projectScope(f), memory.KindFact, "user", "Go 1.24")
	lead := store.Role{Name: "lead", Lead: true}
	solo := store.Team{Name: store.SoloTeam, Roles: []store.Role{lead}}

	got, err := firstPrompt(ctx, st, ms, f.ProjectID, &f.ID, 0, solo, lead)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "### Team") {
		t.Errorf("solo prompt =\n%s\nwant no Team part", got)
	}
	want, err := Context(ctx, st, ms, f.ID, "lead")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("solo prompt =\n%s\nwant what reef context prints:\n%s", got, want)
	}
}
