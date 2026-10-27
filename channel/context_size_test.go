package channel

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/bhanuprakaash/reef/memory"
	"github.com/bhanuprakaash/reef/store"
)

func TestContext_OnlyDoingTasksAreListed(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	for i := 1; i <= 24; i++ {
		task := store.Task{ProjectID: f.ProjectID, FeatureID: &f.ID, Title: fmt.Sprintf("job %d", i)}
		switch {
		case i <= 3:
			task.Status = "doing"
		case i == 24:
			task.Status = "done"
		}
		if _, err := st.CreateTask(ctx, task); err != nil {
			t.Fatal(err)
		}
	}
	c, err := st.CreateChannel(ctx, store.Channel{ProjectID: f.ProjectID, FeatureID: &f.ID, Name: "spike"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateMessage(ctx, store.Message{ChannelID: c.ID, Author: "user", Body: "hi"}); err != nil {
		t.Fatal(err)
	}

	got, err := Context(ctx, st, testMemory(t, st), f.ID, "developer")
	if err != nil {
		t.Fatal(err)
	}
	want := `## What we know
### Doing
task 1: job 1
task 2: job 2
task 3: job 3

### On demand
23 open tasks (list_tasks). 1 earlier channel (search_sessions).
`
	if !strings.HasSuffix(got, want) {
		t.Errorf("Context =\n%s\nwant it to end with\n%s", got, want)
	}
}

func TestFirstPrompt_Size(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	if _, err := st.PutBrief(ctx, store.Brief{
		FeatureID: f.ID, Body: strings.Repeat("Brief line.\n", 10), Author: "user",
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
		{Kind: memory.ScopeTeam, ID: "core"}, teammateScope("lead"),
	} {
		for i := 1; i <= 30; i++ {
			body := fmt.Sprintf("%s item %d\nsecond line", scope.Kind, i)
			remember(t, ms, scope, memory.KindFinding, "user", body)
		}
	}
	for i := 1; i <= 40; i++ {
		task := store.Task{
			ProjectID: f.ProjectID, FeatureID: &f.ID, Title: fmt.Sprintf("task title %d", i),
			Status: []string{"todo", "doing", "done"}[i%3],
		}
		if _, err := st.CreateTask(ctx, task); err != nil {
			t.Fatal(err)
		}
	}
	for i := 1; i <= 15; i++ {
		c, err := st.CreateChannel(ctx, store.Channel{
			ProjectID: f.ProjectID, FeatureID: &f.ID, Name: fmt.Sprintf("channel %d", i),
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := st.CreateMessage(ctx, store.Message{ChannelID: c.ID, Author: "user", Body: "hi"}); err != nil {
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
		"### Your items\n", "### Doing\n", "### On demand\n",
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
	if n := strings.Count(got, "\ntask "); n != 14 {
		t.Errorf("task lines = %d, want only the 14 doing", n)
	}
	if want := "27 open tasks (list_tasks). 15 earlier channels (search_sessions).\n"; !strings.HasSuffix(got, want) {
		t.Errorf("prompt does not end with %q:\n%s", want, got)
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
	if _, err := st.CreateTask(ctx, store.Task{ProjectID: f.ProjectID, FeatureID: &f.ID, Title: "job"}); err != nil {
		t.Fatal(err)
	}
	var channels []store.Channel
	for _, name := range []string{"spike", "build"} {
		c, err := st.CreateChannel(ctx, store.Channel{ProjectID: f.ProjectID, FeatureID: &f.ID, Name: name})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := st.CreateMessage(ctx, store.Message{ChannelID: c.ID, Author: "user", Body: "hi"}); err != nil {
			t.Fatal(err)
		}
		channels = append(channels, c)
	}

	got, err := firstPrompt(ctx, st, testMemory(t, st), f.ProjectID, &f.ID, channels[1].ID, store.Team{}, store.Role{Name: "lead"})
	if err != nil {
		t.Fatal(err)
	}
	if want := "### On demand\n1 open task (list_tasks). 1 earlier channel (search_sessions).\n"; !strings.HasSuffix(got, want) {
		t.Errorf("prompt does not end with %q:\n%s", want, got)
	}
}
