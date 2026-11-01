package space

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/bhanuprakaash/reef/store"
)

// closeSpace ends the Space, as approving its wrap-up does, so its work item may have another.
func closeSpace(t *testing.T, st *store.Store, id int64) {
	t.Helper()
	if _, err := st.DB().Exec("UPDATE space SET closed_at = ? WHERE id = ?",
		time.Now().UTC().Format(time.RFC3339Nano), id); err != nil {
		t.Fatal(err)
	}
}

func TestContext_BaseBranch(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	p, err := st.Project(ctx, f.ProjectID)
	if err != nil {
		t.Fatal(err)
	}

	got, err := Context(ctx, st, testMemory(t, st), f.ID, "architect")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "Base branch") {
		t.Errorf("Context before workspaces =\n%s\nwant no Base branch line", got)
	}

	if out, err := exec.Command("git", "-C", p.Path, "branch", "dev").CombinedOutput(); err != nil {
		t.Fatalf("git branch: %v: %s", err, out)
	}
	p.BaseBranch = "dev"
	if _, err := st.UpdateProject(ctx, p); err != nil {
		t.Fatal(err)
	}
	f, err = st.WorkItem(ctx, f.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureWorkspaces(ctx, st, NewTrees(t.TempDir()), t.TempDir(), f); err != nil {
		t.Fatal(err)
	}
	got, err = Context(ctx, st, testMemory(t, st), f.ID, "architect")
	if err != nil {
		t.Fatal(err)
	}
	if want := "- shop: 1-shop/ on branch work-item/1, base dev (local): origin could not be refreshed"; !strings.Contains(got, want) {
		t.Errorf("Context =\n%s\nwant it to contain %q", got, want)
	}
}
