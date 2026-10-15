package store

import (
	"context"
	"errors"
	"testing"
)

func TestCloseFeatureClosesItsChannelsAndReopenKeepsThemClosed(t *testing.T) {
	ctx := context.Background()
	s := proposalFixture(t)
	if err := s.CloseFeature(ctx, 1); err != nil {
		t.Fatal(err)
	}

	f, err := s.Feature(ctx, 1)
	if err != nil || f.ClosedAt == nil {
		t.Fatalf("feature 1 closedAt = %v, err = %v, want set", f.ClosedAt, err)
	}
	c, err := s.Channel(ctx, 1)
	if err != nil || c.ClosedAt == nil {
		t.Errorf("channel 1 closedAt = %v, err = %v, want set", c.ClosedAt, err)
	}
	if q, err := s.Channel(ctx, 2); err != nil || q.ClosedAt != nil {
		t.Errorf("quick session closedAt = %v, err = %v, want nil", q.ClosedAt, err)
	}

	if err := s.ReopenFeature(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if f, err = s.Feature(ctx, 1); err != nil || f.ClosedAt != nil {
		t.Errorf("feature 1 closedAt after reopen = %v, err = %v, want nil", f.ClosedAt, err)
	}
	if c, err = s.Channel(ctx, 1); err != nil || c.ClosedAt == nil {
		t.Errorf("channel 1 closedAt after feature reopen = %v, err = %v, want still set", c.ClosedAt, err)
	}
	if err := s.CloseFeature(ctx, 99); !errors.Is(err, ErrNotFound) {
		t.Errorf("close feature 99 = %v, want ErrNotFound", err)
	}
}

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

func TestFeatureByRef(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	for _, p := range []Project{{Name: "a", Path: "/a"}, {Name: "b", Path: "/b"}} {
		if _, err := s.CreateProject(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []Feature{
		{ProjectID: 1, Slug: "login", Name: "Login"},
		{ProjectID: 2, Slug: "login", Name: "Login too"},
		{ProjectID: 2, Slug: "search", Name: "Search"},
	} {
		if _, err := s.CreateFeature(ctx, f); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		name    string
		give    string
		wantID  int64
		wantErr string
	}{
		{name: "id", give: "2", wantID: 2},
		{name: "slug", give: "search", wantID: 3},
		{name: "slug in two projects", give: "login",
			wantErr: `invalid: feature slug "login" is in projects 1, 2; use the feature id`},
		{name: "unknown slug", give: "nope", wantErr: `feature "nope": not found`},
		{name: "unknown id", give: "99", wantErr: "feature 99: not found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.FeatureByRef(ctx, tt.give)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Errorf("FeatureByRef(%q) error = %v, want %q", tt.give, err, tt.wantErr)
				}
				return
			}
			if err != nil || got.ID != tt.wantID {
				t.Errorf("FeatureByRef(%q) = %d, %v, want id %d", tt.give, got.ID, err, tt.wantID)
			}
		})
	}
}

func TestFeatureKind(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.CreateProject(ctx, Project{Name: "reef", Path: "/tmp/reef"}); err != nil {
		t.Fatal(err)
	}

	f, err := s.CreateFeature(ctx, Feature{ProjectID: 1, Slug: "a", Name: "A"})
	if err != nil || f.Kind != "journey" {
		t.Fatalf("CreateFeature without kind = %q, %v, want journey", f.Kind, err)
	}
	if _, err := s.CreateFeature(ctx, Feature{ProjectID: 1, Slug: "b", Name: "B", Kind: "epic"}); !errors.Is(err, ErrInvalid) {
		t.Errorf("CreateFeature kind epic = %v, want ErrInvalid", err)
	}
	g, err := s.CreateFeature(ctx, Feature{ProjectID: 1, Slug: "c", Name: "C", Kind: "fixes"})
	if err != nil || g.Kind != "fixes" {
		t.Fatalf("CreateFeature fixes = %q, %v, want fixes", g.Kind, err)
	}

	if got, err := s.UpdateFeature(ctx, Feature{ID: g.ID, Name: "C2"}); err != nil || got.Kind != "fixes" {
		t.Errorf("UpdateFeature without kind = %q, %v, want fixes kept", got.Kind, err)
	}
	if got, err := s.UpdateFeature(ctx, Feature{ID: g.ID, Name: "C2", Kind: "journey"}); err != nil || got.Kind != "journey" {
		t.Errorf("UpdateFeature journey = %q, %v, want journey", got.Kind, err)
	}
	if _, err := s.UpdateFeature(ctx, Feature{ID: g.ID, Name: "C2", Kind: "epic"}); !errors.Is(err, ErrInvalid) {
		t.Errorf("UpdateFeature kind epic = %v, want ErrInvalid", err)
	}
}
