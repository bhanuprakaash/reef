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
	if strings.Contains(strings.ToLower(_schema), "references") {
		t.Fatal("schema has REFERENCES")
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
		name string
		give Writer
		kind string
		want string
	}{
		{"convention by developer", Writer{Author: "ada", Role: "developer", Scope: project}, KindConvention, "candidate"},
		{"convention by architect", Writer{Author: "bo", Role: "architect", Scope: project}, KindConvention, "accepted"},
		{"convention by user", Writer{Author: "user", Scope: project}, KindConvention, "accepted"},
		{"decision by developer", Writer{Author: "ada", Role: "developer", Scope: project}, KindDecision, "accepted"},
		{"decision by tester", Writer{Author: "cy", Role: "tester", Scope: project}, KindDecision, "accepted"},
		{"finding by tester", Writer{Author: "cy", Role: "tester", Scope: project}, KindFinding, "accepted"},
		{"lesson in team scope by lead", Writer{Author: "di", Role: "lead", Scope: team}, KindLesson, "candidate"},
		{"lesson in project scope by developer", Writer{Author: "ada", Role: "developer", Scope: project}, KindLesson, "accepted"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, _ := openTest(t)
			res, err := s.Remember(context.Background(), tt.give, Note{Kind: tt.kind, Body: "x"})
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
