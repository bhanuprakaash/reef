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
