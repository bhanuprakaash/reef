package space

import (
	"context"
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
		t.Errorf("Context with no base branch =\n%s\nwant no Base branch line", got)
	}

	p.BaseBranch = "dev"
	if _, err := st.UpdateProject(ctx, p); err != nil {
		t.Fatal(err)
	}
	got, err = Context(ctx, st, testMemory(t, st), f.ID, "architect")
	if err != nil {
		t.Fatal(err)
	}
	if want := "Base branch: dev. A pull request you open targets it.\n"; !strings.Contains(got, want) {
		t.Errorf("Context =\n%s\nwant it to contain %q", got, want)
	}
}
