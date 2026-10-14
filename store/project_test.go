package store

import (
	"context"
	"errors"
	"testing"
)

func TestProjectByPath(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	for _, p := range []Project{
		{Name: "outer", Path: "/work/shop"},
		{Name: "inner", Path: "/work/shop/api"},
	} {
		if _, err := s.CreateProject(ctx, p); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		name     string
		give     string
		wantName string
	}{
		{name: "exact", give: "/work/shop", wantName: "outer"},
		{name: "nearest parent", give: "/work/shop/api/handlers", wantName: "inner"},
		{name: "parent of a sibling", give: "/work/shop/web", wantName: "outer"},
		{name: "trailing slash", give: "/work/shop/", wantName: "outer"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.ProjectByPath(ctx, tt.give)
			if err != nil || got.Name != tt.wantName {
				t.Errorf("ProjectByPath(%q) = %q, %v, want %q", tt.give, got.Name, err, tt.wantName)
			}
		})
	}

	if _, err := s.ProjectByPath(ctx, "/work/other"); !errors.Is(err, ErrNotFound) {
		t.Errorf("ProjectByPath(/work/other) error = %v, want ErrNotFound", err)
	}
}

func TestProjectMemory(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	p, err := s.CreateProject(ctx, Project{Name: "shop", Path: "/work/shop"})
	if err != nil {
		t.Fatal(err)
	}

	m, err := s.ProjectMemory(ctx, p.ID)
	if err != nil || m.Body != "" {
		t.Fatalf("ProjectMemory of new project = %+v, %v, want empty", m, err)
	}

	m, err = s.PutProjectMemory(ctx, ProjectMemory{ProjectID: p.ID, Body: "use sessions"})
	if err != nil || m.Body != "use sessions" {
		t.Fatalf("PutProjectMemory = %+v, %v, want body set", m, err)
	}

	m, err = s.ProjectMemory(ctx, p.ID)
	if err != nil || m.Body != "use sessions" {
		t.Fatalf("ProjectMemory after put = %+v, %v, want %q", m, err, "use sessions")
	}

	if _, err := s.ProjectMemory(ctx, 9); !errors.Is(err, ErrNotFound) {
		t.Errorf("ProjectMemory(9) error = %v, want ErrNotFound", err)
	}
	if _, err := s.PutProjectMemory(ctx, ProjectMemory{ProjectID: 9, Body: "x"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("PutProjectMemory(9) error = %v, want ErrNotFound", err)
	}
}
