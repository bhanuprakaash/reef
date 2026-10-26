// Package memory keeps Reef's memory items and the links between them.
package memory

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
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
	StatusRejected  = "rejected"
)

const _selectItem = `SELECT id, kind, body, why, scope_kind, scope_id, author, role, status,
	retired_at, replaced_by, created_at FROM memory`

var (
	// ErrNotFound is wrapped when the id matches no item.
	ErrNotFound = errors.New("not found")
	// ErrInvalid is wrapped for bad input or an item in the wrong state.
	ErrInvalid = errors.New("invalid")
)

//go:embed migrations/*.sql
var _migrations embed.FS

var (
	_kinds = map[string]bool{
		KindDecision: true, KindConvention: true, KindFinding: true, KindFact: true, KindLesson: true,
	}
	_scopeKinds = map[string]bool{
		ScopeProject: true, ScopeWorkItem: true, ScopeTeam: true,
		ScopeTeammate: true, ScopeSpace: true, ScopeConversation: true,
	}
	_rels = map[string]bool{RelAbout: true, RelSource: true, RelReplaces: true, RelRelated: true}
	// A kind with no entry is accepted from any role.
	_acceptedRoles = map[string]map[string]bool{
		KindConvention: {"architect": true, "lead": true},
	}
)

// Ref points at a target by kind and id; nothing checks that it exists.
type Ref struct{ Kind, ID string }

// Link is one edge from an item to a target.
type Link struct{ Rel, ToKind, ToID string }

// Writer is who is writing and what every item they write carries.
type Writer struct {
	Author, Role string
	Scope        Ref
	Links        []Link
}

