package store

import (
	"context"
	"testing"
)

func TestUpdateFeatureKeepsSlug(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	if _, err := s.CreateProject(ctx, Project{Name: "reef", Path: "/tmp/reef"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateFeature(ctx, Feature{ProjectID: 1, Slug: "f", Name: "f"}); err != nil {
		t.Fatal(err)
	}

	got, err := s.UpdateFeature(ctx, Feature{ID: 1, Slug: "renamed", Name: "F"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Slug != "f" || got.Name != "F" {
		t.Errorf("UpdateFeature = slug %q name %q, want slug %q name %q", got.Slug, got.Name, "f", "F")
	}
	got, err = s.Feature(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.Slug != "f" || got.Name != "F" {
		t.Errorf("Feature = slug %q name %q, want slug %q name %q", got.Slug, got.Name, "f", "F")
	}
}
