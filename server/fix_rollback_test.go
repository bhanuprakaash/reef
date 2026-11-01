package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/space"
	"github.com/bhanuprakaash/reef/store"
	"github.com/bhanuprakaash/reef/worktree"
)

// twoFixes is a conversation Space with two quick fixes of one project, login and logout, a
// turn that checkpointed both, and each fix edited after it.
func twoFixes(t *testing.T) (url string, st *store.Store, spaceID int64, fixes []store.Workspace) {
	t.Helper()
	ctx := context.Background()
	ts, st := newEmbedServer(t, nil)
	repo := t.TempDir()
	gitIn(t, repo, "init", "-b", "main")
	writeIn(t, repo, "a.txt", "a")
	gitIn(t, repo, "add", ".")
	gitIn(t, repo, "commit", "-m", "first")
	p, err := st.CreateProject(ctx, store.Project{Name: "app", Path: repo})
	if err != nil {
		t.Fatal(err)
	}
	sp, err := st.CreateBotSpace(ctx, store.Space{Name: "chat"}, *testServerBot(t, st, "architect"))
	if err != nil {
		t.Fatal(err)
	}
	turn, err := st.CreateTurn(ctx, store.Turn{SpaceID: sp.ID, Role: "architect", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	data, trees := t.TempDir(), space.NewTrees(t.TempDir())
	for _, name := range []string{"login", "logout"} {
		w, err := space.EnsureFixWorkspace(ctx, st, trees, data, sp.ID, name, p)
		if err != nil {
			t.Fatal(err)
		}
		writeIn(t, w.Dir, "wip.txt", "draft of "+name)
		ref := fmt.Sprintf("refs/reef/checkpoints/x/%d/%d", turn.ID, w.ID)
		if err := worktree.Checkpoint(ctx, w.Dir, ref); err != nil {
			t.Fatal(err)
		}
		if err := st.AddTurnCheckpoint(ctx, turn.ID, w.ID, ref); err != nil {
			t.Fatal(err)
		}
		writeIn(t, w.Dir, "wip.txt", "changed")
		fixes = append(fixes, w)
	}
	turn.Status = "done"
	if _, err := st.EndTurn(ctx, turn); err != nil {
		t.Fatal(err)
	}
	return ts.URL, st, sp.ID, fixes
}

func TestRollback_RestoresTwoFixesOfOneRepository(t *testing.T) {
	url, _, _, fixes := twoFixes(t)

	if status, body := do(t, http.MethodPost, url+"/turns/1/rollback", ""); status != 204 {
		t.Fatalf("rollback = %d %s, want 204", status, body)
	}

	for _, w := range fixes {
		if b, err := os.ReadFile(filepath.Join(w.Dir, "wip.txt")); err != nil || string(b) != "draft of "+w.Name {
			t.Errorf("%s wip.txt = %q, %v, want its own draft", w.Name, b, err)
		}
	}
}

func TestInspector_ToolCallsNameTheFixesTheyWorkIn(t *testing.T) {
	url, st, spaceID, fixes := twoFixes(t)
	for _, input := range []string{
		`{"file_path":"` + fixes[0].Dir + `/a.txt"}`,
		`{"command":"cp login-app/a.txt logout-app/a.txt"}`,
	} {
		if _, err := st.CreateToolCall(context.Background(), 1, 0, "Bash", input); err != nil {
			t.Fatal(err)
		}
	}

	_, body := do(t, http.MethodGet, fmt.Sprintf("%s/spaces/%d/inspector", url, spaceID), "")

	one := fmt.Sprintf(`"targets":[{"projectId":1,"projectName":"app","workspaceId":%d}]`, fixes[0].ID)
	both := fmt.Sprintf(`"targets":[{"projectId":1,"projectName":"app","workspaceId":%d},`+
		`{"projectId":1,"projectName":"app","workspaceId":%d}]`, fixes[0].ID, fixes[1].ID)
	if !strings.Contains(body, one+"}") || !strings.Contains(body, both+"}") {
		t.Errorf("inspector = %s, want a call naming login and one naming both fixes", body)
	}
}

func TestRollback_PartialFailureSaysWhichRepositoriesWereRestored(t *testing.T) {
	url, _, spaceID, fixes := twoFixes(t)
	lock := gitIn(t, fixes[1].Dir, "rev-parse", "--path-format=absolute", "--git-path", "index.lock")
	writeIn(t, filepath.Dir(lock), filepath.Base(lock), "")

	status, body := do(t, http.MethodPost, url+"/turns/1/rollback", "")

	if status != 500 {
		t.Fatalf("rollback = %d %s, want 500", status, body)
	}
	if b, err := os.ReadFile(filepath.Join(fixes[0].Dir, "wip.txt")); err != nil || string(b) != "draft of login" {
		t.Errorf("login wip.txt = %q, %v, want restored", b, err)
	}
	_, msgs := do(t, http.MethodGet, fmt.Sprintf("%s/messages?space=%d", url, spaceID), "")
	if want := "Rollback to before turn 1 stopped partway. Restored: app (login). Not restored: app (logout)."; !strings.Contains(msgs, want) {
		t.Errorf("messages = %s, want %q", msgs, want)
	}
}
