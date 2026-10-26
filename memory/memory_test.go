package memory

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func openTest(t *testing.T) (*Store, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	s, err := Open(db, nil)
	if err != nil {
		t.Fatal(err)
	}
	return s, db
}

func writer() Writer {
	return Writer{
		Author: "ada", Role: "developer",
		Scope: Ref{Kind: ScopeWorkItem, ID: "7"},
		Links: []Link{{Rel: RelSource, ToKind: "message", ToID: "42"}},
	}
}

func count(t *testing.T, db *sql.DB, query string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestOpen_Idempotent(t *testing.T) {
	s, db := openTest(t)
	if _, err := Open(db, nil); err != nil {
		t.Fatalf("second Open: %v", err)
	}
	if n := count(t, db, `SELECT count(*) FROM sqlite_master WHERE name IN ('memory', 'memory_link')`); n != 2 {
		t.Fatalf("tables = %d, want 2", n)
	}
	if _, err := s.Remember(context.Background(), writer(), Note{Kind: KindFact, Body: "x"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(db, nil); err != nil {
		t.Fatalf("Open with rows: %v", err)
	}
	if n := count(t, db, `SELECT count(*) FROM memory`); n != 1 {
		t.Fatalf("rows after reopen = %d, want 1", n)
	}
}

func TestSchema_NoForeignKeys(t *testing.T) {
	files, err := _migrations.ReadDir("migrations")
	if err != nil || len(files) == 0 {
		t.Fatalf("migrations: %v, %d files", err, len(files))
	}
	for _, f := range files {
		body, err := _migrations.ReadFile("migrations/" + f.Name())
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(strings.ToLower(string(body)), "references") {
			t.Fatalf("%s has REFERENCES", f.Name())
		}
	}
}

func TestRemember_WritesItemAndLinks(t *testing.T) {
	s, _ := openTest(t)
	ctx := context.Background()
	res, err := s.Remember(ctx, writer(), Note{
		Kind: KindDecision, Body: "use sqlite", Why: "one file",
		About: []Ref{{Kind: "file", ID: "store/store.go"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Read(ctx, res.Item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != KindDecision || got.Body != "use sqlite" || got.Why != "one file" ||
		got.Author != "ada" || got.Role != "developer" || got.Status != StatusAccepted ||
		got.Scope != (Ref{Kind: ScopeWorkItem, ID: "7"}) || got.CreatedAt.IsZero() ||
		got.RetiredAt != nil || got.ReplacedBy != nil {
		t.Fatalf("item = %+v", got)
	}
	want := []Link{
		{Rel: RelSource, ToKind: "message", ToID: "42"},
		{Rel: RelAbout, ToKind: "file", ToID: "store/store.go"},
	}
	if !reflect.DeepEqual(got.Links, want) {
		t.Fatalf("links = %+v, want %+v", got.Links, want)
	}
}

func TestRemember_Status(t *testing.T) {
	project := Ref{Kind: ScopeProject, ID: "1"}
	team := Ref{Kind: ScopeTeam, ID: "1"}
	tests := []struct {
		name  string
		give  Writer
		kind  string
		needs bool
		want  string
	}{
		{"convention by developer", Writer{Author: "ada", Role: "developer", Scope: project}, KindConvention, false, "candidate"},
		{"convention by architect", Writer{Author: "bo", Role: "architect", Scope: project}, KindConvention, false, "accepted"},
		{"convention by lead", Writer{Author: "di", Role: "lead", Scope: project}, KindConvention, false, "accepted"},
		{"convention by user", Writer{Author: "user", Scope: project}, KindConvention, false, "accepted"},
		{"decision by developer", Writer{Author: "ada", Role: "developer", Scope: project}, KindDecision, false, "accepted"},
		{"decision by tester", Writer{Author: "cy", Role: "tester", Scope: project}, KindDecision, false, "accepted"},
		{"finding by tester", Writer{Author: "cy", Role: "tester", Scope: project}, KindFinding, false, "accepted"},
		{"lesson in team scope by lead", Writer{Author: "di", Role: "lead", Scope: team}, KindLesson, false, "candidate"},
		{"lesson in project scope by developer", Writer{Author: "ada", Role: "developer", Scope: project}, KindLesson, false, "accepted"},
		{"convention by architect asking approval", Writer{Author: "bo", Role: "architect", Scope: project}, KindConvention, true, "candidate"},
		{"decision by developer asking approval", Writer{Author: "ada", Role: "developer", Scope: project}, KindDecision, true, "candidate"},
		{"fact by user asking approval", Writer{Author: "user", Scope: project}, KindFact, true, "candidate"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, _ := openTest(t)
			res, err := s.Remember(context.Background(), tt.give,
				Note{Kind: tt.kind, Body: "x", NeedsApproval: tt.needs})
			if err != nil {
				t.Fatal(err)
			}
			if res.Item.Status != tt.want {
				t.Fatalf("status = %q, want %q", res.Item.Status, tt.want)
			}
		})
	}
}

func TestRemember_Invalid(t *testing.T) {
	s, db := openTest(t)
	tests := []struct {
		name string
		w    Writer
		n    Note
	}{
		{"kind", writer(), Note{Kind: "idea", Body: "x"}},
		{"body", writer(), Note{Kind: KindFact}},
		{"scope kind", Writer{Scope: Ref{Kind: "planet"}}, Note{Kind: KindFact, Body: "x"}},
		{"rel", Writer{Scope: Ref{Kind: ScopeProject}, Links: []Link{{Rel: "likes"}}}, Note{Kind: KindFact, Body: "x"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := s.Remember(context.Background(), tt.w, tt.n); err == nil {
				t.Fatal("want error")
			}
			if n := count(t, db, `SELECT count(*) FROM memory`); n != 0 {
				t.Fatalf("rows = %d, want 0", n)
			}
		})
	}
}

func TestRemember_ReplacesRetiresOld(t *testing.T) {
	s, _ := openTest(t)
	ctx := context.Background()
	old, err := s.Remember(ctx, writer(), Note{Kind: KindDecision, Body: "use postgres"})
	if err != nil {
		t.Fatal(err)
	}
	res, err := s.Remember(ctx, writer(), Note{
		Kind: KindDecision, Body: "use sqlite", Replaces: []int64{old.Item.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	gotOld, err := s.Read(ctx, old.Item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotOld.Status != StatusRetired || gotOld.RetiredAt == nil ||
		gotOld.ReplacedBy == nil || *gotOld.ReplacedBy != res.Item.ID {
		t.Fatalf("old = %+v", gotOld)
	}
	last := res.Item.Links[len(res.Item.Links)-1]
	if last != (Link{Rel: RelReplaces, ToKind: "memory", ToID: "1"}) {
		t.Fatalf("replaces link = %+v", last)
	}
	if res.Item.Status != StatusAccepted {
		t.Fatalf("new status = %q", res.Item.Status)
	}
}

func TestRemember_ReplacesRetiredFailsAndWritesNothing(t *testing.T) {
	s, db := openTest(t)
	ctx := context.Background()
	live, err := s.Remember(ctx, writer(), Note{Kind: KindFact, Body: "live"})
	if err != nil {
		t.Fatal(err)
	}
	dead, err := s.Remember(ctx, writer(), Note{Kind: KindFact, Body: "dead"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Retire(ctx, dead.Item.ID); err != nil {
		t.Fatal(err)
	}
	items := count(t, db, `SELECT count(*) FROM memory`)
	links := count(t, db, `SELECT count(*) FROM memory_link`)

	_, err = s.Remember(ctx, writer(), Note{
		Kind: KindFact, Body: "new", Replaces: []int64{live.Item.ID, dead.Item.ID},
	})
	if err == nil {
		t.Fatal("want error")
	}
	if n := count(t, db, `SELECT count(*) FROM memory`); n != items {
		t.Fatalf("items = %d, want %d", n, items)
	}
	if n := count(t, db, `SELECT count(*) FROM memory_link`); n != links {
		t.Fatalf("links = %d, want %d", n, links)
	}
	got, err := s.Read(ctx, live.Item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusAccepted || got.ReplacedBy != nil {
		t.Fatalf("live item changed: %+v", got)
	}

	_, err = s.Remember(ctx, writer(), Note{Kind: KindFact, Body: "new", Replaces: []int64{999}})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing id err = %v, want ErrNotFound", err)
	}
}

func TestRetire_NothingReplacing(t *testing.T) {
	s, _ := openTest(t)
	ctx := context.Background()
	res, err := s.Remember(ctx, writer(), Note{Kind: KindFinding, Body: "stale"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Retire(ctx, res.Item.ID); err != nil {
		t.Fatal(err)
	}
	got, err := s.Read(ctx, res.Item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusRetired || got.RetiredAt == nil || got.ReplacedBy != nil {
		t.Fatalf("item = %+v", got)
	}
	if err := s.Retire(ctx, res.Item.ID); err == nil {
		t.Fatal("retire of retired item: want error")
	}
	if err := s.Retire(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("retire missing = %v, want ErrNotFound", err)
	}
}

func TestAccept_CandidateBecomesAccepted(t *testing.T) {
	s, db := openTest(t)
	ctx := context.Background()
	res, err := s.Remember(ctx, writer(), Note{Kind: KindFact, Body: "tabs"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Accept(ctx, res.Item.ID); err == nil {
		t.Fatal("accept of accepted item: want error")
	}
	if _, err := db.Exec(`UPDATE memory SET status = 'candidate' WHERE id = ?`, res.Item.ID); err != nil {
		t.Fatal(err)
	}
	pending, err := s.Pending(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].ID != res.Item.ID || len(pending[0].Links) != 1 {
		t.Fatalf("pending = %+v", pending)
	}
	if err := s.Accept(ctx, res.Item.ID); err != nil {
		t.Fatal(err)
	}
	got, err := s.Read(ctx, res.Item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusAccepted {
		t.Fatalf("status = %q", got.Status)
	}
	if pending, err = s.Pending(ctx); err != nil || len(pending) != 0 {
		t.Fatalf("pending after accept = %+v, %v", pending, err)
	}
	if err := s.Accept(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("accept missing = %v, want ErrNotFound", err)
	}
}

func TestAccept_RetiresWhatTheCandidateReplaces(t *testing.T) {
	s, _ := openTest(t)
	ctx := context.Background()
	project := Ref{Kind: ScopeProject, ID: "1"}
	if _, err := s.Remember(ctx, Writer{Author: "user", Scope: project},
		Note{Kind: KindConvention, Body: "use tabs"}); err != nil {
		t.Fatal(err)
	}
	res, err := s.Remember(ctx, Writer{Author: "ada", Role: "developer", Scope: project},
		Note{Kind: KindConvention, Body: "use spaces", Replaces: []int64{1}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Item.ID != 2 || res.Item.Status != StatusCandidate {
		t.Fatalf("new item = %+v, want candidate 2", res.Item)
	}
	link := Link{Rel: RelReplaces, ToKind: "memory", ToID: "1"}
	if res.Item.Links[len(res.Item.Links)-1] != link {
		t.Fatalf("links = %+v, want the replaces link kept", res.Item.Links)
	}

	old, err := s.Read(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if old.Status != StatusAccepted || old.ReplacedBy != nil {
		t.Fatalf("old after unapproved replace = %+v, want accepted", old)
	}
	hits := recall(t, s, Query{Scopes: []Ref{project}, Text: "tabs"})
	if len(hits) == 0 || hits[0].ID != 1 || hits[0].Status != StatusAccepted {
		t.Fatalf("recall of tabs = %+v, want accepted 1 first", hits)
	}

	if err := s.Accept(ctx, 2); err != nil {
		t.Fatal(err)
	}
	if old, err = s.Read(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if old.Status != StatusRetired || old.RetiredAt == nil ||
		old.ReplacedBy == nil || *old.ReplacedBy != 2 {
		t.Fatalf("old after accept = %+v, want retired, replaced_by 2", old)
	}
}

func TestRemember_CandidateReplacingRetiredFails(t *testing.T) {
	s, db := openTest(t)
	ctx := context.Background()
	project := Ref{Kind: ScopeProject, ID: "1"}
	if _, err := s.Remember(ctx, Writer{Author: "user", Scope: project},
		Note{Kind: KindConvention, Body: "use tabs"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Retire(ctx, 1); err != nil {
		t.Fatal(err)
	}
	_, err := s.Remember(ctx, Writer{Author: "ada", Role: "developer", Scope: project},
		Note{Kind: KindConvention, Body: "use spaces", Replaces: []int64{1}})
	const want = "replace 1: invalid: memory 1 is already retired"
	if !errors.Is(err, ErrInvalid) || err.Error() != want {
		t.Fatalf("err = %v, want %s", err, want)
	}
	if n := count(t, db, `SELECT count(*) FROM memory`); n != 1 {
		t.Fatalf("items = %d, want 1", n)
	}
}

func TestRead_Missing(t *testing.T) {
	s, _ := openTest(t)
	if _, err := s.Read(context.Background(), 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func candidate(t *testing.T, s *Store, scope Ref, body string, replaces ...int64) int64 {
	t.Helper()
	res, err := s.Remember(context.Background(), Writer{Author: "ada", Role: "developer", Scope: scope},
		Note{Kind: KindFact, Body: body, Replaces: replaces, NeedsApproval: true})
	if err != nil {
		t.Fatal(err)
	}
	return res.Item.ID
}

func TestReject_CandidateIsNeverFound(t *testing.T) {
	s, _ := openTest(t)
	ctx := context.Background()
	shelf := Ref{Kind: ScopeProject, ID: "1"}
	id := candidate(t, s, shelf, "rotate the signing keys monthly")
	if err := s.Reject(ctx, id); err != nil {
		t.Fatal(err)
	}
	got, err := s.Read(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusRejected || got.RetiredAt == nil {
		t.Fatalf("item = %+v", got)
	}
	if hits := recall(t, s, Query{Scopes: []Ref{shelf}, Text: "signing keys"}); len(hits) != 0 {
		t.Fatalf("recall = %+v, want nothing", hits)
	}
	if pending, err := s.Pending(ctx); err != nil || len(pending) != 0 {
		t.Fatalf("pending = %+v, %v", pending, err)
	}
	res, err := s.Remember(ctx, Writer{Author: "user", Scope: shelf},
		Note{Kind: KindFact, Body: "rotate the signing keys monthly"})
	if err != nil || len(res.Similar) != 0 {
		t.Fatalf("similar = %+v, %v; want none", res.Similar, err)
	}
}

func TestReject_RefusesWhatIsNotACandidate(t *testing.T) {
	s, _ := openTest(t)
	ctx := context.Background()
	shelf := Ref{Kind: ScopeProject, ID: "1"}
	accepted := put(t, s, shelf, Note{Body: "one"})
	retired := put(t, s, shelf, Note{Body: "two"})
	if err := s.Retire(ctx, retired); err != nil {
		t.Fatal(err)
	}
	rejected := candidate(t, s, shelf, "three")
	if err := s.Reject(ctx, rejected); err != nil {
		t.Fatal(err)
	}
	pending := candidate(t, s, shelf, "four")

	tests := []struct {
		name string
		err  error
		want string
	}{
		{"reject accepted", s.Reject(ctx, accepted), "invalid: memory 1 is not a candidate"},
		{"reject retired", s.Reject(ctx, retired), "invalid: memory 2 is not a candidate"},
		{"reject rejected", s.Reject(ctx, rejected), "invalid: memory 3 is not a candidate"},
		{"retire candidate", s.Retire(ctx, pending), "invalid: memory 4 is a candidate"},
		{"retire rejected", s.Retire(ctx, rejected), "invalid: memory 3 is rejected"},
		{"retire retired", s.Retire(ctx, retired), "invalid: memory 2 is already retired"},
		{"accept rejected", s.Accept(ctx, rejected), "invalid: memory 3 is not a candidate"},
		{"replace candidate", func() error {
			_, err := s.Remember(ctx, Writer{Author: "user", Scope: shelf},
				Note{Kind: KindFact, Body: "five", Replaces: []int64{pending}})
			return err
		}(), "replace 4: invalid: memory 4 is a candidate"},
		{"replace rejected", func() error {
			_, err := s.Remember(ctx, Writer{Author: "user", Scope: shelf},
				Note{Kind: KindFact, Body: "five", Replaces: []int64{rejected}})
			return err
		}(), "replace 3: invalid: memory 3 is rejected"},
	}
	for _, tt := range tests {
		if !errors.Is(tt.err, ErrInvalid) || tt.err.Error() != tt.want {
			t.Errorf("%s: err = %v, want %s", tt.name, tt.err, tt.want)
		}
	}
	if err := s.Reject(ctx, 99); !errors.Is(err, ErrNotFound) {
		t.Fatalf("reject missing: err = %v, want ErrNotFound", err)
	}
}

func TestReject_CandidateReplacingAnItemLeavesItLive(t *testing.T) {
	s, _ := openTest(t)
	ctx := context.Background()
	shelf := Ref{Kind: ScopeProject, ID: "1"}
	old := put(t, s, shelf, Note{Body: "deploy on fridays"})
	next := candidate(t, s, shelf, "never deploy on fridays", old)
	if err := s.Reject(ctx, next); err != nil {
		t.Fatal(err)
	}
	got, err := s.Read(ctx, old)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusAccepted || got.RetiredAt != nil || got.ReplacedBy != nil {
		t.Fatalf("old = %+v, want live", got)
	}
	if hits := recall(t, s, Query{Scopes: []Ref{shelf}, Text: "deploy"}); !reflect.DeepEqual(hitIDs(hits), []int64{old}) {
		t.Fatalf("recall = %v, want [%d]", hitIDs(hits), old)
	}
}

func TestOpen_UpgradesTheFirstMemorySchema(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	first, err := _migrations.ReadFile("migrations/001_items.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		string(first),
		`INSERT INTO memory (kind, body, scope_kind, scope_id, status, created_at)
		 VALUES ('fact', 'use sqlite', 'project', '1', 'accepted', '2026-10-01T00:00:00Z'),
		        ('fact', 'use postgres', 'project', '1', 'candidate', '2026-10-02T00:00:00Z')`,
		`INSERT INTO memory_link (from_id, rel, to_kind, to_id) VALUES (2, 'replaces', 'memory', '1')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}

	s, err := Open(db, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if n := count(t, db, `SELECT count(*) FROM memory`); n != 2 {
		t.Fatalf("items = %d, want 2", n)
	}
	got, err := s.Read(ctx, 2)
	if err != nil || got.Body != "use postgres" || !reflect.DeepEqual(got.Links,
		[]Link{{Rel: RelReplaces, ToKind: "memory", ToID: "1"}}) {
		t.Fatalf("item 2 = %+v, %v", got, err)
	}
	hits, err := s.Search(ctx, "sqlite", Filter{Scopes: []Ref{{Kind: ScopeProject, ID: "1"}}})
	if err != nil || !reflect.DeepEqual(ids(hits), []int64{1}) {
		t.Fatalf("search = %v, %v; want [1]", ids(hits), err)
	}
	if err := s.Reject(ctx, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(db, nil); err != nil {
		t.Fatalf("second Open: %v", err)
	}
	if n := count(t, db, `SELECT version FROM memory_version`); n != 2 {
		t.Fatalf("version = %d, want 2", n)
	}
	if n := count(t, db, `SELECT count(*) FROM memory_version`); n != 1 {
		t.Fatalf("version rows = %d, want 1", n)
	}
	id := put(t, s, Ref{Kind: ScopeProject, ID: "1"}, Note{Body: "use redis"})
	if id != 3 {
		t.Fatalf("next id = %d, want 3", id)
	}
	hits, err = s.Search(ctx, "redis", Filter{Scopes: []Ref{{Kind: ScopeProject, ID: "1"}}})
	if err != nil || !reflect.DeepEqual(ids(hits), []int64{3}) {
		t.Fatalf("search after write = %v, %v; want [3]", ids(hits), err)
	}
}