// Note is what a writer passes to Remember. NeedsApproval makes the item a candidate
// whoever writes it.
type Note struct {
	Kind, Body, Why string
	About           []Ref
	Replaces        []int64
	NeedsApproval   bool
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

// Result is what Remember returns. Similar is nil when nothing looks like the new item.
// EmbedErr is why no vector was stored; it is nil when one was stored or no embedder is set.
type Result struct {
	Item     Item
	Similar  []Similar
	EmbedErr error
}

// Store reads and writes memory items in a database it was given.
type Store struct {
	db    *sql.DB
	embed Embedder
}

type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Open brings the memory tables in db up to date. A nil e stores no vectors.
func Open(db *sql.DB, e Embedder) (*Store, error) {
	if err := migrate(context.Background(), db); err != nil {
		return nil, fmt.Errorf("migrate memory tables: %w", err)
	}
	if e == nil {
		e = noneEmbedder{}
	}
	return &Store{db: db, embed: e}, nil
}

// Remember writes one item with its links, all or nothing. Each id in n.Replaces must be a
// live item. When the new item is accepted they are retired and point at it; when it is a
// candidate they stay live until Accept. Lookalikes are reported in Result.Similar but
// never block the write.
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

	vec, embedErr := s.embed.Embed(ctx, embedText(n.Body, n.Why))
	var blob []byte
	if vec != nil {
		blob = encodeVector(vec)
	}
	similar, err := s.lookalikes(ctx, w, n, vec)
	if err != nil {
		return Result{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Result{}, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	status := statusFor(w, n)
	res, err := tx.ExecContext(ctx,
		`INSERT INTO memory (kind, body, why, scope_kind, scope_id, author, role, status, created_at, vector)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		n.Kind, n.Body, n.Why, w.Scope.Kind, w.Scope.ID, w.Author, w.Role, status, now, blob)
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
		if status == StatusCandidate {
			err = checkLive(ctx, tx, old)
		} else {
			err = retire(ctx, tx, old, &id, now)
		}
		if err != nil {
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
	return Result{Item: item, Similar: similar, EmbedErr: embedErr}, nil
}

// Read returns the item with its links.
func (s *Store) Read(ctx context.Context, id int64) (Item, error) {
	return read(ctx, s.db, id)
}

// Reject turns down a candidate. It is kept, marked rejected, and is never found by a search.
func (s *Store) Reject(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE memory SET status = ?, retired_at = ? WHERE id = ? AND status = ?`,
		StatusRejected, time.Now().UTC().Format(time.RFC3339Nano), id, StatusCandidate)
	if err != nil {
		return fmt.Errorf("reject %d: %w", id, err)
	}
	return checkChanged(ctx, s.db, res, id, "not a candidate")
}

// Retire marks an accepted item stale with nothing replacing it.
func (s *Store) Retire(ctx context.Context, id int64) error {
	return retire(ctx, s.db, id, nil, time.Now().UTC().Format(time.RFC3339Nano))
}

// Accept moves a candidate to accepted and retires the items it replaces, skipping any
// already retired.
func (s *Store) Accept(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx,
		`UPDATE memory SET status = ? WHERE id = ? AND status = ?`,
		StatusAccepted, id, StatusCandidate)
	if err != nil {
		return fmt.Errorf("accept %d: %w", id, err)
	}
	if err := checkChanged(ctx, tx, res, id, "not a candidate"); err != nil {
		return err
	}
	old, err := replacesOf(ctx, tx, id)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, o := range old {
		if err := retire(ctx, tx, o, &id, now); err != nil && !errors.Is(err, ErrInvalid) {
			return fmt.Errorf("replace %d: %w", o, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// Scopes returns every scope that holds an item of any status.
func (s *Store) Scopes(ctx context.Context) ([]Ref, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT scope_kind, scope_id FROM memory ORDER BY scope_kind, scope_id`)
	if err != nil {
		return nil, fmt.Errorf("query scopes: %w", err)
	}
	defer rows.Close()
	var out []Ref
	for rows.Next() {
		var r Ref
		if err := rows.Scan(&r.Kind, &r.ID); err != nil {
			return nil, fmt.Errorf("scan scope: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
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

func statusFor(w Writer, n Note) string {
	if n.NeedsApproval {
		return StatusCandidate
	}
	if w.Author == "user" {
		return StatusAccepted
	}
	if w.Scope.Kind == ScopeTeam {
		return StatusCandidate
	}
	if roles := _acceptedRoles[n.Kind]; roles != nil && !roles[w.Role] {
		return StatusCandidate
	}
	return StatusAccepted
}

func validate(w Writer, n Note) error {
	if !_kinds[n.Kind] {
		return fmt.Errorf("%w: unknown kind %q", ErrInvalid, n.Kind)
	}
	if n.Body == "" {
		return fmt.Errorf("%w: body is required", ErrInvalid)
	}
	if !_scopeKinds[w.Scope.Kind] {
		return fmt.Errorf("%w: unknown scope kind %q", ErrInvalid, w.Scope.Kind)
	}
	for _, l := range w.Links {
		if !_rels[l.Rel] {
			return fmt.Errorf("%w: unknown link rel %q", ErrInvalid, l.Rel)
		}
	}
	for i, id := range n.Replaces {
		if slices.Contains(n.Replaces[:i], id) {
			return fmt.Errorf("%w: memory %d listed twice in replaces", ErrInvalid, id)
		}
	}
	return nil
}

func retire(ctx context.Context, q querier, id int64, replacedBy *int64, now string) error {
	res, err := q.ExecContext(ctx,
		`UPDATE memory SET status = ?, retired_at = ?, replaced_by = ?
		 WHERE id = ? AND status = ?`,
		StatusRetired, now, replacedBy, id, StatusAccepted)
	if err != nil {
		return fmt.Errorf("retire %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n > 0 {
		return nil
	}
	return checkLive(ctx, q, id)
}

// checkLive fails unless id is an accepted item.
func checkLive(ctx context.Context, q querier, id int64) error {
	var status string
	err := q.QueryRowContext(ctx, `SELECT status FROM memory WHERE id = ?`, id).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("memory %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return fmt.Errorf("check %d: %w", id, err)
	}
	switch status {
	case StatusCandidate:
		return fmt.Errorf("%w: memory %d is a candidate", ErrInvalid, id)
	case StatusRetired:
		return fmt.Errorf("%w: memory %d is already retired", ErrInvalid, id)
	case StatusRejected:
		return fmt.Errorf("%w: memory %d is rejected", ErrInvalid, id)
	}
	return nil
}

func replacesOf(ctx context.Context, q querier, id int64) ([]int64, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT CAST(to_id AS INTEGER) FROM memory_link
		 WHERE from_id = ? AND rel = ? AND to_kind = 'memory' ORDER BY rowid`, id, RelReplaces)
	if err != nil {
		return nil, fmt.Errorf("query replaces of %d: %w", id, err)
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var old int64
		if err := rows.Scan(&old); err != nil {
			return nil, fmt.Errorf("scan replaced: %w", err)
		}
		out = append(out, old)
	}
	return out, rows.Err()
}

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
	return fmt.Errorf("%w: memory %d is %s", ErrInvalid, id, reason)
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

func migrate(ctx context.Context, db *sql.DB) error {
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

// apply runs one migration unless the tables are already at version or later. Tables made
// before memory_version existed are at version 1.
func apply(ctx context.Context, db *sql.DB, version int, body string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var hasVersion, hasItems bool
	if err := tx.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM sqlite_master WHERE name = 'memory_version'),
		        EXISTS (SELECT 1 FROM sqlite_master WHERE name = 'memory')`).Scan(&hasVersion, &hasItems); err != nil {
		return fmt.Errorf("read version: %w", err)
	}
	var current int
	switch {
	case hasVersion:
		if err := tx.QueryRowContext(ctx, `SELECT version FROM memory_version`).Scan(&current); err != nil {
			return fmt.Errorf("read version: %w", err)
		}
	case hasItems:
		current = 1
	}
	if current >= version {
		return nil
	}
	if _, err := tx.ExecContext(ctx, body); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`CREATE TABLE IF NOT EXISTS memory_version (version INTEGER NOT NULL)`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM memory_version`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO memory_version (version) VALUES (?)`, version); err != nil {
		return err
	}
	return tx.Commit()
}
