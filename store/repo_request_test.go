package store

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
)

func TestRepoRequest_DependsOnStaysInOneFix(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	for _, name := range []string{"lib", "app"} {
		if _, err := s.CreateProject(ctx, Project{Name: name, Path: "/tmp/" + name}); err != nil {
			t.Fatal(err)
		}
	}
	sp, err := s.CreateSpace(ctx, Space{ProjectID: 1, Name: "chat"})
	if err != nil {
		t.Fatal(err)
	}
	put := func(project int64, fix string, number int64) RepoRequest {
		t.Helper()
		w, err := s.EnsureWorkspace(ctx, Workspace{ProjectID: project, SpaceID: &sp.ID, Name: fix, Dir: fmt.Sprintf("/tmp/%s-%d", fix, project), Branch: "fix/" + fix})
		if err != nil {
			t.Fatal(err)
		}
		r, err := s.PutRepoRequest(ctx, RepoRequest{Provider: "gitlab", Host: "gitlab.com", Path: "g/p", Number: number,
			URL: "u", State: "opened", Head: w.Branch, Base: "main", WorkspaceID: w.ID, SpaceID: &sp.ID})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	lib, app, other := put(1, "sync", 1), put(2, "sync", 2), put(1, "other", 3)
	if len(lib.DependsOn) != 0 || lib.DependsOn == nil {
		t.Fatalf("new request dependsOn = %#v, want an empty list", lib.DependsOn)
	}

	got, err := s.SetRepoRequestDependsOn(ctx, app.ID, []int64{lib.ID, lib.ID})
	if err != nil || !reflect.DeepEqual(got.DependsOn, []int64{lib.ID}) {
		t.Fatalf("app dependsOn = %v, %v; want [%d]", got.DependsOn, err, lib.ID)
	}
	again := put(2, "sync", 2)
	if !reflect.DeepEqual(again.DependsOn, []int64{lib.ID}) {
		t.Errorf("a follow-up that opens the same request changed dependsOn to %v", again.DependsOn)
	}

	for name, c := range map[string]struct {
		id   int64
		deps []int64
	}{
		"itself":          {app.ID, []int64{app.ID}},
		"another fix":     {app.ID, []int64{other.ID}},
		"a cycle":         {lib.ID, []int64{app.ID}},
		"unknown request": {app.ID, []int64{99}},
	} {
		if _, err := s.SetRepoRequestDependsOn(ctx, c.id, c.deps); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
	if _, err := s.SetRepoRequestDependsOn(ctx, 99, nil); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown request: err = %v, want ErrNotFound", err)
	}

	got, err = s.SetRepoRequestDependsOn(ctx, app.ID, []int64{})
	if err != nil || len(got.DependsOn) != 0 {
		t.Errorf("clear = %v, %v; want empty", got.DependsOn, err)
	}
}
