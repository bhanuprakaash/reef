package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bhanuprakaash/reef/memory"
)

func TestSharedMemoryChangesNeedApproval(t *testing.T) {
	ctx := context.Background()
	st, tok := mcpFixture(t)
	ms := openMemory(t, st)
	call := callToolMemory(t, st, ms, tok, t.TempDir())
	ts := httptest.NewServer(New(st, ms, t.TempDir(), "/bin/reef"))
	t.Cleanup(ts.Close)

	status := func(id int64) (string, *int64) {
		t.Helper()
		it, err := ms.Read(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return it.Status, it.ReplacedBy
	}
	if _, isErr := call("remember", map[string]any{"kind": "fact", "body": "uses postgres", "scope": "project"}); isErr {
		t.Fatal("remember fact failed")
	}
	if _, isErr := call("remember", map[string]any{
		"kind": "convention", "body": "use tabs", "scope": "project",
	}); isErr {
		t.Fatal("remember convention failed")
	}
	if _, isErr := call("remember", map[string]any{
		"kind": "fact", "body": "uses sqlite", "scope": "project", "replaces": []any{1},
	}); isErr {
		t.Fatal("remember replacement failed")
	}
	for id, want := range map[int64]string{1: "accepted", 2: "candidate", 3: "candidate"} {
		if got, _ := status(id); got != want {
			t.Errorf("memory %d status = %q, want %q", id, got, want)
		}
	}

	for range 2 {
		if code, body := do(t, http.MethodPost, ts.URL+"/memory/3/approve", ""); code != http.StatusOK {
			t.Fatalf("approve = %d %s, want 200", code, body)
		}
	}
	if got, by := status(1); got != "retired" || by == nil || *by != 3 {
		t.Errorf("memory 1 = %q replaced by %v, want retired by 3", got, by)
	}
	if code, _ := do(t, http.MethodPost, ts.URL+"/memory/2/reject", ""); code != http.StatusOK {
		t.Errorf("reject = %d, want 200", code)
	}
	if got, _ := status(2); got != "rejected" {
		t.Errorf("memory 2 status = %q, want rejected", got)
	}

	if _, isErr := call("remember", map[string]any{
		"kind": "fact", "body": "uses duckdb", "scope": "project", "replaces": []any{3},
	}); isErr {
		t.Fatal("remember second replacement failed")
	}
	if _, isErr := call("propose_wrap", map[string]any{"brief": "v2", "retire": []any{3}}); isErr {
		t.Fatal("propose_wrap failed")
	}
	if got, _ := status(3); got != "accepted" {
		t.Errorf("memory 3 before approval = %q, want accepted", got)
	}
	if code, body := do(t, http.MethodPost, ts.URL+"/proposals/1/approve", ""); code != http.StatusOK {
		t.Fatalf("approve proposal = %d %s, want 200", code, body)
	}
	if got, by := status(3); got != memory.StatusRetired || by != nil {
		t.Errorf("memory 3 = %q replaced by %v, want retired by the wrap-up with no replacement", got, by)
	}
}
