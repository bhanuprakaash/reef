package server

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/store"
	"github.com/bhanuprakaash/reef/worktree"
)

func TestNoteInterrupted_SaysWhichRepositoriesChanged(t *testing.T) {
	ctx := context.Background()
	env, app, infra := twoTargetEnv(t)
	env.exec(t, "UPDATE work_item SET status = 'in_progress' WHERE id = 1")
	turn, err := env.st.CreateTurn(ctx, store.Turn{SpaceID: 2, Role: "lead", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	for i, dir := range []string{app, infra} {
		const ref = "refs/reef/checkpoints/1"
		if err := worktree.Checkpoint(ctx, dir, ref); err != nil {
			t.Fatal(err)
		}
		if err := env.st.AddTurnCheckpoint(ctx, turn.ID, int64(i+1), ref); err != nil {
			t.Fatal(err)
		}
	}
	writeIn(t, app, "a.txt", "app edited")
	writeIn(t, app, "b.txt", "new")

	failed, err := env.st.FailRunningTurns(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	paused, err := env.st.PauseInterrupted(ctx, failed)
	if err != nil {
		t.Fatal(err)
	}
	if err := NoteInterrupted(ctx, env.st, paused, failed); err != nil {
		t.Fatal(err)
	}

	_, body := do(t, http.MethodGet, env.url+"/messages?space=2", "")
	if !strings.Contains(body, "Interrupted by a restart. Since the turn started, app: 2 files changed; infra: no changes.") {
		t.Errorf("messages = %s, want the changes of app and infra", body)
	}
}
