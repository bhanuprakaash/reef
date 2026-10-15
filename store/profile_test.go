package store

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestProfile(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	if _, err := s.Profile(ctx); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Profile before a name = %v, want ErrNotFound", err)
	}
	if _, err := s.PutProfile(ctx, Profile{Name: "  "}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("PutProfile blank = %v, want ErrInvalid", err)
	}

	if _, err := s.PutProfile(ctx, Profile{Name: "  Ada Lovelace "}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PutProfile(ctx, Profile{Name: "Ada"}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Profile(ctx)
	if err != nil || got.Name != "Ada" || got.Role != "" || !reflect.DeepEqual(got.Style, []string{}) {
		t.Fatalf("Profile = %+v, %v, want Ada with no role or style", got, err)
	}

	in := Profile{Name: "Ada", Role: " Developer ", Style: []string{" Short answers", "", "Tests first "}}
	if _, err := s.PutProfile(ctx, in); err != nil {
		t.Fatal(err)
	}
	got, err = s.Profile(ctx)
	if err != nil || got.Role != "Developer" || !reflect.DeepEqual(got.Style, []string{"Short answers", "Tests first"}) {
		t.Fatalf("Profile = %+v, %v, want role Developer and two style entries", got, err)
	}
	var rows int
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM profile").Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("profile rows = %d, %v, want 1", rows, err)
	}
}
