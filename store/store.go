// Package store keeps Reef's data in one SQLite file.
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
	ErrInvalid  = errors.New("invalid")
)

//go:embed migrations/*.sql
var _migrations embed.FS

const _dsnParams = "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)"

type Store struct {
	db   *sql.DB
	lock *os.File
}

// Open opens dir/reef.db, creating dir and the file if needed, and applies
// any migration newer than the file's user_version. It holds a lock on the
// folder until Close, so a second Open of the same dir fails.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	lock, err := os.OpenFile(filepath.Join(dir, "reef.lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = lock.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf("data folder %s is in use by another reef", dir)
		}
		return nil, fmt.Errorf("lock data folder: %w", err)
	}
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "reef.db")+_dsnParams)
	if err != nil {
		_ = lock.Close()
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err := migrate(context.Background(), db); err != nil {
		_ = db.Close()
		_ = lock.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Store{db: db, lock: lock}, nil
}

// Close closes the database and releases the folder lock.
func (s *Store) Close() error {
	err := s.db.Close()
	if lerr := s.lock.Close(); err == nil {
		err = lerr
	}
	return err
}

func isUnique(err error) bool {
	var se *sqlite.Error
	return errors.As(err, &se) && se.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE
}

func isForeignKey(err error) bool {
	var se *sqlite.Error
	return errors.As(err, &se) && se.Code() == sqlite3.SQLITE_CONSTRAINT_FOREIGNKEY
}

// exists reports whether table has a row with the id. table is always a
// package constant, never caller input.
func (s *Store) exists(ctx context.Context, table string, id int64) (bool, error) {
	var ok bool
	err := s.db.QueryRowContext(ctx,
		"SELECT EXISTS (SELECT 1 FROM "+table+" WHERE id = ?)", id).Scan(&ok)
	return ok, err
}

func migrate(ctx context.Context, db *sql.DB) error {
	var version int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("read user_version: %w", err)
	}
	files, err := _migrations.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("list migrations: %w", err)
	}
	names := make([]string, 0, len(files))
	for _, f := range files {
		names = append(names, f.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		n, err := strconv.Atoi(strings.SplitN(name, "_", 2)[0])
		if err != nil {
			return fmt.Errorf("migration %s: bad number: %w", name, err)
		}
		if n <= version {
			continue
		}
		body, err := _migrations.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		if err := apply(ctx, db, n, string(body)); err != nil {
			return fmt.Errorf("apply %s: %w", name, err)
		}
	}
	return nil
}

func apply(ctx context.Context, db *sql.DB, version int, body string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, body); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "PRAGMA user_version = "+strconv.Itoa(version)); err != nil {
		return err
	}
	return tx.Commit()
}
