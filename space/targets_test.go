package space

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/store"
)

// writingAgent writes one file into each folder of files, relative to its working folder, when it
// gets a prompt, as an agent editing two repositories would.
type writingAgent struct {
	*recordingAgent
	files map[string]string
}

func (a *writingAgent) Send(ctx context.Context, prompt string) error {
	for path, body := range a.files {
		if err := os.WriteFile(filepath.Join(a.cfg.Cwd, path), []byte(body), 0o644); err != nil {
			return err
		}
	}
	return a.recordingAgent.Send(ctx, prompt)
}

func TestSession_OneAssignmentChangesAndCheckpointsEveryTarget(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	infra := t.TempDir()
	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"-c", "user.name=t", "-c", "user.email=t@t", "commit", "--allow-empty", "-m", "first"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", infra}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	p2, err := st.CreateProject(ctx, store.Project{Name: "infra", Path: infra})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddWorkItemTarget(ctx, f.ID, p2.ID, "user", nil); err != nil {
		t.Fatal(err)
	}
	r := newTreeRig(t, st)
	ch, err := st.CreateSpace(ctx, store.Space{
		ProjectID: f.ProjectID, WorkItemID: &f.ID, Name: "a", Role: "lead", Team: "crew",
	})
	if err != nil {
		t.Fatal(err)
	}
	var ag, first *writingAgent
	s, err := Open(ctx, st, testMemory(t, st), r.dir, "/bin/reef", ch.ID, r.trees, func(string) (agent.Agent, error) {
		ag = &writingAgent{recordingAgent: newRecordingAgent(), files: map[string]string{
			"1-shop/api.txt": "api", "2-infra/deploy.txt": "deploy",
		}}
		if first == nil {
			first = ag
		}
		return ag, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	if err := s.RunAs(ctx, "developer", "go", nil, nil, discard); err != nil {
		t.Fatal(err)
	}
	if err := s.RunAs(ctx, "developer", "again", nil, nil, discard); err != nil {
		t.Fatal(err)
	}

	root := WorkRoot(r.dir, "1")
	if ag.cfg.Cwd != root {
		t.Errorf("agent Cwd = %q, want the assignment root %q", ag.cfg.Cwd, root)
	}
	for dir, want := range map[string]string{"1-shop": "?? api.txt", "2-infra": "?? deploy.txt"} {
		out, err := exec.Command("git", "-C", filepath.Join(root, dir), "status", "--porcelain").Output()
		if err != nil || strings.TrimSpace(string(out)) != want {
			t.Errorf("status of %s = %q, %v, want %q", dir, out, err, want)
		}
	}
	for _, want := range []string{
		"- shop: 1-shop/ on branch work-item/1",
		"- infra: 2-infra/ on branch work-item/1",
	} {
		if !strings.Contains(first.prompts[0], want) {
			t.Errorf("first prompt = %q, want it to list %q", first.prompts[0], want)
		}
	}
	turns, err := st.Turns(ctx, store.TurnFilter{SpaceID: &ch.ID})
	if err != nil || len(turns) != 2 {
		t.Fatalf("turns = %+v, %v, want two", turns, err)
	}
	if got := turns[0].Checkpoints; len(got) != 2 || got[0].ProjectID != f.ProjectID || got[1].ProjectID != p2.ID {
		t.Errorf("checkpoints of turn 1 = %+v, want one for each of projects %d and %d", got, f.ProjectID, p2.ID)
	}
	cps := turns[1].Checkpoints
	if len(cps) != 2 {
		t.Fatalf("checkpoints of turn 2 = %+v, want two", cps)
	}
	for i, file := range []string{"1-shop/api.txt", "2-infra/deploy.txt"} {
		dir, name := filepath.Join(root, filepath.Dir(file)), filepath.Base(file)
		out, err := exec.Command("git", "-C", dir, "show", cps[i].Ref+":"+name).Output()
		if err != nil || len(out) == 0 {
			t.Errorf("checkpoint of turn 2 in %s has no %s: %q, %v", dir, name, out, err)
		}
	}
}

func TestEnsureWorkspace_PersonalChatCannotTakeAnAssignmentTree(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	dir := t.TempDir()
	ws, err := EnsureWorkspaces(ctx, st, NewTrees(t.TempDir()), dir, f)
	if err != nil || len(ws) != 1 || ws[0].State != store.WorkspaceReady {
		t.Fatalf("workspaces = %+v, %v, want one ready", ws, err)
	}
	chat := sessionSpace(t, st, f, true)

	got, key, err := WorkDir(ctx, st, dir, chat.ID)
	if err != nil || key != "" || strings.HasPrefix(got, WorkRoot(dir, "1")) {
		t.Errorf("WorkDir of a personal chat = %q, %q, %v, want its own folder outside the assignment", got, key, err)
	}
	_, err = st.EnsureWorkspace(ctx, store.Workspace{
		ProjectID: f.ProjectID, SpaceID: &chat.ID, Name: "tidy", Dir: ws[0].Dir, Branch: "fix/tidy",
	})
	if !errors.Is(err, store.ErrConflict) {
		t.Errorf("EnsureWorkspace of the assignment's folder for a chat = %v, want ErrConflict", err)
	}
}
