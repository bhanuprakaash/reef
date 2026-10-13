// Package store keeps Reef's data in one SQLite file.
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

var (
	// ErrNotFound is wrapped when the id asked for matches no row.
	ErrNotFound = errors.New("not found")
	// ErrConflict is wrapped when a unique rule would break.
	ErrConflict = errors.New("conflict")
	// ErrInvalid is wrapped when the caller's input is wrong, such as a bad
	// field or an unknown parent id.
	ErrInvalid = errors.New("invalid")
)

//go:embed migrations/*.sql
var _migrations embed.FS

const _walTimeout = 5 * time.Second

const _dsnParams = "_pragma=foreign_keys(ON)" +
	"&_pragma=busy_timeout(5000)&_txlock=immediate"

// Store is Reef's data, read and written through one SQLite file.
type Store struct {
	db *sql.DB
}

// parent is a row a write points at. A nil id is a parent the write leaves unset.
type parent struct {
	table string
	id    *int64
}

// Open opens dir/reef.db, creating dir and the file if needed, and applies
// any migration newer than the file's user_version. Several processes may
// open the same dir at once.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve data dir: %w", err)
	}
	uri := url.URL{Scheme: "file", Path: filepath.Join(abs, "reef.db"), RawQuery: _dsnParams}
	db, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	ctx := context.Background()
	if err := enableWAL(ctx, db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable WAL: %w", err)
	}
	if err := migrate(ctx, db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Store{db: db}, nil
}

// Close closes this process's handle on the database file.
func (s *Store) Close() error {
	return s.db.Close()
}

// writeErr is the error to return when the INSERT or UPDATE op failed with err.
// SQLite does not say which foreign key broke, so a foreign-key failure looks up
// the parents and names the first one that is gone: kind ErrNotFound for a parent
// named in the URL, ErrInvalid for one named in the body.
func (s *Store) writeErr(
	ctx context.Context, op string, err error, kind error, parents ...parent,
) error {
	if !isForeignKey(err) {
		return fmt.Errorf("%s: %w", op, err)
	}
	for _, p := range parents {
		if p.id == nil {
			continue
		}
		ok, qerr := s.exists(ctx, p.table, *p.id)
		if qerr != nil {
			return fmt.Errorf("%s: check %s: %w", op, p.table, qerr)
		}
		if ok {
			continue
		}
		if kind == ErrNotFound {
			return fmt.Errorf("%s %d: %w", p.table, *p.id, ErrNotFound)
		}
		return fmt.Errorf("%w: unknown %s %d", kind, p.table, *p.id)
	}
	return fmt.Errorf("%s: %w", op, err)
}

// exists reports whether table has a row with the id. table is always a
// package constant, never caller input.
func (s *Store) exists(ctx context.Context, table string, id int64) (bool, error) {
	var ok bool
	err := s.db.QueryRowContext(ctx,
		"SELECT EXISTS (SELECT 1 FROM "+table+" WHERE id = ?)", id).Scan(&ok)
	return ok, err
}

func (s *Store) requireFeature(ctx context.Context, id int64) error {
	ok, err := s.exists(ctx, "feature", id)
	if err != nil {
		return fmt.Errorf("check feature: %w", err)
	}
	if !ok {
		return fmt.Errorf("feature %d: %w", id, ErrNotFound)
	}
	return nil
}

func (s *Store) requireChannel(ctx context.Context, id int64) error {
	ok, err := s.exists(ctx, "channel", id)
	if err != nil {
		return fmt.Errorf("check channel: %w", err)
	}
	if !ok {
		return fmt.Errorf("channel %d: %w", id, ErrNotFound)
	}
	return nil
}

func (s *Store) checkFeatureInProject(
	ctx context.Context, featureID *int64, projectID int64,
) error {
	if featureID == nil {
		return nil
	}
	f, err := s.Feature(ctx, *featureID)
	if errors.Is(err, ErrNotFound) {
		return fmt.Errorf("%w: unknown feature %d", ErrInvalid, *featureID)
	}
	if err != nil {
		return fmt.Errorf("check feature: %w", err)
	}
	if f.ProjectID != projectID {
		return fmt.Errorf("%w: feature %d is not in project %d", ErrInvalid, f.ID, projectID)
	}
	return nil
}

func isUnique(err error) bool {
	var se *sqlite.Error
	return errors.As(err, &se) && se.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE
}

func isForeignKey(err error) bool {
	var se *sqlite.Error
	return errors.As(err, &se) && se.Code() == sqlite3.SQLITE_CONSTRAINT_FOREIGNKEY
}

// enableWAL switches the file to WAL mode, which sticks to the file. SQLite
// fails a switch at once, ignoring busy_timeout, when another process is
// switching the same fresh file, so it retries until the other one is done.
func enableWAL(ctx context.Context, db *sql.DB) error {
	deadline := time.Now().Add(_walTimeout)
	for {
		_, err := db.ExecContext(ctx, "PRAGMA journal_mode=WAL")
		var se *sqlite.Error
		if err == nil || !errors.As(err, &se) || se.Code()&0xff != sqlite3.SQLITE_BUSY ||
			time.Now().After(deadline) {
			return err
		}
		time.Sleep(10 * time.Millisecond)
	}
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

	var newest int
	for _, name := range names {
		n, err := strconv.Atoi(strings.SplitN(name, "_", 2)[0])
		if err != nil {
			return fmt.Errorf("migration %s: bad number: %w", name, err)
		}
		newest = n
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
	if version > newest {
		return fmt.Errorf("database is newer than this reef (version %d, this reef knows %d)",
			version, newest)
	}
	return nil
}

func apply(ctx context.Context, db *sql.DB, version int, body string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var current int
	if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&current); err != nil {
		return fmt.Errorf("read user_version: %w", err)
	}
	if current >= version {
		return nil
	}
	if _, err := tx.ExecContext(ctx, body); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		"PRAGMA user_version = "+strconv.Itoa(version)); err != nil {
		return err
	}
	return tx.Commit()
}

func scanAll[T any](
	rows *sql.Rows, scan func(interface{ Scan(...any) error }) (T, error),
) ([]T, error) {
	var out []T
	for rows.Next() {
		v, err := scan(rows)
		if err != nil {
			_ = rows.Close()
			return nil, err
		}
		out = append(out, v)
	}
	err := rows.Err()
	if cerr := rows.Close(); err == nil {
		err = cerr
	}
	return out, err
}
