package server

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bhanuprakaash/reef/memory"
	"github.com/bhanuprakaash/reef/store"
)

func countMemory(t *testing.T, st *store.Store) int {
	t.Helper()
	var n int
	if err := st.DB().QueryRow("SELECT COUNT(*) FROM memory").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestApproveProposalSavesItemsAndRetires(t *testing.T) {
	ctx := context.Background()
	st, tok := mcpFixture(t)
	ms := openMemory(t, st)
	call := callTool(t, st, tok)
	call("remember", map[string]any{"kind": "fact", "body": "old one"})
	call("remember", map[string]any{"kind": "fact", "body": "old two"})
	call("remember", map[string]any{"kind": "fact", "body": "old project", "scope": "project"})
	got, isErr := call("propose_wrap", map[string]any{
		"brief": "v2",
		"items": []any{
			map[string]any{"kind": "decision", "body": "use sessions", "why": "simple"},
			map[string]any{"kind": "lesson", "body": "reply short", "scope": "teammate"},
			map[string]any{"kind": "fact", "body": "login at /in", "scope": "project", "replaces": []any{3}},
		},
		"retire": []any{1},
	})
	if isErr {
		t.Fatalf("propose_wrap = %q", got)
	}
	if err := ms.Retire(ctx, 1); err != nil {
		t.Fatal(err)
	}

	a, err := ApproveProposal(ctx, st, ms, 1)
	if err != nil {
		t.Fatal(err)
	}
	if a.Proposal.Status != store.ProposalApproved || a.Added != 3 {
		t.Errorf("approve = status %q, added %d, want approved and 3", a.Proposal.Status, a.Added)
	}
	want := map[int64]struct {
		scope  memory.Ref
		status string
	}{
		1: {memory.Ref{Kind: "work_item", ID: "1"}, memory.StatusRetired},
		2: {memory.Ref{Kind: "work_item", ID: "1"}, memory.StatusAccepted},
		3: {memory.Ref{Kind: "project", ID: "1"}, memory.StatusRetired},
		4: {memory.Ref{Kind: "work_item", ID: "1"}, memory.StatusAccepted},
		5: {memory.Ref{Kind: "teammate", ID: "2"}, memory.StatusAccepted},
		6: {memory.Ref{Kind: "project", ID: "1"}, memory.StatusAccepted},
	}
	for id, w := range want {
		it, err := ms.Read(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if it.Scope != w.scope || it.Status != w.status {
			t.Errorf("memory %d = %v %s, want %v %s", id, it.Scope, it.Status, w.scope, w.status)
		}
	}
	if it, _ := ms.Read(ctx, 4); it.Author != "architect" || it.Kind != "decision" || it.Why != "simple" {
		t.Errorf("memory 4 = %+v, want architect's decision \"simple\"", it)
	}
	if it, _ := ms.Read(ctx, 4); !reflect.DeepEqual(it.Links,
		[]memory.Link{{Rel: memory.RelSource, ToKind: "proposal", ToID: "1"}}) {
		t.Errorf("memory 4 links = %+v, want the source proposal 1", it.Links)
	}
	if b, err := st.Brief(ctx, 1); err != nil || b.Body != "v2" {
		t.Errorf("brief = %q, %v, want v2", b.Body, err)
	}
	if c, err := st.Space(ctx, 1); err != nil || c.ClosedAt == nil {
		t.Errorf("space = %+v, %v, want closed", c, err)
	}
}

func TestApproveProposalLeadTeamLessonIsCandidate(t *testing.T) {
	ctx := context.Background()
	st, _ := mcpFixture(t)
	if _, err := testServerCreateTeam(t, st, ctx, store.Team{Name: "core", Roles: []store.Role{
		{Name: "lead", Lead: true, Backend: "claude"},
		{Name: "developer", Backend: "claude"},
	}}); err != nil {
		t.Fatal(err)
	}
	one := int64(1)
	closeSpace(t, st, 1)
	ch, err := st.CreateSpace(ctx, store.Space{
		ProjectID: 1, WorkItemID: &one, Name: "team", Team: "core", Role: "lead", Backend: "claude",
	})
	if err != nil {
		t.Fatal(err)
	}
	items := map[string]any{"items": []any{
		map[string]any{"kind": "lesson", "body": "ask before refactoring", "scope": "team"},
	}, "brief": "v2"}
	developer, err := testToken(t, st, ctx, ch.ID, "developer")
	if err != nil {
		t.Fatal(err)
	}
	got, isErr := callTool(t, st, developer)("propose_wrap", items)
	if want := `invalid: only the lead of team "core" can propose team items`; got != want || !isErr {
		t.Errorf("developer propose_wrap = %q (error %v), want %q", got, isErr, want)
	}
	lead, err := testToken(t, st, ctx, ch.ID, "lead")
	if err != nil {
		t.Fatal(err)
	}
	if got, isErr := callTool(t, st, lead)("propose_wrap", items); isErr {
		t.Fatalf("lead propose_wrap = %q", got)
	}

	ms := openMemory(t, st)
	if a, err := ApproveProposal(ctx, st, ms, 1); err != nil || a.Added != 1 {
		t.Fatalf("approve = %d, %v, want 1 added", a.Added, err)
	}
	pending, err := ms.Pending(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].Body != "ask before refactoring" ||
		pending[0].Scope != (memory.Ref{Kind: "team", ID: "core"}) || pending[0].Author != "lead" {
		t.Errorf("pending = %+v, want the lead's team lesson as a candidate in team:core", pending)
	}
}

type cancelEmbedder struct {
	cancel context.CancelFunc
	calls  int
	at     int
}

func (e *cancelEmbedder) Embed(context.Context, string) ([]float32, error) {
	e.calls++
	if e.calls == e.at {
		e.cancel()
	}
	return nil, nil
}

func (*cancelEmbedder) MinCosine() float64 { return 0 }

func TestApproveProposalRetryAfterFailureSavesEachItemOnce(t *testing.T) {
	st, tok := mcpFixture(t)
	if got, isErr := callTool(t, st, tok)("propose_wrap", map[string]any{
		"brief": "v2",
		"items": []any{
			map[string]any{"kind": "decision", "body": "alpha"},
			map[string]any{"kind": "finding", "body": "bravo"},
			map[string]any{"kind": "fact", "body": "charlie"},
		},
	}); isErr {
		t.Fatalf("propose_wrap = %q", got)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e := &cancelEmbedder{cancel: cancel, at: 2}
	ms, err := memory.Open(st.DB(), e)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := ApproveProposal(ctx, st, ms, 1); err == nil {
		t.Fatal("approve succeeded, want it to fail on the second item")
	}
	if n := countMemory(t, st); n != 1 {
		t.Errorf("memory rows after the failed approve = %d, want 1", n)
	}
	if p, err := st.PendingProposal(context.Background(), 1); err != nil || p.Status != store.ProposalPending {
		t.Errorf("pending proposal = %+v, %v, want it still pending", p, err)
	}
	if c, err := st.Space(context.Background(), 1); err != nil || c.ClosedAt != nil {
		t.Errorf("space = %+v, %v, want it open", c, err)
	}
	if b, err := st.Brief(context.Background(), 1); err != nil || b.Body != "Build login" {
		t.Errorf("brief = %q, %v, want it unchanged", b.Body, err)
	}

	a, err := ApproveProposal(context.Background(), st, ms, 1)
	if err != nil {
		t.Fatal(err)
	}
	if a.Proposal.Status != store.ProposalApproved || a.Added != 2 {
		t.Errorf("second approve = status %q, added %d, want approved and 2", a.Proposal.Status, a.Added)
	}
	if n := countMemory(t, st); n != 3 {
		t.Errorf("memory rows after the second approve = %d, want 3", n)
	}
	if b, err := st.Brief(context.Background(), 1); err != nil || b.Body != "v2" {
		t.Errorf("brief = %q, %v, want v2", b.Body, err)
	}
}

func TestApproveProposalSkipsItemAlreadySaved(t *testing.T) {
	ctx := context.Background()
	st, tok := mcpFixture(t)
	call := callTool(t, st, tok)
	call("remember", map[string]any{"kind": "decision", "body": "use sessions"})
	call("remember", map[string]any{"kind": "lesson", "body": "ask first", "scope": "teammate", "needsApproval": true})
	if got, isErr := call("propose_wrap", map[string]any{
		"brief": "v2",
		"items": []any{
			map[string]any{"kind": "decision", "body": "use sessions"},
			map[string]any{"kind": "lesson", "body": "ask first", "scope": "teammate"},
			map[string]any{"kind": "decision", "body": "use sessions", "scope": "project"},
		},
	}); isErr {
		t.Fatalf("propose_wrap = %q", got)
	}

	a, err := ApproveProposal(ctx, st, openMemory(t, st), 1)
	if err != nil {
		t.Fatal(err)
	}
	if n := countMemory(t, st); a.Added != 1 || n != 3 {
		t.Errorf("added = %d, memory rows = %d, want 1 added (the same body in project) and 3 rows", a.Added, n)
	}
}

type failEmbedder struct{}

func (failEmbedder) Embed(context.Context, string) ([]float32, error) {
	return nil, errors.New("provider down")
}

func (failEmbedder) MinCosine() float64 { return 0 }

func TestApproveProposalReportsLookalikesAndEmbedErrors(t *testing.T) {
	ctx := context.Background()
	st, tok := mcpFixture(t)
	call := callTool(t, st, tok)
	call("remember", map[string]any{"kind": "decision", "body": "use sessions for login"})
	if got, isErr := call("propose_wrap", map[string]any{
		"brief": "v2",
		"items": []any{map[string]any{"kind": "decision", "body": "use sessions for login."}},
	}); isErr {
		t.Fatalf("propose_wrap = %q", got)
	}
	ms, err := memory.Open(st.DB(), failEmbedder{})
	if err != nil {
		t.Fatal(err)
	}

	a, err := ApproveProposal(ctx, st, ms, 1)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"memory 2 looks like memory 1", "memory 2: vector not stored: provider down"}
	if !reflect.DeepEqual(a.Warnings, want) {
		t.Errorf("warnings = %q, want %q", a.Warnings, want)
	}
}

type slowEmbedder struct{}

func (slowEmbedder) Embed(context.Context, string) ([]float32, error) {
	time.Sleep(20 * time.Millisecond)
	return nil, nil
}

func (slowEmbedder) MinCosine() float64 { return 0 }

func TestApproveProposalRESTTwiceAtOnceSavesEachItemOnce(t *testing.T) {
	st, tok := mcpFixture(t)
	call := callTool(t, st, tok)
	call("remember", map[string]any{"kind": "fact", "body": "old"})
	if got, isErr := call("propose_wrap", map[string]any{
		"brief": "v2",
		"items": []any{
			map[string]any{"kind": "decision", "body": "alpha"},
			map[string]any{"kind": "finding", "body": "bravo"},
		},
		"retire": []any{1},
	}); isErr {
		t.Fatalf("propose_wrap = %q", got)
	}
	ms, err := memory.Open(st.DB(), slowEmbedder{})
	if err != nil {
		t.Fatal(err)
	}

	h := New(st, ms, t.TempDir(), "/bin/reef")
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	t.Cleanup(h.Close)

	replies := make(chan string, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			status, body := do(t, "POST", ts.URL+"/proposals/1/approve", "")
			replies <- fmt.Sprintf("%d %s", status, body)
		}()
	}
	wg.Wait()
	close(replies)

	var failed []string
	for r := range replies {
		if strings.HasPrefix(r, "400") {
			failed = append(failed, r)
		}
	}
	if want := []string{`400 {"error":"invalid: proposal 1 is already approved"}`}; !reflect.DeepEqual(failed, want) {
		t.Errorf("rejected approves = %q, want %q", failed, want)
	}
	if n := countMemory(t, st); n != 3 {
		t.Errorf("memory rows = %d, want 3: the old one and each item once", n)
	}
}

func TestApproveProposalWarnsWhenASkippedItemDropsItsReplaces(t *testing.T) {
	st, tok := mcpFixture(t)
	call := callTool(t, st, tok)
	call("remember", map[string]any{"kind": "fact", "body": "sessions last a day"})
	call("remember", map[string]any{"kind": "decision", "body": "use sessions"})
	if got, isErr := call("propose_wrap", map[string]any{
		"brief": "v2",
		"items": []any{map[string]any{
			"kind": "decision", "body": "use sessions", "replaces": []any{1},
		}},
	}); isErr {
		t.Fatalf("propose_wrap = %q", got)
	}

	a, err := ApproveProposal(context.Background(), st, openMemory(t, st), 1)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{`memory 1 not replaced: item "use sessions" already saved`}
	if a.Added != 0 || !reflect.DeepEqual(a.Warnings, want) {
		t.Errorf("approve = added %d, warnings %q, want 0 and %q", a.Added, a.Warnings, want)
	}
}

func TestApproveProposalKeepsAnItemItsOwnProposalRetires(t *testing.T) {
	rows := func(t *testing.T, st *store.Store) []string {
		t.Helper()
		r, err := st.DB().Query("SELECT id || ' ' || body || ' ' || status FROM memory ORDER BY id")
		if err != nil {
			t.Fatal(err)
		}
		defer r.Close()
		var out []string
		for r.Next() {
			var s string
			if err := r.Scan(&s); err != nil {
				t.Fatal(err)
			}
			out = append(out, s)
		}
		return out
	}
	propose := func(t *testing.T) *store.Store {
		t.Helper()
		st, tok := mcpFixture(t)
		call := callTool(t, st, tok)
		call("remember", map[string]any{"kind": "decision", "body": "use sessions"})
		if got, isErr := call("propose_wrap", map[string]any{
			"brief": "v2",
			"items": []any{
				map[string]any{"kind": "decision", "body": "use sessions"},
				map[string]any{"kind": "fact", "body": "login at /in"},
			},
			"retire": []any{1},
		}); isErr {
			t.Fatalf("propose_wrap = %q", got)
		}
		return st
	}
	want := []string{
		"1 use sessions retired", "2 use sessions accepted", "3 login at /in accepted",
	}

	clean := propose(t)
	if _, err := ApproveProposal(context.Background(), clean, openMemory(t, clean), 1); err != nil {
		t.Fatal(err)
	}
	if got := rows(t, clean); !reflect.DeepEqual(got, want) {
		t.Errorf("clean approve memory = %q, want %q", got, want)
	}

	retried := propose(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ms, err := memory.Open(retried.DB(), &cancelEmbedder{cancel: cancel, at: 2})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApproveProposal(ctx, retried, ms, 1); err == nil {
		t.Fatal("first approve succeeded, want it to fail on the second item")
	}
	if _, err := ApproveProposal(context.Background(), retried, ms, 1); err != nil {
		t.Fatal(err)
	}
	if got := rows(t, retried); !reflect.DeepEqual(got, want) {
		t.Errorf("retried approve memory = %q, want %q", got, want)
	}
}
