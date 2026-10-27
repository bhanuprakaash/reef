package space

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/bhanuprakaash/reef/memory"
	"github.com/bhanuprakaash/reef/store"
)

func TestFirstPrompt_Size(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	if _, err := st.PutBrief(ctx, store.Brief{
		WorkItemID: f.ID, Body: strings.Repeat("Brief line.\n", 10), Author: "user",
	}); err != nil {
		t.Fatal(err)
	}
	ms := testMemory(t, st)
	for i := 1; i <= 60; i++ {
		body := fmt.Sprintf("decision body number %d\nsecond line", i)
		remember(t, ms, workScope(f), memory.KindDecision, "user", body)
	}
	for _, scope := range []memory.Ref{
		workScope(f), projectScope(f),
		{Kind: memory.ScopeTeam, ID: "core"}, teammateScope("1"),
	} {
		for i := 1; i <= 30; i++ {
			body := fmt.Sprintf("%s item %d\nsecond line", scope.Kind, i)
			remember(t, ms, scope, memory.KindFinding, "user", body)
		}
	}
	for i := 1; i <= 15; i++ {
		c, err := st.CreateSpace(ctx, store.Space{
			ProjectID: f.ProjectID, WorkItemID: &f.ID, Name: fmt.Sprintf("space %d", i),
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := st.CreateMessage(ctx, store.Message{SpaceID: c.ID, Author: "user", Body: "hi"}); err != nil {
			t.Fatal(err)
		}
		closeSpace(t, st, c.ID)
	}
	for i := 1; i <= 40; i++ {
		w, err := st.CreateWorkItem(ctx, store.WorkItem{
			ProjectID: f.ProjectID, ParentID: &f.ID, Type: "task", Title: fmt.Sprintf("sub-item title %d", i),
		})
		if err != nil {
			t.Fatal(err)
		}
		status := map[int]string{1: "in_progress", 2: "in_progress", 3: "done", 4: "done", 5: "dropped"}[i]
		if status == "" {
			continue
		}
		if _, err := st.MoveWorkItem(ctx, w.ID, status, store.ByLead); err != nil {
			t.Fatal(err)
		}
	}
	lead := store.Role{Name: "lead", Lead: true}
	team := store.Team{Name: "core", Roles: []store.Role{lead, {Name: "developer"}}}

	got, err := firstPrompt(ctx, st, ms, f.ProjectID, &f.ID, 0, team, lead)
	if err != nil {
		t.Fatal(err)
	}
	last := -1
	for _, h := range []string{"## You\n", "## How to work\n", "## What we know\n"} {
		at := strings.Index(got, h)
		if n := strings.Count(got, h); n != 1 || at < last {
			t.Errorf("header %q appears %d times or out of order in\n%s", h, n, got)
		}
		last = at
	}
	for _, h := range []string{
		"### Brief\n", "### Decisions\n", "### Findings\n", "### Project\n", "### Team\n",
		"### Your items\n", "### In progress\n", "### On demand\n",
	} {
		if n := strings.Count(got, h); n != 1 {
			t.Errorf("header %q appears %d times in\n%s", h, n, got)
		}
	}
	if n := strings.Count(got, "\ndecision "); n != 60 {
		t.Errorf("decision lines = %d, want 60", n)
	}
	for _, scope := range []string{"work_item", "project", "team", "teammate"} {
		limit := map[string]int{"work_item": 5, "project": 10, "team": 5, "teammate": 5}[scope]
		want := fmt.Sprintf("%d of 30; recall(..., scope=%s)\n", limit, scope)
		if n := strings.Count(got, want); n != 1 {
			t.Errorf("count line %q appears %d times in\n%s", want, n, got)
		}
	}
	if n := strings.Count(got, "\nfinding "); n != 25 {
		t.Errorf("finding lines = %d, want 5+10+5+5", n)
	}
	if want := "### On demand\n37 open sub-items (list_sub_items). 15 earlier Spaces (search_sessions).\n"; !strings.HasSuffix(got, want) {
		t.Errorf("prompt does not end with %q:\n%s", want, got)
	}
	if want := "### In progress\nsub-item 2: sub-item title 1 (task)\nsub-item 3: sub-item title 2 (task)\n"; !strings.Contains(got, want) {
		t.Errorf("prompt does not have %q:\n%s", want, got)
	}
	if n := strings.Count(got, "\nsub-item "); n != 2 {
		t.Errorf("sub-item lines = %d, want only the 2 in progress", n)
	}
	if strings.Contains(got, "second line") {
		t.Error("a decision shows its second line")
	}
	if n := utf8.RuneCountInString(got); n >= 8000 {
		t.Errorf("prompt = %d characters, want under 8000", n)
	}
}

func TestFirstPrompt_OnDemandSingular(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	var spaces []store.Space
	for _, name := range []string{"spike", "build"} {
		c, err := st.CreateSpace(ctx, store.Space{ProjectID: f.ProjectID, WorkItemID: &f.ID, Name: name})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := st.CreateMessage(ctx, store.Message{SpaceID: c.ID, Author: "user", Body: "hi"}); err != nil {
			t.Fatal(err)
		}
		spaces = append(spaces, c)
		if name == "spike" {
			closeSpace(t, st, c.ID)
		}
	}

	got, err := firstPrompt(ctx, st, testMemory(t, st), f.ProjectID, &f.ID, spaces[1].ID, store.Team{}, store.Role{Name: "lead"})
	if err != nil {
		t.Fatal(err)
	}
	if want := "### On demand\n1 earlier Space (search_sessions).\n"; !strings.HasSuffix(got, want) {
		t.Errorf("prompt does not end with %q:\n%s", want, got)
	}
}
