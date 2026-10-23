// Package eval scores memory.Recall against the scenarios in fixture.json.
//
// The overall recall@10 and MRR line is printed to stdout, which `go test`
// hides for a passing package given by path: run it with -v, or from inside
// this directory. -v also prints one line per scenario. Scenarios marked
// needs_embedder run only with `-tags embed` and REEF_EMBED_PROVIDER and
// REEF_EMBED_KEY set; every other scenario runs without an embedder.
package eval

import (
	"bytes"
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/memory"
	_ "modernc.org/sqlite"
)

const _atK = 10

//go:embed fixture.json
var _fixture []byte

type fixture struct {
	Thresholds struct {
		RecallAt10 float64 `json:"recall_at_10"`
		MRR        float64 `json:"mrr"`
	} `json:"thresholds"`
	Scenarios []scenario `json:"scenarios"`
}

type scenario struct {
	Name          string   `json:"name"`
	Category      string   `json:"category"`
	NeedsEmbedder bool     `json:"needs_embedder"`
	Items         []item   `json:"items"`
	Query         query    `json:"query"`
	Expect        []string `json:"expect"`
	Ordered       bool     `json:"ordered"`
	Forbidden     []string `json:"forbidden"`
}

type item struct {
	Ref      string   `json:"ref"`
	Kind     string   `json:"kind"`
	Body     string   `json:"body"`
	Why      string   `json:"why"`
	Scope    string   `json:"scope"`
	Author   string   `json:"author"`
	Role     string   `json:"role"`
	About    []string `json:"about"`
	Replaces []string `json:"replaces"`
	Retire   bool     `json:"retire"`
}

type query struct {
	Text   string   `json:"text"`
	Scopes []string `json:"scopes"`
	Scope  string   `json:"scope"`
	Limit  int      `json:"limit"`
}

func TestRecall(t *testing.T) {
	var fx fixture
	dec := json.NewDecoder(bytes.NewReader(_fixture))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&fx); err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	emb, err := newEmbedder()
	if err != nil {
		t.Fatalf("embedder: %v", err)
	}

	var ran, skipped int
	var sumRecall, sumMRR float64
	for _, sc := range fx.Scenarios {
		t.Run(sc.Name, func(t *testing.T) {
			var e memory.Embedder
			if sc.NeedsEmbedder {
				if emb == nil {
					skipped++
					t.Skip("needs an embedder: build with -tags embed and set REEF_EMBED_PROVIDER and REEF_EMBED_KEY")
				}
				e = emb
			}
			recall, mrr := run(t, sc, e)
			ran++
			sumRecall += recall
			sumMRR += mrr
			t.Logf("%-22s recall@%d=%.2f mrr=%.2f", sc.Category, _atK, recall, mrr)
		})
	}

	if ran == 0 {
		t.Fatal("no scenario ran")
	}
	recall, mrr := sumRecall/float64(ran), sumMRR/float64(ran)
	fmt.Printf("eval: %d scenarios, %d skipped, recall@%d=%.3f mrr=%.3f (thresholds %.2f, %.2f)\n",
		ran, skipped, _atK, recall, mrr, fx.Thresholds.RecallAt10, fx.Thresholds.MRR)
	if recall < fx.Thresholds.RecallAt10 {
		t.Errorf("recall@%d = %.3f, under threshold %.2f", _atK, recall, fx.Thresholds.RecallAt10)
	}
	if mrr < fx.Thresholds.MRR {
		t.Errorf("mrr = %.3f, under threshold %.2f", mrr, fx.Thresholds.MRR)
	}
}

// run seeds a fresh database with the scenario's items and asks its query. It
// fails the test at once on a forbidden item, or on expected items out of order
// when the scenario is ordered. It returns recall@10 and the reciprocal rank
// of the first expected item.
func run(t *testing.T, sc scenario, e memory.Embedder) (recall, mrr float64) {
	t.Helper()
	if len(sc.Expect) == 0 {
		t.Fatal("scenario expects nothing")
	}
	ctx := context.Background()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	s, err := memory.Open(db, e)
	if err != nil {
		t.Fatal(err)
	}

	ids := make(map[string]int64)
	names := make(map[int64]string)
	for _, it := range sc.Items {
		id := seed(ctx, t, s, it, ids)
		ids[it.Ref] = id
		names[id] = it.Ref
	}
	for _, it := range sc.Items {
		if it.Retire {
			if err := s.Retire(ctx, ids[it.Ref]); err != nil {
				t.Fatalf("retire %s: %v", it.Ref, err)
			}
		}
	}

	q := memory.Query{Text: sc.Query.Text, Limit: sc.Query.Limit}
	for _, r := range sc.Query.Scopes {
		q.Scopes = append(q.Scopes, ref(t, r))
	}
	if sc.Query.Scope != "" {
		r := ref(t, sc.Query.Scope)
		q.Scope = &r
	}
	hits, err := s.Recall(ctx, q)
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	got := make([]int64, len(hits))
	returned := make([]string, len(hits))
	for i, h := range hits {
		got[i], returned[i] = h.ID, names[h.ID]
	}

	for _, f := range sc.Forbidden {
		if slices.Contains(got, lookup(t, ids, f)) {
			t.Fatalf("forbidden %s returned; results: %v", f, returned)
		}
	}
	if sc.Ordered {
		last := -1
		for _, x := range sc.Expect {
			i := slices.Index(got, lookup(t, ids, x))
			if i < 0 || i < last {
				t.Fatalf("want %v in that order; results: %v", sc.Expect, returned)
			}
			last = i
		}
	}

	top := got[:min(len(got), _atK)]
	found := 0
	for _, x := range sc.Expect {
		if i := slices.Index(top, lookup(t, ids, x)); i >= 0 {
			found++
			mrr = max(mrr, 1/float64(i+1))
		}
	}
	return float64(found) / float64(len(sc.Expect)), mrr
}

func seed(ctx context.Context, t *testing.T, s *memory.Store, it item, ids map[string]int64) int64 {
	t.Helper()
	author := it.Author
	if author == "" {
		author = "user"
	}
	n := memory.Note{Kind: it.Kind, Body: it.Body, Why: it.Why}
	for _, a := range it.About {
		n.About = append(n.About, ref(t, a))
	}
	for _, r := range it.Replaces {
		n.Replaces = append(n.Replaces, lookup(t, ids, r))
	}
	res, err := s.Remember(ctx, memory.Writer{Author: author, Role: it.Role, Scope: ref(t, it.Scope)}, n)
	if err != nil {
		t.Fatalf("remember %s: %v", it.Ref, err)
	}
	return res.Item.ID
}

func ref(t *testing.T, s string) memory.Ref {
	t.Helper()
	kind, id, ok := strings.Cut(s, ":")
	if !ok {
		t.Fatalf("bad ref %q, want kind:id", s)
	}
	return memory.Ref{Kind: kind, ID: id}
}

func lookup(t *testing.T, ids map[string]int64, name string) int64 {
	t.Helper()
	id, ok := ids[name]
	if !ok {
		t.Fatalf("unknown item ref %q", name)
	}
	return id
}
