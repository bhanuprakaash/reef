package channel

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/store"
)

func TestSession_CheckpointsWriteTurnsOnly(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	if _, err := st.ApplyTeam(ctx, store.Team{Name: "crew", Roles: []store.Role{
		{Name: "lead", Lead: true, Backend: "claude"},
		{Name: "developer", Write: true, Backend: "claude"},
		{Name: "tester", Backend: "claude"},
	}}); err != nil {
		t.Fatal(err)
	}
	ch, err := st.CreateChannel(ctx, store.Channel{
		ProjectID: f.ProjectID, FeatureID: &f.ID, Name: "chat", Role: "lead", Backend: "claude",
		Team: "crew",
	})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	s, err := open(ctx, st, dir, "/bin/reef", ch.ID, func(string) (agent.Agent, error) {
		return newRecordingAgent(), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	if err := s.RunAs(ctx, "tester", "look", nil, discard); err != nil {
		t.Fatal(err)
	}
	tree := filepath.Join(dir, "worktrees", "1", "login")
	if err := os.WriteFile(filepath.Join(tree, "note.txt"), []byte("draft"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.RunAs(ctx, "developer", "write", nil, discard); err != nil {
		t.Fatal(err)
	}

	turns, err := st.Turns(ctx, store.TurnFilter{ChannelID: &ch.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 2 || turns[0].Role != "tester" || turns[0].Checkpoint != nil ||
		turns[1].Role != "developer" || turns[1].Checkpoint == nil ||
		*turns[1].Checkpoint != "refs/reef/checkpoints/2" {
		t.Fatalf("turns = %+v, want tester with no checkpoint, developer with refs/reef/checkpoints/2", turns)
	}
	out, err := exec.Command("git", "-C", tree, "show", "refs/reef/checkpoints/2:note.txt").Output()
	if err != nil || string(out) != "draft" {
		t.Errorf("checkpoint note.txt = %q, %v, want draft", out, err)
	}
}

func TestRunner_FailsTurnWhenCheckpointFails(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch := sessionChannel(t, st, f, true)
	ag := newRecordingAgent()
	r := NewRunner(st, ag, ch.ID, "architect", "claude", "")
	r.CheckpointDir = t.TempDir()

	if err := r.Run(ctx, "go", nil, discard); err == nil {
		t.Fatal("Run in a folder that is not a git checkout succeeded, want an error")
	}
	turns, err := st.Turns(ctx, store.TurnFilter{ChannelID: &ch.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 1 || turns[0].Status != "failed" || turns[0].Checkpoint != nil ||
		!strings.Contains(turns[0].Error, "not a git repository") {
		t.Errorf("turns = %+v, want one failed turn with no checkpoint", turns)
	}
	if len(ag.prompts) != 0 {
		t.Errorf("prompts = %q, want none", ag.prompts)
	}
}

func TestRunner_RunsWithoutCheckpointInRepoWithNoCommit(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	ch := sessionChannel(t, st, f, true)
	ag := newRecordingAgent()
	r := NewRunner(st, ag, ch.ID, "architect", "claude", "")
	r.CheckpointDir = t.TempDir()
	if out, err := exec.Command("git", "-C", r.CheckpointDir, "init", "-b", "main").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}

	if err := r.Run(ctx, "go", nil, discard); err != nil {
		t.Fatal(err)
	}
	turns, err := st.Turns(ctx, store.TurnFilter{ChannelID: &ch.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 1 || turns[0].Status != "done" || turns[0].Checkpoint != nil {
		t.Errorf("turns = %+v, want one done turn with no checkpoint", turns)
	}
	if len(ag.prompts) != 1 {
		t.Errorf("prompts = %q, want one", ag.prompts)
	}
}
