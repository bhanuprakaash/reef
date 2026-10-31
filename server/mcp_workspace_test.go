package server

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/store"
)

func TestOpenWorkspace_QuickFixesOfAConversation(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	st, err := openStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	repoA, _ := originRepo(t, "develop")
	repoB, _ := originRepo(t, "develop")
	for i, name := range []string{"App", "Lib"} {
		p, err := st.CreateProject(ctx, store.Project{Name: name, Path: []string{repoA, repoB}[i]})
		if err != nil {
			t.Fatal(err)
		}
		p.BaseBranch = "develop"
		if _, err := st.UpdateProject(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	bot := *testServerBot(t, st, "architect")
	var toks []store.Token
	for _, name := range []string{"one", "two"} {
		sp, err := st.CreateBotSpace(ctx, store.Space{Name: name}, bot)
		if err != nil {
			t.Fatal(err)
		}
		tok, err := st.CreateToken(ctx, sp.ID, bot)
		if err != nil {
			t.Fatal(err)
		}
		toks = append(toks, tok)
	}
	call := func(i int, args map[string]any) (openWorkspaceOut, string, bool) {
		t.Helper()
		text, isErr := callToolResult(t, st, toks[i], dataDir)("open_workspace", args)
		var out openWorkspaceOut
		if !isErr {
			if err := json.Unmarshal([]byte(text), &out); err != nil {
				t.Fatal(err)
			}
		}
		return out, text, isErr
	}

	first, _, isErr := call(0, map[string]any{"project": 1, "fix": "login-bug"})
	if isErr || first.State != "ready" || first.BaseSource != "origin" || first.Branch != "fix/1-login-bug" {
		t.Fatalf("new fix = %+v, error %v; want ready from origin on fix/1-login-bug", first, isErr)
	}
	if want := filepath.Join(dataDir, "chats", "1", "login-bug-app"); first.Dir != want {
		t.Errorf("dir = %s, want %s", first.Dir, want)
	}
	if got := gitIn(t, first.Dir, "log", "-1", "--format=%s"); got != "develop" {
		t.Errorf("fix starts at %q, want the develop commit", got)
	}

	again, _, isErr := call(0, map[string]any{"project": 1, "fix": "login-bug"})
	if isErr || again.Dir != first.Dir {
		t.Errorf("same fix = %+v, error %v; want the same folder", again, isErr)
	}

	_, text, isErr := call(0, map[string]any{"project": 1})
	if !isErr || !strings.Contains(text, "login-bug") || !strings.Contains(text, "Ask the user") {
		t.Errorf("no fix with one open = %q, error %v; want a question naming login-bug", text, isErr)
	}

	second, _, isErr := call(0, map[string]any{"project": 1, "fix": "typo"})
	if isErr || second.Dir == first.Dir || second.Branch != "fix/1-typo" {
		t.Errorf("independent fix = %+v, error %v; want its own folder and fix/1-typo", second, isErr)
	}
	other, _, isErr := call(0, map[string]any{"project": 2})
	if isErr || other.Fix != "fix-1" {
		t.Errorf("first fix of another Project = %+v, error %v; want fix-1", other, isErr)
	}

	writeIn(t, first.Dir, "edit.txt", "mine")
	foreign, _, isErr := call(1, map[string]any{"project": 1, "fix": "login-bug"})
	if isErr || foreign.Dir == first.Dir || foreign.Branch != "fix/2-login-bug" {
		t.Errorf("other Space naming the fix = %+v, error %v; want its own folder and fix/2-login-bug", foreign, isErr)
	}
	if got := gitIn(t, first.Dir, "status", "--porcelain"); got != "?? edit.txt" {
		t.Errorf("first Space's tree = %q, want only its own edit", got)
	}

	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := openStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	rows, err := reopened.SpaceWorkspaces(ctx, 1)
	if err != nil || len(rows) != 3 || rows[0].Dir != first.Dir || rows[0].State != "ready" {
		t.Errorf("after restart = %+v, %v; want the 3 fixes of Space 1", rows, err)
	}
	items, err := reopened.WorkItems(ctx, store.WorkItemFilter{})
	if err != nil || len(items) != 0 {
		t.Errorf("work items = %d, %v; want none", len(items), err)
	}
}
