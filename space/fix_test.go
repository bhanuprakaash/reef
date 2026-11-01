package space

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/store"
	"github.com/bhanuprakaash/reef/worktree"
)

func TestSession_ConversationWriteTurnCheckpointsItsFixes(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	if _, err := testApplyTeam(t, st, ctx, store.Team{Name: "crew", Roles: []store.Role{
		{Name: "developer", Lead: true, Write: true, Backend: "claude"},
	}}); err != nil {
		t.Fatal(err)
	}
	ch, err := st.CreateSpace(ctx, store.Space{ProjectID: f.ProjectID, Name: "chat", Role: "developer", Team: "crew"})
	if err != nil {
		t.Fatal(err)
	}
	p, err := st.Project(ctx, f.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	trees := NewTrees(t.TempDir())
	w, err := EnsureFixWorkspace(ctx, st, trees, dir, ch.ID, "login", p)
	if err != nil || w.State != store.WorkspaceReady || w.Branch != "fix/1-login" ||
		w.Dir != filepath.Join(ChatDir(dir, ch.ID), "login-shop") {
		t.Fatalf("fix = %+v, %v; want ready on fix/1-login under the chat folder", w, err)
	}
	if err := os.WriteFile(filepath.Join(w.Dir, "wip.txt"), []byte("draft"), 0o644); err != nil {
		t.Fatal(err)
	}

	s, err := Open(ctx, st, testMemory(t, st), dir, "/bin/reef", ch.ID, trees, func(string) (agent.Agent, error) {
		return newRecordingAgent(), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if err := s.RunAs(ctx, "developer", "go on", nil, nil, discard); err != nil {
		t.Fatal(err)
	}

	turns, err := st.Turns(ctx, store.TurnFilter{SpaceID: &ch.ID})
	if err != nil || len(turns) != 1 || len(turns[0].Checkpoints) != 1 {
		t.Fatalf("turns = %+v, %v; want one turn with one checkpoint", turns, err)
	}
	out, err := exec.Command("git", "-C", w.Dir, "show", turns[0].Checkpoints[0].Ref+":wip.txt").Output()
	if err != nil || string(out) != "draft" {
		t.Errorf("checkpoint wip.txt = %q, %v; want draft", out, err)
	}

	var b strings.Builder
	if err := fixLines(ctx, st, &b, ch.ID); err != nil {
		t.Fatal(err)
	}
	if got := b.String(); !strings.Contains(got, "- login in shop: login-shop/ on branch fix/1-login (ready)") ||
		!strings.Contains(got, "ask the user") {
		t.Errorf("context lines = %q; want the open fix and the ask rule", got)
	}
}

func TestSession_TwoFixesOfOneProjectEachRestoreFromTheirOwnCheckpoint(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	if _, err := testApplyTeam(t, st, ctx, store.Team{Name: "crew", Roles: []store.Role{
		{Name: "developer", Lead: true, Write: true, Backend: "claude"},
	}}); err != nil {
		t.Fatal(err)
	}
	ch, err := st.CreateSpace(ctx, store.Space{ProjectID: f.ProjectID, Name: "chat", Role: "developer", Team: "crew"})
	if err != nil {
		t.Fatal(err)
	}
	p, err := st.Project(ctx, f.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	trees := NewTrees(t.TempDir())
	var fixes []store.Workspace
	for _, name := range []string{"login", "logout"} {
		w, err := EnsureFixWorkspace(ctx, st, trees, dir, ch.ID, name, p)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(w.Dir, "wip.txt"), []byte("draft of "+name), 0o644); err != nil {
			t.Fatal(err)
		}
		fixes = append(fixes, w)
	}
	s, err := Open(ctx, st, testMemory(t, st), dir, "/bin/reef", ch.ID, trees, func(string) (agent.Agent, error) {
		return newRecordingAgent(), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if err := s.RunAs(ctx, "developer", "go on", nil, nil, discard); err != nil {
		t.Fatal(err)
	}

	turns, err := st.Turns(ctx, store.TurnFilter{SpaceID: &ch.ID})
	if err != nil || len(turns) != 1 || len(turns[0].Checkpoints) != 2 {
		t.Fatalf("turns = %+v, %v; want one turn with a checkpoint per fix", turns, err)
	}
	if a, b := turns[0].Checkpoints[0].Ref, turns[0].Checkpoints[1].Ref; a == b {
		t.Fatalf("both fixes checkpointed at %s", a)
	}
	for i, c := range turns[0].Checkpoints {
		w := fixes[i]
		if err := os.WriteFile(filepath.Join(w.Dir, "wip.txt"), []byte("changed"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := worktree.Restore(ctx, w.Dir, c.Ref); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(w.Dir, "wip.txt"))
		if want := "draft of " + w.Name; err != nil || string(got) != want {
			t.Errorf("%s wip.txt after restore = %q, %v; want %q", w.Name, got, err, want)
		}
	}
}
