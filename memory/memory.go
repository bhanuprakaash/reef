// Package memory keeps Reef's memory items and the links between them.
package memory

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"strconv"
	"time"
)

const (
	KindDecision   = "decision"
	KindConvention = "convention"
	KindFinding    = "finding"
	KindFact       = "fact"
	KindLesson     = "lesson"
)

const (
	ScopeProject      = "project"
	ScopeWorkItem     = "work_item"
	ScopeTeam         = "team"
	ScopeTeammate     = "teammate"
	ScopeSpace        = "space"
	ScopeConversation = "conversation"
)

const (
	RelAbout    = "about"
	RelSource   = "source"
	RelReplaces = "replaces"
	RelRelated  = "related"
)

const (
	StatusCandidate = "candidate"
	StatusAccepted  = "accepted"
	StatusRetired   = "retired"
)

// ErrNotFound is wrapped when the id asked for matches no item.
var ErrNotFound = errors.New("not found")

//go:embed schema.sql
var _schema string

var (
	_kinds = map[string]bool{
		KindDecision: true, KindConvention: true, KindFinding: true, KindFact: true, KindLesson: true,
	}
	_scopeKinds = map[string]bool{
		ScopeProject: true, ScopeWorkItem: true, ScopeTeam: true,
		ScopeTeammate: true, ScopeSpace: true, ScopeConversation: true,
	}
	_rels = map[string]bool{RelAbout: true, RelSource: true, RelReplaces: true, RelRelated: true}
)

// Ref points at something by kind and id. Both are plain strings; nothing
// checks that the target exists.
type Ref struct{ Kind, ID string }

// Link is one edge from an item to a target.
type Link struct{ Rel, ToKind, ToID string }

// Writer is who is writing and what every item they write carries.
type Writer struct {
	Author, Role string
	Scope        Ref
	Links        []Link
}

// Note is what a writer passes to Remember.
type Note struct {
	Kind, Body, Why string
	About           []Ref
	Replaces        []int64
}

// Item is one stored memory with its links.
type Item struct {
	ID              int64
	Kind, Body, Why string
	Scope           Ref
	Author, Role    string
	Status          string
	RetiredAt       *time.Time
	ReplacedBy      *int64
	CreatedAt       time.Time
	Links           []Link
}

// Result is what Remember returns.
type Result struct{ Item Item }

// Store reads and writes memory items in a database it was given.
type Store struct {
	db *sql.DB
}

// Open creates the memory tables in db if they are missing.
func Open(db *sql.DB) (*Store, error) {
	if _, err := db.Exec(_schema); err != nil {
		return nil, fmt.Errorf("create memory tables: %w", err)
	}
	return &Store{db: db}, nil
}

// Remember writes one item with its links. Each id in n.Replaces must be a
// live item; it is retired and points at the new one. If any step fails,
// nothing is written.
func (s *Store) Remember(ctx context.Context, w Writer, n Note) (Result, error) {
	if err := validate(w, n); err != nil {
		return Result{}, err
	}
	links := make([]Link, 0, len(w.Links)+len(n.About)+len(n.Replaces))
	links = append(links, w.Links...)
	for _, r := range n.About {
		links = append(links, Link{Rel: RelAbout, ToKind: r.Kind, ToID: r.ID})
	}
	for _, id := range n.Replaces {
		links = append(links, Link{
			Rel: RelReplaces, ToKind: "memory", ToID: strconv.FormatInt(id, 10),
		})
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Result{}, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := tx.ExecContext(ctx,
		`INSERT INTO memory (kind, body, why, scope_kind, scope_id, author, role, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		n.Kind, n.Body, n.Why, w.Scope.Kind, w.Scope.ID, w.Author, w.Role,
		statusFor(w, n.Kind), now)
	if err != nil {
		return Result{}, fmt.Errorf("insert item: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Result{}, fmt.Errorf("item id: %w", err)
	}
	for _, l := range links {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO memory_link (from_id, rel, to_kind, to_id) VALUES (?, ?, ?, ?)`,
			id, l.Rel, l.ToKind, l.ToID); err != nil {
			return Result{}, fmt.Errorf("insert link: %w", err)
		}
	}
	for _, old := range n.Replaces {
		if err := retire(ctx, tx, old, &id, now); err != nil {
			return Result{}, fmt.Errorf("replace %d: %w", old, err)
		}
	}
	item, err := read(ctx, tx, id)
	if err != nil {
		return Result{}, err
	}
	if err := tx.Commit(); err != nil {
		return Result{}, fmt.Errorf("commit: %w", err)
	}
	return Result{Item: item}, nil
}

// Read returns the item with its links.
func (s *Store) Read(ctx context.Context, id int64) (Item, error) {
	return read(ctx, s.db, id)
}

// Retire marks a live item stale with nothing replacing it.
func (s *Store) Retire(ctx context.Context, id int64) error {
	return retire(ctx, s.db, id, nil, time.Now().UTC().Format(time.RFC3339Nano))
}

