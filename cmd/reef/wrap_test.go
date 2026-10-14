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
	for _, slug := range []string{"login", "billing"} {
		f, err := st.CreateFeature(ctx, store.Feature{ProjectID: p.ID, Slug: slug, Name: slug})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := st.CreateChannel(ctx, store.Channel{ProjectID: p.ID, FeatureID: &f.ID, Name: "chat"}); err != nil {
			t.Fatal(err)
		}
	}
	other, err := st.CreateProject(ctx, store.Project{Name: "other", Path: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	f, err := st.CreateFeature(ctx, store.Feature{ProjectID: other.ID, Slug: "outside", Name: "outside"})
	if err != nil {
		t.Fatal(err)
	}
	ch, err := st.CreateChannel(ctx, store.Channel{ProjectID: other.ID, FeatureID: &f.ID, Name: "chat"})
	if err != nil {
		t.Fatal(err)
	}
	day := time.Now().UTC().Format(time.DateOnly)
	for _, prop := range []store.Proposal{
		{ChannelID: 1, Role: "architect", Brief: "sign in with sessions", Notes: "n1",
			Decisions: []store.ProposedDecision{{Body: "cookie sessions", Why: "simple"}, {Body: "24h expiry"}}},
		{ChannelID: 2, Role: "reviewer", Brief: "bill monthly", Notes: "n2"},
		{ChannelID: ch.ID, Role: "developer", Brief: "outside project"},
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
			"1  login  1  architect  " + day + "\n2  billing  2  reviewer  " + day + "\n"},
		{"list feature", []string{"wrap", "list", "billing"}, "2  billing  2  reviewer  " + day + "\n"},
		{"show", []string{"wrap", "show", "1"},
			"proposal 1 from architect\nstatus: pending\n\nbrief:\nsign in with sessions\n\nnotes:\nn1\n\n" +
				"decisions:\n- cookie sessions (simple)\n- 24h expiry\n"},
		{"approve", []string{"wrap", "approve", "1"}, "approved: brief, notes and 2 decisions saved\n"},
		{"show decided", []string{"wrap", "show", "1"},
			"proposal 1 from architect\nstatus: approved\n\nbrief:\nsign in with sessions\n\nnotes:\nn1\n\n" +
				"decisions:\n- cookie sessions (simple)\n- 24h expiry\n"},
		{"reject", []string{"wrap", "reject", "2"}, "rejected: nothing changed\n"},
		{"list empty", []string{"wrap", "list"}, ""},
	}
	for _, tt := range steps {
		if got := mustReef(t, dir, tt.give...); got != tt.want {
			t.Errorf("%s = %q, want %q", tt.name, got, tt.want)
		}
	}

	if got, want := mustReef(t, dir, "brief", "login"), "sign in with sessions\n"; got != want {
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
