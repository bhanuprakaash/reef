package server

import (
	"context"
	"testing"

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

	p, added, err := ApproveProposal(ctx, st, ms, 1)
	if err != nil {
		t.Fatal(err)
	}
	if p.Status != store.ProposalApproved || added != 3 {
		t.Errorf("approve = status %q, added %d, want approved and 3", p.Status, added)
	}
	want := map[int64]struct {
		scope  memory.Ref
		status string
	}{
		1: {memory.Ref{Kind: "work_item", ID: "1"}, memory.StatusRetired},
		2: {memory.Ref{Kind: "work_item", ID: "1"}, memory.StatusAccepted},
		3: {memory.Ref{Kind: "project", ID: "1"}, memory.StatusRetired},
		4: {memory.Ref{Kind: "work_item", ID: "1"}, memory.StatusAccepted},
		5: {memory.Ref{Kind: "teammate", ID: "architect"}, memory.StatusAccepted},
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
	if b, err := st.Brief(ctx, 1); err != nil || b.Body != "v2" {
		t.Errorf("brief = %q, %v, want v2", b.Body, err)
	}
	if c, err := st.Channel(ctx, 1); err != nil || c.ClosedAt == nil {
		t.Errorf("channel = %+v, %v, want closed", c, err)
	}
}

func TestApproveProposalLeadTeamLessonIsCandidate(t *testing.T) {
	ctx := context.Background()
	st, _ := mcpFixture(t)
	if _, err := st.CreateTeam(ctx, store.Team{Name: "core", Roles: []store.Role{
		{Name: "lead", Lead: true, Backend: "claude"},
		{Name: "developer", Backend: "claude"},
	}}); err != nil {
		t.Fatal(err)
	}
	one := int64(1)
	ch, err := st.CreateChannel(ctx, store.Channel{
		ProjectID: 1, FeatureID: &one, Name: "team", Team: "core", Role: "lead", Backend: "claude",
	})
	if err != nil {
		t.Fatal(err)
	}
	items := map[string]any{"items": []any{
		map[string]any{"kind": "lesson", "body": "ask before refactoring", "scope": "team"},
	}, "brief": "v2"}
	developer, err := st.CreateToken(ctx, ch.ID, "developer")
	if err != nil {
		t.Fatal(err)
	}
	got, isErr := callTool(t, st, developer)("propose_wrap", items)
	if want := `invalid: only the lead of team "core" can propose team items`; got != want || !isErr {
		t.Errorf("developer propose_wrap = %q (error %v), want %q", got, isErr, want)
	}
	lead, err := st.CreateToken(ctx, ch.ID, "lead")
	if err != nil {
		t.Fatal(err)
	}
	if got, isErr := callTool(t, st, lead)("propose_wrap", items); isErr {
		t.Fatalf("lead propose_wrap = %q", got)
	}

	ms := openMemory(t, st)
	if _, added, err := ApproveProposal(ctx, st, ms, 1); err != nil || added != 1 {
		t.Fatalf("approve = %d, %v, want 1 added", added, err)
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

	if _, _, err := ApproveProposal(ctx, st, ms, 1); err == nil {
		t.Fatal("approve succeeded, want it to fail on the second item")
	}
	if n := countMemory(t, st); n != 1 {
		t.Errorf("memory rows after the failed approve = %d, want 1", n)
	}
	if p, err := st.PendingProposal(context.Background(), 1); err != nil || p.Status != store.ProposalPending {
		t.Errorf("pending proposal = %+v, %v, want it still pending", p, err)
	}
	if c, err := st.Channel(context.Background(), 1); err != nil || c.ClosedAt != nil {
		t.Errorf("channel = %+v, %v, want it open", c, err)
	}
	if b, err := st.Brief(context.Background(), 1); err != nil || b.Body != "Build login" {
		t.Errorf("brief = %q, %v, want it unchanged", b.Body, err)
	}

	p, added, err := ApproveProposal(context.Background(), st, ms, 1)
	if err != nil {
		t.Fatal(err)
	}
	if p.Status != store.ProposalApproved || added != 2 {
		t.Errorf("second approve = status %q, added %d, want approved and 2", p.Status, added)
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

	_, added, err := ApproveProposal(ctx, st, openMemory(t, st), 1)
	if err != nil {
		t.Fatal(err)
	}
	if n := countMemory(t, st); added != 1 || n != 3 {
		t.Errorf("added = %d, memory rows = %d, want 1 added (the same body in project) and 3 rows", added, n)
	}
}
