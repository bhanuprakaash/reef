package main

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/bhanuprakaash/reef/store"
)

func TestCLI_Wrap(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if cwd, err = store.ResolvePath(cwd); err != nil {
		t.Fatal(err)
	}
	p, err := st.CreateProject(ctx, store.Project{Name: "shop", Path: cwd})
	if err != nil {
		t.Fatal(err)
	}
	for _, title := range []string{"login", "billing"} {
		f, err := st.CreateWorkItem(ctx, store.WorkItem{ProjectID: p.ID, Title: title})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := st.CreateSpace(ctx, store.Space{ProjectID: p.ID, WorkItemID: &f.ID, Name: "chat"}); err != nil {
			t.Fatal(err)
		}
	}
	other, err := st.CreateProject(ctx, store.Project{Name: "other", Path: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	f, err := st.CreateWorkItem(ctx, store.WorkItem{ProjectID: other.ID, Title: "outside"})
	if err != nil {
		t.Fatal(err)
	}
	ch, err := st.CreateSpace(ctx, store.Space{ProjectID: other.ID, WorkItemID: &f.ID, Name: "chat"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB().ExecContext(ctx, "UPDATE space SET closed_at = ? WHERE id = ?",
		time.Now().UTC().Format(time.RFC3339Nano), ch.ID); err != nil {
		t.Fatal(err)
	}
	brief, err := st.CreateSpace(ctx, store.Space{ProjectID: other.ID, WorkItemID: &f.ID, Name: "plan"})
	if err != nil {
		t.Fatal(err)
	}
	search, err := st.CreateWorkItem(ctx, store.WorkItem{ProjectID: p.ID, Title: "search"})
	if err != nil {
		t.Fatal(err)
	}
	sch, err := st.CreateSpace(ctx, store.Space{ProjectID: p.ID, WorkItemID: &search.ID, Name: "chat"})
	if err != nil {
		t.Fatal(err)
	}
	day := time.Now().UTC().Format(time.DateOnly)
	for _, prop := range []store.Proposal{
		{SpaceID: 1, Role: "architect", Brief: "sign in with sessions",
			Items: []store.ProposedItem{
				{Kind: "decision", Body: "cookie sessions", Why: "simple", Scope: "work_item"},
				{Kind: "lesson", Body: "24h expiry", Scope: "teammate"},
			}},
		{SpaceID: 2, Role: "reviewer", Brief: "bill monthly", Retire: []int64{7},
			Items: []store.ProposedItem{{Kind: "fact", Body: "bills on the 1st", Scope: "project", Replaces: []int64{8}}}},
		{SpaceID: ch.ID, Role: "developer", Brief: "outside project"},
		{SpaceID: brief.ID, Role: "lead", Kind: store.ProposalBrief, Brief: "outside plan"},
		{SpaceID: sch.ID, Role: "lead", Kind: store.ProposalBrief, Brief: "search plan"},
	} {
		if _, err := st.CreateProposal(ctx, prop); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	steps := []struct {
		name string
		give []string
		want string
	}{
		{"list all", []string{"wrap", "list"},
			"1  1  1  architect  wrap  " + day + "\n2  2  2  reviewer  wrap  " + day +
				"\n5  4  5  lead  brief  " + day + "\n"},
		{"list work item", []string{"wrap", "list", "2"}, "2  2  2  reviewer  wrap  " + day + "\n"},
		{"show", []string{"wrap", "show", "1"},
			"proposal 1 from architect\nstatus: pending\n\nbrief:\nsign in with sessions\n\n" +
				"items:\n- decision (work_item): cookie sessions (simple)\n- lesson (teammate): 24h expiry\n"},
		{"show retire", []string{"wrap", "show", "2"},
			"proposal 2 from reviewer\nstatus: pending\n\nbrief:\nbill monthly\n\n" +
				"items:\n- fact (project): bills on the 1st [replaces [8]]\n\nretire memory items: [7]\n"},
		{"approve", []string{"wrap", "approve", "1"}, "approved: brief and 2 items saved\n"},
		{"show decided", []string{"wrap", "show", "1"},
			"proposal 1 from architect\nstatus: approved\n\nbrief:\nsign in with sessions\n\n" +
				"items:\n- decision (work_item): cookie sessions (simple)\n- lesson (teammate): 24h expiry\n"},
		{"reject", []string{"wrap", "reject", "2"}, "rejected: nothing changed\n"},
		{"list after deciding", []string{"wrap", "list"}, "5  4  5  lead  brief  " + day + "\n"},
		{"show brief", []string{"wrap", "show", "4"},
			"brief update 4 from lead\nstatus: pending\n\nbrief:\noutside plan\n"},
		{"approve brief", []string{"wrap", "approve", "4"}, "approved: brief saved\n"},
	}
	for _, tt := range steps {
		if got := mustReef(t, dir, tt.give...); got != tt.want {
			t.Errorf("%s = %q, want %q", tt.name, got, tt.want)
		}
	}

	if got, want := mustReef(t, dir, "brief", "1"), "sign in with sessions\n"; got != want {
		t.Errorf("brief = %q, want %q", got, want)
	}
	for _, args := range [][]string{{"wrap", "approve", "1"}, {"wrap", "reject", "1"}} {
		_, err := reef(t, dir, args...)
		if want := "invalid: proposal 1 is already approved"; err == nil || err.Error() != want {
			t.Errorf("reef %v error = %v, want %q", args, err, want)
		}
	}
	if _, err := reef(t, dir, "wrap", "approve", "2"); err == nil ||
		err.Error() != "invalid: proposal 2 is already rejected" {
		t.Errorf("approve rejected error = %v", err)
	}
}

func TestCLI_WrapApprovePrintsAWarningForEachLookalike(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	p, err := st.CreateProject(ctx, store.Project{Name: "shop", Path: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	f, err := st.CreateWorkItem(ctx, store.WorkItem{ProjectID: p.ID, Title: "login"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateSpace(ctx, store.Space{ProjectID: p.ID, WorkItemID: &f.ID, Name: "chat"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateProposal(ctx, store.Proposal{
		SpaceID: 1, Role: "architect", Brief: "sign in",
		Items: []store.ProposedItem{{Kind: "decision", Body: "use sessions for login.", Scope: "work_item"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	mustReef(t, dir, "memory", "add", "decision", "use sessions for login", "--scope", "work_item:1")

	got := mustReef(t, dir, "wrap", "approve", "1")
	if want := "approved: brief and 1 items saved\nwarning: memory 2 looks like memory 1\n"; got != want {
		t.Errorf("wrap approve = %q, want %q", got, want)
	}
}
