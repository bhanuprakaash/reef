package space

import (
	"context"
	"fmt"
	"reflect"
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
		remember(t, ms, teammateScope("developer"), memory.KindLesson, "developer",
			fmt.Sprintf("developer lesson %d", i))
		remember(t, ms, teammateScope("tester"), memory.KindLesson, "tester",
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
		"5 of 6; recall(..., scope=teammate)\n",
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

func TestFirstPrompt_TeamItemsOnlyForATeam(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ms := testMemory(t, st)
	w := memory.Writer{
		Author: "user", Role: "user", Scope: memory.Ref{Kind: memory.ScopeTeam, ID: "core"},
	}
	if _, err := ms.Remember(ctx, w, memory.Note{Kind: memory.KindConvention, Body: "review before merge"}); err != nil {
		t.Fatal(err)
	}
	lead := store.Role{Name: "lead", Lead: true}
	team := store.Team{Name: "core", Roles: []store.Role{lead, {Name: "developer"}}}

	got, err := firstPrompt(ctx, st, ms, f.ProjectID, &f.ID, 0, team, lead)
	if err != nil {
		t.Fatal(err)
	}
	if want := "### Team\nconvention 1: review before merge ("; !strings.Contains(got, want) {
		t.Errorf("team prompt =\n%s\nwant it to contain %q", got, want)
	}
	got, err = firstPrompt(ctx, st, ms, f.ProjectID, &f.ID, 0, store.Team{}, lead)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "### Team") {
		t.Errorf("prompt with no team =\n%s\nwant no Team part", got)
	}
}

func TestFirstPrompt_QuickSessionHasNoFeatureParts(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ms := testMemory(t, st)
	remember(t, ms, workScope(f), memory.KindDecision, "user", "a feature decision")
	remember(t, ms, projectScope(f), memory.KindFact, "user", "a project fact")
	remember(t, ms, teammateScope("lead"), memory.KindLesson, "lead", "a lead lesson")

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

func TestOpen_FirstPromptIsWhatReefContextPrints(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ms := testMemory(t, st)
	remember(t, ms, workScope(f), memory.KindDecision, "user", "use sessions")
	remember(t, ms, workScope(f), memory.KindFinding, "tester", "login is slow")
	remember(t, ms, projectScope(f), memory.KindFact, "user", "Go 1.24")
	remember(t, ms, teammateScope("architect"), memory.KindLesson, "architect", "keep it small")
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

func TestScopes(t *testing.T) {
	st, _ := openStore(t)
	if _, err := st.ApplyTeam(context.Background(), store.Team{Name: "core",
		Roles: []store.Role{{Name: "lead", Lead: true, Backend: "claude"}}}); err != nil {
		t.Fatal(err)
	}
	feature := int64(7)
	tests := []struct {
		name string
		give store.Space
		want []memory.Ref
	}{
		{
			name: "feature Space of a real team",
			give: store.Space{ProjectID: 3, WorkItemID: &feature, Team: "core"},
			want: []memory.Ref{
				{Kind: "work_item", ID: "7"}, {Kind: "project", ID: "3"},
				{Kind: "team", ID: "core"}, {Kind: "teammate", ID: "2"},
			},
		},
		{
			name: "feature Space of the solo team",
			give: store.Space{ProjectID: 3, WorkItemID: &feature, Team: store.SoloTeam},
			want: []memory.Ref{
				{Kind: "work_item", ID: "7"}, {Kind: "project", ID: "3"}, {Kind: "teammate", ID: "1"},
			},
		},
		{
			name: "quick session of a real team",
			give: store.Space{ProjectID: 3, Team: "core"},
			want: []memory.Ref{
				{Kind: "project", ID: "3"}, {Kind: "team", ID: "core"}, {Kind: "teammate", ID: "2"},
			},
		},
		{
			name: "space with no team",
			give: store.Space{ProjectID: 3},
			want: []memory.Ref{{Kind: "project", ID: "3"}, {Kind: "teammate", ID: "lead"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Scopes(context.Background(), st, tt.give, "lead")
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Scopes = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFirstPrompt_SoloSpaceHasNoTeamPartAndMatchesReefContext(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ms := testMemory(t, st)
	remember(t, ms, projectScope(f), memory.KindFact, "user", "Go 1.24")
	if _, err := ms.Remember(ctx, memory.Writer{
		Author: "user", Role: "user", Scope: memory.Ref{Kind: memory.ScopeTeam, ID: store.SoloTeam},
	}, memory.Note{Kind: memory.KindLesson, Body: "kept on the solo shelf"}); err != nil {
		t.Fatal(err)
	}
	lead := store.Role{Name: "lead", Lead: true}
	solo := store.Team{Name: store.SoloTeam, Roles: []store.Role{lead}}

	got, err := firstPrompt(ctx, st, ms, f.ProjectID, &f.ID, 0, solo, lead)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "### Team") || strings.Contains(got, "kept on the solo shelf") {
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
