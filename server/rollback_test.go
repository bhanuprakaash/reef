package server

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/store"
	"github.com/bhanuprakaash/reef/worktree"
)

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	full := append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)
	out, err := exec.Command("git", full...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func writeIn(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// rollbackEnv is a feature channel (id 2) whose worktree has a checkpoint on turn 1, with
// a.txt changed and new.txt untracked when the checkpoint was taken.
func rollbackEnv(t *testing.T) (env *liveEnv, tree string) {
	t.Helper()
	ctx := context.Background()
	env = newLiveEnv(t)
	repo := t.TempDir()
	gitIn(t, repo, "init", "-b", "main")
	writeIn(t, repo, "a.txt", "a")
	gitIn(t, repo, "add", ".")
	gitIn(t, repo, "commit", "-m", "first")
	do(t, http.MethodPost, env.url+"/projects", `{"name":"app","path":"`+repo+`"}`)
	do(t, http.MethodPost, env.url+"/features", `{"projectId":2,"slug":"login","name":"Login"}`)
	do(t, http.MethodPost, env.url+"/channels", `{"projectId":2,"featureId":1,"name":"c","backend":"claude"}`)
	tree = worktree.Path(env.dir, 2, "login")
	if err := worktree.Ensure(ctx, repo, tree, worktree.Branch("login"), ""); err != nil {
		t.Fatal(err)
	}
	writeIn(t, tree, "a.txt", "a edited")
	writeIn(t, tree, "new.txt", "n")
	const ref = "refs/reef/checkpoints/1"
	if err := worktree.Checkpoint(ctx, tree, ref); err != nil {
		t.Fatal(err)
	}
	turn, err := env.st.CreateTurn(ctx, store.Turn{ChannelID: 2, Role: "lead", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.st.SetTurnCheckpoint(ctx, turn.ID, ref); err != nil {
		t.Fatal(err)
	}
	turn.Status = "done"
	if _, err := env.st.EndTurn(ctx, turn); err != nil {
		t.Fatal(err)
	}
	return env, tree
}

func TestRollback_RestoresTheWorktree(t *testing.T) {
	env, tree := rollbackEnv(t)
	head := gitIn(t, tree, "rev-parse", "HEAD")
	status := gitIn(t, tree, "status", "--porcelain")
	writeIn(t, tree, "a.txt", "a by the turn")
	writeIn(t, tree, "made.txt", "m")
	gitIn(t, tree, "add", ".")
	gitIn(t, tree, "commit", "-m", "by the turn")
	if err := os.Remove(filepath.Join(tree, "new.txt")); err != nil {
		t.Fatal(err)
	}

	if status, body := do(t, http.MethodPost, env.url+"/turns/1/rollback", ""); status != 204 || body != "" {
		t.Fatalf("rollback = %d %q, want 204", status, body)
	}

	if got := gitIn(t, tree, "rev-parse", "HEAD"); got != head {
		t.Errorf("HEAD = %q, want %q", got, head)
	}
	if got := gitIn(t, tree, "status", "--porcelain"); got != status || got != "M a.txt\n?? new.txt" {
		t.Errorf("status = %q, want %q", got, status)
	}
	if b, err := os.ReadFile(filepath.Join(tree, "a.txt")); err != nil || string(b) != "a edited" {
		t.Errorf("a.txt = %q, %v, want %q", b, err, "a edited")
	}
	if _, err := os.Stat(filepath.Join(tree, "made.txt")); !os.IsNotExist(err) {
		t.Errorf("made.txt exists after rollback, err = %v", err)
	}
	_, body := do(t, http.MethodGet, env.url+"/messages?channel=2", "")
	if !strings.Contains(body, `"author":"reef","body":"Rolled back to before turn 1."`) {
		t.Errorf("messages = %s, want the rollback note from reef", body)
	}
	sub := subscribeTo(t, env.url, 2)
	if status, _ := env.sendTo(t, 2, "go on"); status != 202 {
		t.Errorf("send after rollback = %d, want 202: the channel is free again", status)
	}
	sub.want(t, _idleFrame, `turn {"role":"lead","status":"running"}`,
		`agent {"kind":"text","text":"hi"}`, `turn {"role":"lead","status":"done"}`)
	if got := env.ranTexts(); !slices.Equal(got, []string{"[channel 2, message 2]\ngo on"}) {
		t.Errorf("ran = %q, want the wake without the note: the runner adds it when it sends", got)
	}
}

func TestRollback_NotesEveryChannelOnTheTree(t *testing.T) {
	env, _ := rollbackEnv(t)
	do(t, http.MethodPost, env.url+"/channels", `{"projectId":2,"featureId":1,"name":"d","backend":"claude"}`)

	if status, body := do(t, http.MethodPost, env.url+"/turns/1/rollback", ""); status != 204 || body != "" {
		t.Fatalf("rollback = %d %q, want 204", status, body)
	}

	for _, id := range []int{2, 3} {
		_, body := do(t, http.MethodGet, env.url+"/messages?channel="+strconv.Itoa(id), "")
		if !strings.Contains(body, `"author":"reef","body":"Rolled back to before turn 1."`) {
			t.Errorf("channel %d messages = %s, want the rollback note from reef", id, body)
		}
	}
}

func TestRollback_Refused(t *testing.T) {
	env, tree := rollbackEnv(t)
	if _, err := env.st.CreateTurn(context.Background(),
		store.Turn{ChannelID: 2, Role: "lead", Backend: "claude"}); err != nil {
		t.Fatal(err)
	}
	writeIn(t, tree, "a.txt", "a by the turn")

	tests := []struct {
		name       string
		turn       string
		wantStatus int
		wantBody   string
	}{
		{"no checkpoint", "2", 400, `{"error":"invalid: turn 2 has no checkpoint"}`},
		{"unknown", "9", 404, `{"error":"turn 9: not found"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := do(t, http.MethodPost, env.url+"/turns/"+tt.turn+"/rollback", "")
			if status != tt.wantStatus || body != tt.wantBody {
				t.Errorf("rollback = %d %s, want %d %s", status, body, tt.wantStatus, tt.wantBody)
			}
		})
	}

	unlock, err := env.h.s.trees.Lock(context.Background(), tree)
	if err != nil {
		t.Fatal(err)
	}
	status, body := do(t, http.MethodPost, env.url+"/turns/1/rollback", "")
	if status != 409 || !strings.HasSuffix(body, `has a turn running: conflict"}`) {
		t.Errorf("rollback while a write turn holds the tree = %d %s, want 409", status, body)
	}
	unlock()

	env.sendTo(t, 2, "block")
	<-env.fake.started
	status, body = do(t, http.MethodPost, env.url+"/turns/1/rollback", "")
	if status != 409 || body != `{"error":"channel 2 has a turn running: conflict"}` {
		t.Errorf("rollback while a turn runs = %d %s, want 409", status, body)
	}
	env.fake.release <- struct{}{}
	if b, err := os.ReadFile(filepath.Join(tree, "a.txt")); err != nil || string(b) != "a by the turn" {
		t.Errorf("a.txt = %q, %v, want it untouched by refused rollbacks", b, err)
	}
	_, msgs := do(t, http.MethodGet, env.url+"/messages?channel=2", "")
	if strings.Contains(msgs, "Rolled back") {
		t.Errorf("messages = %s, want no rollback note after refused rollbacks", msgs)
	}
}
