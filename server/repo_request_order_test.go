package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/bhanuprakaash/reef/store"
)

func TestOpenRepoRequest_QuickFixAcrossRepositoriesKeepsMergeOrder(t *testing.T) {
	ctx := context.Background()
	ts, st := newEmbedServer(t, nil)
	bot := *testServerBot(t, st, "architect")
	sp, err := st.CreateBotSpace(ctx, store.Space{Name: "fix"}, bot)
	if err != nil {
		t.Fatal(err)
	}
	tok, err := st.CreateToken(ctx, sp.ID, bot)
	if err != nil {
		t.Fatal(err)
	}
	c, err := st.PutRepoConnection(ctx, store.RepoConnection{Provider: store.RepoGitLab, Host: "gitlab.com", State: store.RepoConnected, CheckedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	ws := map[string]store.Workspace{}
	repos := map[string]string{}
	for _, name := range []string{"lib", "app"} {
		repo, _ := originRepo(t, "develop")
		p, err := st.CreateProject(ctx, store.Project{Name: name, Path: repo, BaseBranch: "main"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = st.SetProjectRemote(ctx, p.ID, c.ID, "group/"+name); err != nil {
			t.Fatal(err)
		}
		w, err := st.EnsureWorkspace(ctx, store.Workspace{ProjectID: p.ID, SpaceID: &sp.ID, Name: "sync", Dir: repo, Branch: "fix", Base: "main"})
		if err != nil {
			t.Fatal(err)
		}
		gitIn(t, repo, "checkout", "-b", "fix")
		w.State = store.WorkspaceReady
		if err = st.SetWorkspace(ctx, w); err != nil {
			t.Fatal(err)
		}
		ws[name], repos[name] = w, repo
	}
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	git := fmt.Sprintf(`#!/bin/sh
if [ "$1 $2" = 'remote get-url' ]; then
  if [ "$(pwd -P)" = "$(cd "$LIB_DIR" && pwd -P)" ]; then echo https://gitlab.com/group/lib.git; else echo https://gitlab.com/group/app.git; fi
else exec %q "$@"; fi
`, realGit)
	glab := `#!/bin/sh
name=$(basename "$4")
iid=12; [ "$name" = lib ] && iid=11
if [ "$2" = list ]; then
  if [ -f "$MR_DIR/$name" ]; then echo '[{"iid":'$iid',"web_url":"https://gitlab.com/group/'$name'/-/merge_requests/'$iid'","state":"opened","source_branch":"fix","target_branch":"main","source_project_id":5,"target_project_id":5}]'; else echo '[]'; fi
elif [ "$2" = create ]; then
  touch "$MR_DIR/$name"
fi
`
	for name, content := range map[string]string{"git": git, "glab": glab} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(content), 0755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("MR_DIR", t.TempDir())
	t.Setenv("LIB_DIR", repos["lib"])

	type request struct {
		ID        int64
		Number    int64
		DependsOn []int64
	}
	call := func(args map[string]any) (request, string, bool) {
		t.Helper()
		text, isErr := callToolResult(t, st, tok, t.TempDir())("open_repo_request", args)
		var r request
		if !isErr {
			if err := json.Unmarshal([]byte(text), &r); err != nil {
				t.Fatal(err)
			}
		}
		return r, text, isErr
	}
	lib, text, isErr := call(map[string]any{"workspaceId": ws["lib"].ID, "title": "Lib", "description": "d"})
	if isErr || lib.Number != 11 || len(lib.DependsOn) != 0 {
		t.Fatalf("lib request = %s, error %v", text, isErr)
	}
	app, text, isErr := call(map[string]any{"workspaceId": ws["app"].ID, "title": "App", "description": "d", "dependsOn": []int64{lib.ID}})
	if isErr || app.Number != 12 || !reflect.DeepEqual(app.DependsOn, []int64{lib.ID}) {
		t.Fatalf("app request = %s, error %v", text, isErr)
	}
	app, text, isErr = call(map[string]any{"workspaceId": ws["app"].ID, "title": "App", "description": "d"})
	if isErr || app.Number != 12 || !reflect.DeepEqual(app.DependsOn, []int64{lib.ID}) {
		t.Fatalf("follow-up app request = %s, error %v", text, isErr)
	}
	if _, text, isErr = call(map[string]any{"workspaceId": ws["lib"].ID, "title": "Lib", "description": "d", "dependsOn": []int64{app.ID}}); !isErr ||
		!strings.Contains(text, "would wait on itself") {
		t.Fatalf("cycle = %s, error %v", text, isErr)
	}

	status, body := do(t, http.MethodGet, fmt.Sprintf("%s/spaces/%d/workspaces", ts.URL, sp.ID), "")
	var listed []struct {
		ProjectName string
		Requests    []store.RepoRequest
	}
	if status != 200 || json.Unmarshal([]byte(body), &listed) != nil || len(listed) != 2 ||
		len(listed[0].Requests) != 1 || len(listed[1].Requests) != 1 ||
		!reflect.DeepEqual(listed[1].Requests[0].DependsOn, []int64{lib.ID}) {
		t.Fatalf("space workspaces = %d %s", status, body)
	}
}