// Accept moves a candidate to accepted.
func (s *Store) Accept(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE memory SET status = ? WHERE id = ? AND status = ?`,
		StatusAccepted, id, StatusCandidate)
	if err != nil {
		return fmt.Errorf("accept %d: %w", id, err)
	}
	return checkChanged(ctx, s.db, res, id, "not a candidate")
}

// Pending returns the candidates waiting for the user, oldest first.
func (s *Store) Pending(ctx context.Context) ([]Item, error) {
	rows, err := s.db.QueryContext(ctx,
		_selectItem+` WHERE status = ? ORDER BY id`, StatusCandidate)
	if err != nil {
		return nil, fmt.Errorf("query pending: %w", err)
	}
	defer rows.Close()
	var items []Item
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("scan pending: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close pending: %w", err)
	}
	for i := range items {
		if items[i].Links, err = links(ctx, s.db, items[i].ID); err != nil {
			return nil, err
		}
	}
	return items, nil
}

// _acceptedRoles lists the roles whose writes of a kind are accepted. A kind
// with no entry is accepted from any role.
var _acceptedRoles = map[string]map[string]bool{
	KindConvention: {"architect": true, "lead": true},
}

// statusFor is the status a write gets. The user is always accepted; anyone
// else is a candidate in team scope or when their role may not write the kind.
func statusFor(w Writer, kind string) string {
	if w.Author == "user" {
		return StatusAccepted
	}
	if w.Scope.Kind == ScopeTeam {
		return StatusCandidate
	}
	if roles := _acceptedRoles[kind]; roles != nil && !roles[w.Role] {
		return StatusCandidate
	}
	return StatusAccepted
}

func validate(w Writer, n Note) error {
	if !_kinds[n.Kind] {
		return fmt.Errorf("invalid kind %q", n.Kind)
	}
	if n.Body == "" {
		return errors.New("empty body")
	}
	if !_scopeKinds[w.Scope.Kind] {
		return fmt.Errorf("invalid scope kind %q", w.Scope.Kind)
	}
	for _, l := range w.Links {
		if !_rels[l.Rel] {
			return fmt.Errorf("invalid link rel %q", l.Rel)
		}
	}
	return nil
}

type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

const _selectItem = `SELECT id, kind, body, why, scope_kind, scope_id, author, role, status,
	retired_at, replaced_by, created_at FROM memory`

// retire marks a live item retired. replacedBy is nil when nothing replaces it.
func retire(ctx context.Context, q querier, id int64, replacedBy *int64, now string) error {
	res, err := q.ExecContext(ctx,
		`UPDATE memory SET status = ?, retired_at = ?, replaced_by = ?
		 WHERE id = ? AND status != ?`,
		StatusRetired, now, replacedBy, id, StatusRetired)
	if err != nil {
		return fmt.Errorf("retire %d: %w", id, err)
	}
	return checkChanged(ctx, q, res, id, "already retired")
}

// checkChanged turns an update that touched no row into ErrNotFound when the
// id is missing, or into reason when the item exists but was in the wrong state.
func checkChanged(ctx context.Context, q querier, res sql.Result, id int64, reason string) error {
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n > 0 {
		return nil
	}
	var one int
	err = q.QueryRowContext(ctx, `SELECT 1 FROM memory WHERE id = ?`, id).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("memory %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return fmt.Errorf("check %d: %w", id, err)
	}
	return fmt.Errorf("memory %d: %s", id, reason)
}

func read(ctx context.Context, q querier, id int64) (Item, error) {
	rows, err := q.QueryContext(ctx, _selectItem+` WHERE id = ?`, id)
	if err != nil {
		return Item{}, fmt.Errorf("query item %d: %w", id, err)
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return Item{}, fmt.Errorf("query item %d: %w", id, err)
		}
		return Item{}, fmt.Errorf("memory %d: %w", id, ErrNotFound)
	}
	it, err := scanItem(rows)
	if err != nil {
		return Item{}, err
	}
	if err := rows.Close(); err != nil {
		return Item{}, fmt.Errorf("close item %d: %w", id, err)
	}
	if it.Links, err = links(ctx, q, id); err != nil {
		return Item{}, err
	}
	return it, nil
}

func links(ctx context.Context, q querier, id int64) ([]Link, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT rel, to_kind, to_id FROM memory_link WHERE from_id = ? ORDER BY rowid`, id)
	if err != nil {
		return nil, fmt.Errorf("query links of %d: %w", id, err)
	}
	defer rows.Close()
	var out []Link
	for rows.Next() {
		var l Link
		if err := rows.Scan(&l.Rel, &l.ToKind, &l.ToID); err != nil {
			return nil, fmt.Errorf("scan link: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func scanItem(rows *sql.Rows) (Item, error) {
	var (
		it         Item
		retiredAt  sql.NullString
		replacedBy sql.NullInt64
		createdAt  string
	)
	if err := rows.Scan(&it.ID, &it.Kind, &it.Body, &it.Why, &it.Scope.Kind, &it.Scope.ID,
		&it.Author, &it.Role, &it.Status, &retiredAt, &replacedBy, &createdAt); err != nil {
		return Item{}, fmt.Errorf("scan item: %w", err)
	}
	var err error
	if it.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
		return Item{}, fmt.Errorf("parse created_at: %w", err)
	}
	if retiredAt.Valid {
		t, err := time.Parse(time.RFC3339Nano, retiredAt.String)
		if err != nil {
			return Item{}, fmt.Errorf("parse retired_at: %w", err)
		}
		it.RetiredAt = &t
	}
	if replacedBy.Valid {
		it.ReplacedBy = &replacedBy.Int64
	}
	return it, nil
}
