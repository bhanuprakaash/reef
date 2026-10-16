package channel

import (
	"context"
	"strings"
	"testing"
)

func TestContext_BaseBranch(t *testing.T) {
	ctx := context.Background()
	st, f := openStore(t)
	p, err := st.Project(ctx, f.ProjectID)
	if err != nil {
		t.Fatal(err)
	}

	got, err := Context(ctx, st, f.ID, "architect")
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
	got, err = Context(ctx, st, f.ID, "architect")
	if err != nil {
		t.Fatal(err)
	}
	if want := "Base branch: dev. A pull request you open targets it.\n"; !strings.Contains(got, want) {
		t.Errorf("Context =\n%s\nwant it to contain %q", got, want)
	}
}
