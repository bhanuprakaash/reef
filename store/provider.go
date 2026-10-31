package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Provider is a claude or codex install the user added next to the ones found on PATH. A role
// names it in Role.Provider to run that install's binary.
type Provider struct {
	Name      string    `json:"name"`
	Kind      string    `json:"kind"`
	Folder    string    `json:"folder"`
	CreatedAt time.Time `json:"createdAt"`
}

// Bin is the executable of the provider's kind in its folder: FOLDER/KIND or FOLDER/bin/KIND.
func (p Provider) Bin() (string, error) {
	for _, path := range []string{
		filepath.Join(p.Folder, p.Kind), filepath.Join(p.Folder, "bin", p.Kind),
	} {
		if fi, err := os.Stat(path); err == nil && fi.Mode().IsRegular() && fi.Mode().Perm()&0o111 != 0 {
			return path, nil
		}
	}
	return "", fmt.Errorf("%w: no %s binary in %s", ErrInvalid, p.Kind, p.Folder)
}

// CreateProvider adds p. Its name is unique, and neither claude nor codex: those are the
// providers found on PATH.
func (s *Store) CreateProvider(ctx context.Context, p Provider) (Provider, error) {
	switch {
	case !_roleName.MatchString(p.Name):
		return Provider{}, fmt.Errorf("%w: provider name %q must be letters, digits, _ or -",
			ErrInvalid, p.Name)
	case isBackend(p.Name):
		return Provider{}, fmt.Errorf("%w: provider name %q is taken by the one found on PATH",
			ErrConflict, p.Name)
	case !isBackend(p.Kind):
		return Provider{}, fmt.Errorf("%w: provider kind must be claude or codex", ErrInvalid)
	case !filepath.IsAbs(p.Folder):
		return Provider{}, fmt.Errorf("%w: provider folder %q: not an absolute path",
			ErrInvalid, p.Folder)
	}
	if fi, err := os.Stat(p.Folder); err != nil || !fi.IsDir() {
		return Provider{}, fmt.Errorf("%w: provider folder %q: not a directory", ErrInvalid, p.Folder)
	}
	if _, err := p.Bin(); err != nil {
		return Provider{}, err
	}
	now := time.Now().UTC()
	if _, err := s.db.ExecContext(ctx,
		"INSERT INTO provider (name, kind, folder, created_at) VALUES (?, ?, ?, ?)",
		p.Name, p.Kind, p.Folder, now.Format(time.RFC3339Nano)); err != nil {
		if isUnique(err) {
			return Provider{}, fmt.Errorf("%w: a provider named %q already exists", ErrConflict, p.Name)
		}
		return Provider{}, fmt.Errorf("insert provider: %w", err)
	}
	p.CreatedAt = now
	return p, nil
}

// Provider returns the added provider with the name, or ErrNotFound.
func (s *Store) Provider(ctx context.Context, name string) (Provider, error) {
	p, err := scanProvider(s.db.QueryRowContext(ctx,
		"SELECT name, kind, folder, created_at FROM provider WHERE name = ?", name))
	if errors.Is(err, sql.ErrNoRows) {
		return Provider{}, fmt.Errorf("provider %q: %w", name, ErrNotFound)
	}
	return p, err
}

// Providers returns the added providers, oldest first.
func (s *Store) Providers(ctx context.Context) ([]Provider, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT name, kind, folder, created_at FROM provider ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("list providers: %w", err)
	}
	return scanAll(rows, scanProvider)
}

// RoleBin is the binary the role runs: the one of the added provider it names, which must still
// exist, match the role's backend and hold a binary. Empty when the role names none.
func (s *Store) RoleBin(ctx context.Context, r Role) (string, error) {
	if r.Provider == "" {
		return "", nil
	}
	p, err := s.Provider(ctx, r.Provider)
	switch {
	case errors.Is(err, ErrNotFound):
		return "", fmt.Errorf("%w: role %s: provider %q is not installed",
			ErrInvalid, r.Name, r.Provider)
	case err != nil:
		return "", err
	case p.Kind != r.Backend:
		return "", fmt.Errorf("%w: role %s: provider %q is a %s install, the role runs on %s",
			ErrInvalid, r.Name, r.Provider, p.Kind, r.Backend)
	}
	return p.Bin()
}

// DeleteProvider removes the added provider with the name, or fails with ErrNotFound. It fails
// with ErrConflict while a Bot names the provider.
func (s *Store) DeleteProvider(ctx context.Context, name string) error {
	var bot string
	err := s.db.QueryRowContext(ctx, "SELECT name FROM bot WHERE provider = ?", name).
		Scan(&bot)
	if err == nil {
		return fmt.Errorf("%w: provider %q is used by bot %s; change that bot first",
			ErrConflict, name, bot)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("delete provider: %w", err)
	}
	res, err := s.db.ExecContext(ctx, "DELETE FROM provider WHERE name = ?", name)
	if err != nil {
		return fmt.Errorf("delete provider: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return fmt.Errorf("provider %q: %w", name, ErrNotFound)
	}
	return nil
}

func scanProvider(r interface{ Scan(...any) error }) (Provider, error) {
	var p Provider
	var created string
	if err := r.Scan(&p.Name, &p.Kind, &p.Folder, &created); err != nil {
		return Provider{}, err
	}
	var err error
	if p.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return Provider{}, fmt.Errorf("provider %s: bad created_at: %w", p.Name, err)
	}
	return p, nil
}
