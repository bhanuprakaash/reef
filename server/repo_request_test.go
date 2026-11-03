package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bhanuprakaash/reef/store"
)

func TestGitLabRequest_CreateRetryAndRejection(t *testing.T) {
	ctx := context.Background()
	ts, st := newEmbedServer(t, nil)
	repo, _ := originRepo(t, "develop")
	p, err := st.CreateProject(ctx, store.Project{Name: "request", Path: repo, BaseBranch: "main"})
	if err != nil {
		t.Fatal(err)
	}
	sp, err := st.CreateBotSpace(ctx, store.Space{Name: "fix"}, *testServerBot(t, st, "architect"))
	if err != nil {
		t.Fatal(err)
	}
	c, err := st.PutRepoConnection(ctx, store.RepoConnection{Provider: store.RepoGitLab, Host: "gitlab.com", State: store.RepoConnected, CheckedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = st.SetProjectRemote(ctx, p.ID, c.ID, "group/app"); err != nil {
		t.Fatal(err)
	}
	w, err := st.EnsureWorkspace(ctx, store.Workspace{ProjectID: p.ID, SpaceID: &sp.ID, Name: "fix", Dir: repo, Branch: "fix", Base: "main"})
	if err != nil {
		t.Fatal(err)
	}
	gitIn(t, repo, "checkout", "-b", "fix")
	w.State = store.WorkspaceReady
	if err = st.SetWorkspace(ctx, w); err != nil {
		t.Fatal(err)
	}
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	marker := filepath.Join(t.TempDir(), "created")
	git := fmt.Sprintf("#!/bin/sh\nif [ \"$1 $2\" = 'remote get-url' ]; then echo https://gitlab.com/group/app.git; else exec %q \"$@\"; fi\n", realGit)
	glab := `#!/bin/sh
if [ "$2" = list ]; then
  if [ -f "$MR_MARKER" ]; then echo '[{"iid":7,"web_url":"https://gitlab.com/group/app/-/merge_requests/7","state":"opened","source_branch":"fix","target_branch":"main","source_project_id":12,"target_project_id":12}]'; else echo '[]'; fi
elif [ "$2" = create ]; then
  if [ -n "$MR_REJECT" ]; then echo 'token glpat-secret' >&2; exit 1; fi
  touch "$MR_MARKER"
fi
`
	for name, content := range map[string]string{"git": git, "glab": glab} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(content), 0755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("MR_MARKER", marker)
	url := fmt.Sprintf("%s/workspaces/%d/request", ts.URL, w.ID)
	t.Setenv("MR_REJECT", "yes")
	status, body := do(t, http.MethodPost, url, `{"title":"Fix","description":"details"}`)
	if status != http.StatusConflict || !strings.Contains(body, "GitLab could not create") || strings.Contains(body, "secret") {
		t.Fatalf("rejection = %d %s", status, body)
	}
	rows, err := st.WorkspaceRequests(ctx, w.ID)
	if err != nil || len(rows) != 0 {
		t.Fatalf("after rejection: %+v %v", rows, err)
	}
	if got := gitIn(t, repo, "branch", "--show-current"); got != "fix" {
		t.Fatalf("branch changed: %s", got)
	}
	t.Setenv("MR_REJECT", "")
	for i := 0; i < 2; i++ {
		status, body = do(t, http.MethodPost, url, `{"title":"Fix","description":"details"}`)
		var got store.RepoRequest
		if status != 200 || json.Unmarshal([]byte(body), &got) != nil || got.Number != 7 || got.Path != "group/app" || got.SpaceID == nil || *got.SpaceID != sp.ID {
			t.Fatalf("request %d = %d %s", i, status, body)
		}
	}
	status, body = do(t, http.MethodGet, fmt.Sprintf("%s/workspaces/%d/requests", ts.URL, w.ID), "")
	if status != 200 || json.Unmarshal([]byte(body), &rows) != nil || len(rows) != 1 {
		t.Fatalf("list = %d %s", status, body)
	}
}
