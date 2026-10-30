package space

import (
	"context"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/memory"
	"github.com/bhanuprakaash/reef/store"
)

func TestSession_OneBotRunsWithProfileAccess(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	for _, name := range []string{"a", "b"} {
		if _, err := testApplyTeam(t, st, ctx, store.Team{Name: name, Roles: []store.Role{
			{Name: "lead", Lead: true, Backend: "claude"},
		}}); err != nil {
			t.Fatal(err)
		}
	}
	sam, err := st.CreateBot(ctx, store.Bot{
		Name: "sam", Backend: "claude", Description: "You test.", Write: true,
		Tools: []string{"Read", "Edit", "Write"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddMember(ctx, "a", sam.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddMember(ctx, "b", sam.ID, false); err != nil {
		t.Fatal(err)
	}
	ms := testMemory(t, st)
	item := remember(t, ms, botScope(strconv.FormatInt(sam.ID, 10)),
		memory.KindLesson, "sam", "Check the empty case first.")
	dataDir := t.TempDir()

	tests := []struct {
		team       string
		wantTools  []string
		wantDeny   []string
		checkpoint bool
	}{
		{"a", []string{"Read", "Edit", "Write", "Task", "mcp__reef__*"}, nil, true},
		{"b", []string{"Read", "Edit", "Write", "Task", "mcp__reef__*"}, nil, true},
	}
	for _, tt := range tests {
		ch, err := st.CreateSpace(ctx, store.Space{
			ProjectID: f.ProjectID, WorkItemID: &f.ID, Name: tt.team, Role: "lead",
			Team: tt.team,
		})
		if err != nil {
			t.Fatal(err)
		}
		closeSpace(t, st, ch.ID)
		ag := newRecordingAgent()
		s, err := Open(ctx, st, ms, dataDir, "/bin/reef", ch.ID, NewTrees(t.TempDir()),
			func(string) (agent.Agent, error) { return ag, nil })
		if err != nil {
			t.Fatal(err)
		}
		if err := s.RunAs(ctx, "sam", "go", nil, nil, discard); err != nil {
			t.Fatal(err)
		}
		_ = s.Close()

		if !reflect.DeepEqual(ag.cfg.Tools, tt.wantTools) ||
			!reflect.DeepEqual(ag.cfg.DisallowedTools, tt.wantDeny) {
			t.Errorf("team %s: tools %q, denied %q, want %q, %q",
				tt.team, ag.cfg.Tools, ag.cfg.DisallowedTools, tt.wantTools, tt.wantDeny)
		}
		turns, err := st.Turns(ctx, store.TurnFilter{SpaceID: &ch.ID})
		if err != nil || len(turns) != 1 {
			t.Fatalf("team %s: turns = %v, %v, want one", tt.team, turns, err)
		}
		if got := turns[0].Checkpoint != nil; got != tt.checkpoint {
			t.Errorf("team %s: checkpoint = %v, want %v", tt.team, got, tt.checkpoint)
		}
		want := "lesson " + strconv.FormatInt(item.ID, 10) + ": Check the empty case first."
		if !strings.Contains(turns[0].Prompt, "You test.") || !strings.Contains(turns[0].Prompt, want) {
			t.Errorf("team %s: prompt =\n%s\nwant it to hold the description and %q", tt.team, turns[0].Prompt, want)
		}
	}
}
