package server

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/store"
	"github.com/bhanuprakaash/reef/worktree"
)

// twoTargetEnv is work item 1 "Rename" on project 2 "app" with project 3 "infra" as a second
// target, its Space (id 2), and a workspace of each, returned as app and infra folders.
func twoTargetEnv(t *testing.T) (env *liveEnv, app, infra string) {
	t.Helper()
	env = newLiveEnv(t)
	for _, name := range []string{"app", "infra"} {
		repo := t.TempDir()
		gitIn(t, repo, "init", "-b", "main")
		writeIn(t, repo, "a.txt", name)
		gitIn(t, repo, "add", ".")
		gitIn(t, repo, "commit", "-m", "first")
		do(t, http.MethodPost, env.url+"/projects", `{"name":"`+name+`","path":"`+repo+`"}`)
	}
	do(t, http.MethodPost, env.url+"/work-items", `{"projectId":2,"title":"Rename"}`)
	do(t, http.MethodPost, env.url+"/work-items/1/targets", `{"projectId":3}`)
	approveForSpace(t, env.st, 1)
	do(t, http.MethodPost, env.url+"/spaces", `{"projectId":2,"workItemId":1,"name":"c"}`)
	do(t, http.MethodPost, env.url+"/work-items/1/workspaces", "")
	ws, err := env.st.WorkItemWorkspaces(context.Background(), 1)
	if err != nil || len(ws) != 2 {
		t.Fatalf("workspaces = %+v, %v, want two", ws, err)
	}
	return env, ws[0].Dir, ws[1].Dir
}

func TestRollback_RestoresEveryWorkspaceOfTheTurn(t *testing.T) {
	ctx := context.Background()
	env, app, infra := twoTargetEnv(t)
	writeIn(t, app, "a.txt", "app edited")
	writeIn(t, infra, "new.txt", "n")
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
	turn.Status = "done"
	if _, err := env.st.EndTurn(ctx, turn); err != nil {
		t.Fatal(err)
	}
	if status, body := do(t, http.MethodGet, env.url+"/turns?space=2&rollbackable=1", ""); status != 200 ||
		!strings.Contains(body, `"checkpoints":[{"workspaceId":1,"projectId":2,"ref":"refs/reef/checkpoints/1"},`+
			`{"workspaceId":2,"projectId":3,"ref":"refs/reef/checkpoints/1"}]`) {
		t.Errorf("rollbackable turns = %d %s, want the turn with a checkpoint in each workspace", status, body)
	}
	writeIn(t, app, "a.txt", "app by the turn")
	writeIn(t, infra, "made.txt", "m")
	if err := os.Remove(filepath.Join(infra, "new.txt")); err != nil {
		t.Fatal(err)
	}

	if status, body := do(t, http.MethodPost, env.url+"/turns/1/rollback", ""); status != 204 || body != "" {
		t.Fatalf("rollback = %d %q, want 204", status, body)
	}

	if got := gitIn(t, app, "status", "--porcelain"); got != "M a.txt" {
		t.Errorf("status of app = %q, want a.txt as the checkpoint had it", got)
	}
	if b, err := os.ReadFile(filepath.Join(app, "a.txt")); err != nil || string(b) != "app edited" {
		t.Errorf("app a.txt = %q, %v, want app edited", b, err)
	}
	if got := gitIn(t, infra, "status", "--porcelain"); got != "?? new.txt" {
		t.Errorf("status of infra = %q, want only new.txt as the checkpoint had it", got)
	}
}

func TestGetWorkItemDiff_OneSectionPerWorkspace(t *testing.T) {
	env, app, infra := twoTargetEnv(t)
	writeIn(t, app, "api.txt", "a")
	writeIn(t, infra, "deploy.txt", "d")

	status, body := do(t, http.MethodGet, env.url+"/work-items/1/diff", "")

	if status != 200 || strings.Count(body, `"path":`) != 2 ||
		!strings.Contains(body, `"path":"2-app/api.txt"`) || !strings.Contains(body, `"path":"3-infra/deploy.txt"`) {
		t.Errorf("diff = %d %s, want one file under 2-app and one under 3-infra", status, body)
	}
}

func TestToolCallsNameTheTargetTheyWorkIn(t *testing.T) {
	ctx := context.Background()
	env, app, infra := twoTargetEnv(t)
	env.fake.tools = map[string]string{"lead": "Write"}
	env.fake.toolInput = map[string]string{"lead": `{"file_path":"` + infra + `/deploy.txt"}`}
	sub := subscribeTo(t, env.url, 2)
	sub.want(t, _idleFrame)

	if status, _ := env.sendTo(t, 2, "go"); status != 202 {
		t.Fatalf("send = %d, want 202", status)
	}
	sub.want(t, `turn {"role":"lead","status":"running"}`,
		`agent {"kind":"tool_call","toolId":"t","toolName":"Write","input":{"file_path":"`+infra+`/deploy.txt"},`+
			`"targets":[{"projectId":3,"projectName":"infra","workspaceId":2}]}`)

	turn, err := env.st.CreateTurn(ctx, store.Turn{SpaceID: 2, Role: "lead", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{
		`{"file_path":"` + app + `/api.txt"}`,
		`{"command":"cd 3-infra && git status"}`,
		`{"command":"cp 2-app/api.txt 3-infra/api.txt"}`,
		`{"command":"ls /tmp"}`,
	} {
		if _, err := env.st.CreateToolCall(ctx, turn.ID, 0, "Bash", input); err != nil {
			t.Fatal(err)
		}
	}

	_, body := do(t, http.MethodGet, env.url+"/spaces/2/inspector", "")
	for _, want := range []string{
		`"targets":[{"projectId":2,"projectName":"app","workspaceId":1}]`,
		`"input":"{\"command\":\"cd 3-infra \u0026\u0026 git status\"}","result":"","targets":[{"projectId":3,"projectName":"infra","workspaceId":2}]}`,
		`"input":"{\"command\":\"cp 2-app/api.txt 3-infra/api.txt\"}","result":"","targets":[{"projectId":2,"projectName":"app","workspaceId":1},` +
			`{"projectId":3,"projectName":"infra","workspaceId":2}]}`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("inspector = %s, want it to contain %s", body, want)
		}
	}
	if got := strings.Count(body, `"targets":`); got != 3 {
		t.Errorf("inspector has %d targets lists, want 3: a command in no repository has none", got)
	}
}
