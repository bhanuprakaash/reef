package server

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/store"
	"github.com/bhanuprakaash/reef/worktree"
)

// projectTrees extends rollbackEnv (project 2, work item 1 "login", worktree login, ref
// checkpoints/1) with work item 2 "signup" and a clean worktree whose turn has ref
// checkpoints/2, and a quick-session Space whose quick-fix workspace has a turn with ref checkpoints/3.
// It returns the main checkout's .git folder and signup's worktree.
func projectTrees(t *testing.T, env *liveEnv, login string) (repo, signup string) {
	t.Helper()
	ctx := context.Background()
	repo = gitIn(t, login, "rev-parse", "--path-format=absolute", "--git-common-dir")
	main := filepath.Dir(repo)
	do(t, http.MethodPost, env.url+"/work-items", `{"projectId":2,"title":"Signup"}`)
	approveForSpace(t, env.st, 2)
	do(t, http.MethodPost, env.url+"/spaces", `{"projectId":2,"workItemId":2,"name":"s"}`)
	do(t, http.MethodPost, env.url+"/spaces", `{"projectId":2,"name":"quick"}`)
	do(t, http.MethodPost, env.url+"/work-items/2/workspaces", "")
	ws, err := env.st.WorkItemWorkspaces(ctx, 2)
	if err != nil || len(ws) != 1 {
		t.Fatalf("workspaces = %+v, %v, want one", ws, err)
	}
	signup = ws[0].Dir
	fixID := int64(4)
	fix, err := env.st.EnsureWorkspace(ctx, store.Workspace{
		ProjectID: 2, SpaceID: &fixID, Name: "tidy", Dir: filepath.Join(env.dir, "chats", "4", "tidy-app"),
		Branch: "fix/4-tidy",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.Create(ctx, main, fix.Dir, fix.Branch, ""); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		space     int64
		workspace int64
		dir       string
	}{{3, ws[0].ID, signup}, {4, fix.ID, fix.Dir}} {
		turn, err := env.st.CreateTurn(ctx, store.Turn{SpaceID: c.space, Role: "lead", Backend: "claude"})
		if err != nil {
			t.Fatal(err)
		}
		ref := "refs/reef/checkpoints/" + strconv.FormatInt(turn.ID, 10)
		if err := worktree.Checkpoint(ctx, c.dir, ref); err != nil {
			t.Fatal(err)
		}
		if err := env.st.AddTurnCheckpoint(ctx, turn.ID, c.workspace, ref); err != nil {
			t.Fatal(err)
		}
	}
	return repo, signup
}

func TestDeleteProject_KeepsRepositoryWorkspacesRefsAndUnfinishedChanges(t *testing.T) {
	env, login := rollbackEnv(t)
	repo, signup := projectTrees(t, env, login)
	gitIn(t, login, "add", ".")
	gitIn(t, login, "commit", "-m", "work")
	writeIn(t, signup, "new.txt", "unfinished")

	if status, body := do(t, http.MethodDelete, env.url+"/projects/2", ""); status != 204 || body != "" {
		t.Fatalf("delete = %d %q, want 204", status, body)
	}

	if got, err := os.ReadFile(filepath.Join(signup, "new.txt")); err != nil || string(got) != "unfinished" {
		t.Errorf("unfinished change after delete = %q, %v, want kept", got, err)
	}
	if _, err := os.Stat(login); err != nil {
		t.Errorf("worktree after delete: %v, want kept", err)
	}
	got := gitIn(t, repo, "for-each-ref", "--format=%(refname)", "refs/reef/checkpoints")
	if got != "refs/reef/checkpoints/1\nrefs/reef/checkpoints/2\nrefs/reef/checkpoints/3" {
		t.Errorf("checkpoint refs = %q, want all three", got)
	}
	if n := env.count(t, "SELECT COUNT(*) FROM workspace WHERE project_id = 2 AND state = 'removed'"); n != 0 {
		t.Errorf("removed workspace rows = %d, want 0", n)
	}
	if status, body := env.sendTo(t, 4, "hello"); status != 202 {
		t.Errorf("send to the quick-fix conversation after delete = %d %s, want 202", status, body)
	}
	if status, body := do(t, http.MethodGet, env.url+"/projects/2", ""); status != 200 ||
		!strings.Contains(body, `"name":"app"`) || !strings.Contains(body, `"deletedAt":"`) {
		t.Errorf("GET project after delete = %d %s, want 200 with deletedAt", status, body)
	}
	if _, body := do(t, http.MethodGet, env.url+"/projects", ""); strings.Contains(body, `"name":"app"`) {
		t.Errorf("GET /projects = %s, want the deleted project left out", body)
	}
	if status, body := do(t, http.MethodGet, env.url+"/projects?deleted=1", ""); status != 200 || !strings.Contains(body, `"name":"app"`) {
		t.Errorf("deleted projects = %d %s, want app", status, body)
	}
	if status, body := do(t, http.MethodGet, env.url+"/spaces/2/projects?history=1", ""); status != 200 || !strings.Contains(body, `"name":"app"`) || !strings.Contains(body, `"deletedAt":"`) {
		t.Errorf("history projects = %d %s, want deleted app", status, body)
	}
	if status, _ := do(t, http.MethodGet, env.url+"/messages?space=2", ""); status != 200 {
		t.Errorf("GET messages of a deleted project's Space = %d, want 200", status)
	}
}

func TestDeleteProject_RetiresOnlyTheProjectsOwnMemory(t *testing.T) {
	env, login := rollbackEnv(t)
	projectTrees(t, env, login)
	proj := seedMemory(t, env, "project:2")
	item := seedMemory(t, env, "work_item:1")
	other := seedMemory(t, env, "project:1")

	if status, body := do(t, http.MethodDelete, env.url+"/projects/2", ""); status != 204 || body != "" {
		t.Fatalf("delete = %d %q, want 204", status, body)
	}

	wantStatuses(t, env, proj, "retired", "retired", "rejected")
	wantStatuses(t, env, item, "accepted", "accepted", "candidate")
	wantStatuses(t, env, other, "accepted", "accepted", "candidate")
}

func TestDeleteProject_StopsTheRunningTurnAndPausesTheMixedItem(t *testing.T) {
	env, app, infra := twoTargetEnv(t)
	env.exec(t, "UPDATE work_item SET status = 'in_progress' WHERE id = 1")
	if status, body := env.sendTo(t, 2, "block"); status != 202 {
		t.Fatalf("send = %d %s, want 202", status, body)
	}
	<-env.fake.started

	if status, body := do(t, http.MethodDelete, env.url+"/projects/3", ""); status != 204 || body != "" {
		t.Fatalf("delete infra = %d %q, want 204", status, body)
	}

	if n := env.count(t, "SELECT COUNT(*) FROM turn WHERE space_id = 2 AND status = 'running'"); n != 0 {
		t.Errorf("running turns after delete = %d, want 0", n)
	}
	for _, dir := range []string{app, infra} {
		if _, err := os.Stat(dir); err != nil {
			t.Errorf("workspace %s after delete: %v, want kept", dir, err)
		}
	}
	if _, body := do(t, http.MethodGet, env.url+"/work-items/1", ""); !strings.Contains(body, `"status":"needs_you"`) {
		t.Errorf("item = %s, want needs_you", body)
	}
	if _, body := do(t, http.MethodGet, env.url+"/messages?space=2", ""); !strings.Contains(body, "Project infra was deleted.") {
		t.Errorf("messages = %s, want a note naming infra", body)
	}
	if n := env.count(t, "SELECT COUNT(*) FROM workspace WHERE state = 'removed'"); n != 0 {
		t.Errorf("removed workspace rows = %d, want 0", n)
	}

	if status, body := do(t, http.MethodPost, env.url+"/spaces/2/continue", ""); status != 409 ||
		!strings.Contains(body, "runs against deleted project infra") {
		t.Errorf("continue = %d %s, want 409 naming infra", status, body)
	}
	if status, body := do(t, http.MethodDelete, env.url+"/work-items/1/targets/3", ""); status != 204 {
		t.Fatalf("remove deleted target = %d %s, want 204", status, body)
	}
	if status, body := do(t, http.MethodPost, env.url+"/spaces/2/continue", ""); status != 204 && status != 202 {
		t.Errorf("continue after the target is removed = %d %s, want it accepted", status, body)
	}
}

func TestDeleteProject_QueuedItemDoesNotStartAndNothingNewIsAdmitted(t *testing.T) {
	env, _, infra := twoTargetEnv(t)
	do(t, http.MethodPost, env.url+"/work-items", `{"projectId":2,"title":"Second"}`)
	do(t, http.MethodPost, env.url+"/work-items/2/targets", `{"projectId":3}`)
	approveForSpace(t, env.st, 2)
	env.exec(t, "UPDATE work_item SET status = 'queued', queue_position = 1 WHERE id = 2")
	if _, err := env.st.MarkProjectDeleting(context.Background(), 3); err != nil {
		t.Fatal(err)
	}

	if _, err := env.st.MoveWorkItem(context.Background(), 2, "in_progress", store.ByQueue); err == nil {
		t.Error("queue start while deleting succeeded, want ErrConflict")
	}
	if status, body := do(t, http.MethodPost, env.url+"/work-items/1/targets", `{"projectId":3}`); status != 400 {
		t.Errorf("add deleting project as target = %d %s, want 400", status, body)
	}
	if status, body := do(t, http.MethodPost, env.url+"/work-items", `{"projectId":3,"title":"New"}`); status != 400 {
		t.Errorf("create work item in deleting project = %d %s, want 400", status, body)
	}
	if status, body := env.sendTo(t, 2, "hello"); status != 409 || !strings.Contains(body, "deleted project infra") {
		t.Errorf("send to a Space against a deleting project = %d %s, want 409", status, body)
	}

	if status, body := do(t, http.MethodDelete, env.url+"/projects/3", ""); status != 204 {
		t.Fatalf("delete infra = %d %s, want 204", status, body)
	}

	if _, body := do(t, http.MethodGet, env.url+"/work-items/2", ""); !strings.Contains(body, `"status":"backlog"`) {
		t.Errorf("queued item = %s, want back in backlog", body)
	}
	if n := env.count(t, "SELECT COUNT(*) FROM space WHERE work_item_id = 2"); n != 0 {
		t.Errorf("Spaces of the queued item = %d, want 0", n)
	}
	if _, err := os.Stat(infra); err != nil {
		t.Errorf("infra workspace: %v, want kept", err)
	}
}

func TestDeleteProject_RestartFinishesADeletionCutBetweenDeletingAndDeleted(t *testing.T) {
	env, _, _ := twoTargetEnv(t)
	env.exec(t, "UPDATE work_item SET status = 'in_progress' WHERE id = 1")
	proj := seedMemory(t, env, "project:3")
	if _, err := env.st.MarkProjectDeleting(context.Background(), 3); err != nil {
		t.Fatal(err)
	}
	if n := env.count(t, "SELECT COUNT(*) FROM project WHERE id = 3 AND deleted_at IS NOT NULL"); n != 0 {
		t.Fatalf("deleted before restart = %d, want 0", n)
	}

	env.restart(t)

	if n := env.count(t, "SELECT COUNT(*) FROM project WHERE id = 3 AND deleted_at IS NOT NULL"); n != 1 {
		t.Errorf("deleted after restart = %d, want 1", n)
	}
	if _, body := do(t, http.MethodGet, env.url+"/work-items/1", ""); !strings.Contains(body, `"status":"needs_you"`) {
		t.Errorf("item after restart = %s, want needs_you", body)
	}
	wantStatuses(t, env, proj, "retired", "retired", "rejected")
	if n := env.count(t, "SELECT COUNT(*) FROM turn WHERE space_id = 2"); n != 0 {
		t.Errorf("turns after restart = %d, want none", n)
	}
}
