package store

import (
	"context"
	"errors"
	"testing"
)

func TestDeleteToken(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.CreateProject(ctx, Project{Name: "shop", Path: "/tmp/shop"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateChannel(ctx, Channel{ProjectID: 1, Name: "chat"}); err != nil {
		t.Fatal(err)
	}
	tok, err := s.CreateToken(ctx, 1, "architect")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := s.Token(ctx, tok.Token); err != nil || got.Role != "architect" {
		t.Fatalf("Token before delete = %+v, %v, want role architect", got, err)
	}

	if err := s.DeleteToken(ctx, tok.Token); err != nil {
		t.Fatalf("DeleteToken = %v, want nil", err)
	}
	if _, err := s.Token(ctx, tok.Token); !errors.Is(err, ErrNotFound) {
		t.Errorf("Token after delete = %v, want ErrNotFound", err)
	}
	if err := s.DeleteToken(ctx, tok.Token); !errors.Is(err, ErrNotFound) {
		t.Errorf("second DeleteToken = %v, want ErrNotFound", err)
	}
}
