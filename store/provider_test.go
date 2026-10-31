package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func providerFolder(t *testing.T, bins ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, bin := range bins {
		path := filepath.Join(dir, bin)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestStore_CreateProviderRejects(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()
	work := providerFolder(t, "claude")
	if _, err := st.CreateProvider(ctx, Provider{Name: "work", Kind: "claude", Folder: work}); err != nil {
		t.Fatal(err)
	}
	notExec := t.TempDir()
	if err := os.WriteFile(filepath.Join(notExec, "claude"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		give Provider
		want error
	}{
		{"bad kind", Provider{Name: "a", Kind: "gemini", Folder: work}, ErrInvalid},
		{"bad name", Provider{Name: "my work", Kind: "claude", Folder: work}, ErrInvalid},
		{"relative folder", Provider{Name: "a", Kind: "claude", Folder: "bin"}, ErrInvalid},
		{"missing folder", Provider{Name: "a", Kind: "claude", Folder: work + "/nope"}, ErrInvalid},
		{"no binary of the kind", Provider{Name: "a", Kind: "codex", Folder: work}, ErrInvalid},
		{"binary not executable", Provider{Name: "a", Kind: "claude", Folder: notExec}, ErrInvalid},
		{"duplicate name", Provider{Name: "work", Kind: "claude", Folder: work}, ErrConflict},
		{"name of a detected default", Provider{Name: "codex", Kind: "claude", Folder: work}, ErrConflict},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := st.CreateProvider(ctx, tt.give); !errors.Is(err, tt.want) {
				t.Errorf("CreateProvider(%+v) error = %v, want %v", tt.give, err, tt.want)
			}
		})
	}
}

func TestStore_ProviderBinAndDelete(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()
	dir := providerFolder(t, "bin/codex")
	p, err := st.CreateProvider(ctx, Provider{Name: "cx", Kind: "codex", Folder: dir})
	if err != nil {
		t.Fatal(err)
	}
	if bin, err := p.Bin(); err != nil || bin != filepath.Join(dir, "bin", "codex") {
		t.Errorf("Bin() = %q, %v, want %s/bin/codex", bin, err, dir)
	}
	lead := Role{Name: "t-lead", Lead: true, Backend: "codex", Provider: "cx"}
	if _, err := testApplyTeam(t, st, ctx, Team{Name: "t", Roles: []Role{lead}}); err != nil {
		t.Fatalf("ApplyTeam with a provider: %v", err)
	}
	got, err := st.Team(ctx, "t")
	if err != nil || got.Roles[0].Provider != "cx" {
		t.Errorf("Team roles = %+v, %v, want provider cx", got.Roles, err)
	}
	lead.Backend = "claude"
	if _, err := testApplyTeam(t, st, ctx, Team{Name: "t", Roles: []Role{lead}}); !errors.Is(err, ErrInvalid) {
		t.Errorf("ApplyTeam with a codex provider on a claude role error = %v, want ErrInvalid", err)
	}
	lead.Backend, lead.Provider = "codex", "gone"
	if _, err := testApplyTeam(t, st, ctx, Team{Name: "t", Roles: []Role{lead}}); !errors.Is(err, ErrInvalid) {
		t.Errorf("ApplyTeam with an unknown provider error = %v, want ErrInvalid", err)
	}
	err = st.DeleteProvider(ctx, "cx")
	if !errors.Is(err, ErrConflict) || !strings.Contains(err.Error(), "used by bot t-lead") {
		t.Errorf("DeleteProvider of a provider a role names error = %v, want ErrConflict naming bot t-lead", err)
	}
	if _, err := st.UpdateBot(ctx, Bot{ID: got.Roles[0].BotID, Name: "t-lead", Backend: "codex"}); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteProvider(ctx, "cx"); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteProvider(ctx, "cx"); !errors.Is(err, ErrNotFound) {
		t.Errorf("second DeleteProvider error = %v, want ErrNotFound", err)
	}
	if _, err := st.RoleBin(ctx, got.Roles[0]); !errors.Is(err, ErrInvalid) {
		t.Errorf("RoleBin of a removed provider error = %v, want ErrInvalid", err)
	}
}
