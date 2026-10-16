package server

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/bhanuprakaash/reef/store"
	"github.com/bhanuprakaash/reef/worktree"
)

func TestRollback_OnlyInTheTreeItWasTakenIn(t *testing.T) {
	ctx := context.Background()
	env, tree := rollbackEnv(t)
	p, err := env.st.Project(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	do(t, http.MethodPost, env.url+"/channels", `{"projectId":2,"name":"quick","backend":"claude"}`)
	const ref = "refs/reef/checkpoints/2"
	if err := worktree.Checkpoint(ctx, p.Path, ref); err != nil {
		t.Fatal(err)
	}
	turn, err := env.st.CreateTurn(ctx, store.Turn{ChannelID: 3, Role: "lead", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.st.SetTurnCheckpoint(ctx, turn.ID, ref); err != nil {
		t.Fatal(err)
	}
	if _, body := do(t, http.MethodGet, env.url+"/turns?channel=3&rollbackable=1", ""); !strings.Contains(body, `"id":2,`) {
		t.Errorf("rollbackable turns of the quick session = %s, want turn 2", body)
	}
	if _, err := env.st.PromoteChannel(ctx, 3, store.Feature{Slug: "search", Name: "Search"}); err != nil {
		t.Fatal(err)
	}
	other := worktree.Path(env.dir, 2, "search")
	if err := worktree.Ensure(ctx, p.Path, other, worktree.Branch("search"), ""); err != nil {
		t.Fatal(err)
	}
	writeIn(t, other, "work.txt", "w")

	if status, body := do(t, http.MethodGet, env.url+"/turns?channel=3&rollbackable=1", ""); status != 200 || body != "[]" {
		t.Errorf("rollbackable turns after promote = %d %s, want 200 []", status, body)
	}
	if status, body := do(t, http.MethodGet, env.url+"/turns?rollbackable=1", ""); status != 400 {
		t.Errorf("rollbackable without a channel = %d %s, want 400", status, body)
	}
	status, body := do(t, http.MethodPost, env.url+"/turns/2/rollback", "")
	want := `{"error":"turn 2 was checkpointed in ` + p.Path + `, not ` + other + `: conflict"}`
	if status != 409 || body != want {
		t.Errorf("rollback after promote = %d %s, want 409 %s", status, body, want)
	}
	if got := gitIn(t, other, "status", "--porcelain"); got != "?? work.txt" {
		t.Errorf("status of the feature worktree = %q, want work.txt left alone", got)
	}
	if got := gitIn(t, tree, "status", "--porcelain"); got != "M a.txt\n?? new.txt" {
		t.Errorf("status of the other worktree = %q, want it untouched", got)
	}
}

func TestRollback_RefusedWhileTheTreeIsRead(t *testing.T) {
	env, tree := rollbackEnv(t)
	writeIn(t, tree, "a.txt", "a by the turn")
	done := env.h.s.trees.Read(tree)

	status, body := do(t, http.MethodPost, env.url+"/turns/1/rollback", "")
	if want := `{"error":"` + tree + ` has a turn running: conflict"}`; status != 409 || body != want {
		t.Errorf("rollback while a read-only turn runs = %d %s, want 409 %s", status, body, want)
	}
	if b, err := os.ReadFile(filepath.Join(tree, "a.txt")); err != nil || string(b) != "a by the turn" {
		t.Errorf("a.txt = %q, %v, want it untouched", b, err)
	}
	done()
	if status, body := do(t, http.MethodPost, env.url+"/turns/1/rollback", ""); status != 204 {
		t.Errorf("rollback after the read ended = %d %s, want 204", status, body)
	}
}

func TestRollback_RefusedWhileReefChatHasTheChannel(t *testing.T) {
	env, tree := rollbackEnv(t)
	writeIn(t, tree, "a.txt", "a by the turn")
	lock, err := os.Create(filepath.Join(env.dir, "chat-2.lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Close() }()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}

	status, body := do(t, http.MethodPost, env.url+"/turns/1/rollback", "")
	if want := `{"error":"channel 2 is open elsewhere: conflict"}`; status != 409 || body != want {
		t.Errorf("rollback while reef chat has the channel = %d %s, want 409 %s", status, body, want)
	}
	if b, err := os.ReadFile(filepath.Join(tree, "a.txt")); err != nil || string(b) != "a by the turn" {
		t.Errorf("a.txt = %q, %v, want it untouched", b, err)
	}
}

func TestRollback_ChecksOutTheBranchItWasTakenOn(t *testing.T) {
	env, tree := rollbackEnv(t)
	head := gitIn(t, tree, "rev-parse", "HEAD")
	gitIn(t, tree, "checkout", "-b", "other")
	writeIn(t, tree, "b.txt", "b")
	gitIn(t, tree, "add", ".")
	gitIn(t, tree, "commit", "-m", "on other")

	if status, body := do(t, http.MethodPost, env.url+"/turns/1/rollback", ""); status != 204 {
		t.Fatalf("rollback = %d %s, want 204", status, body)
	}

	if got := gitIn(t, tree, "branch", "--show-current"); got != "feature/login" {
		t.Errorf("branch = %q, want feature/login", got)
	}
	if got := gitIn(t, tree, "rev-parse", "feature/login"); got != head {
		t.Errorf("feature/login = %q, want %q", got, head)
	}
	if got := gitIn(t, tree, "status", "--porcelain"); got != "M a.txt\n?? new.txt" {
		t.Errorf("status = %q, want the checkpoint's changes", got)
	}
}
