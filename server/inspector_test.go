package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/store"
)

func TestGetInspector(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	h := New(st, testMemory(t, st), t.TempDir(), "/bin/reef")
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	t.Cleanup(h.Close)

	do(t, http.MethodPost, ts.URL+"/projects", `{"name":"reef","path":"/tmp"}`)
	do(t, http.MethodPost, ts.URL+"/spaces", `{"projectId":1,"name":"c","backend":"claude"}`)
	if _, err := st.CreateTurn(ctx, store.Turn{SpaceID: 1, Role: "architect", Backend: "claude"}); err != nil {
		t.Fatal(err)
	}
	cost := 0.05
	if _, err := st.EndTurn(ctx, store.Turn{
		ID: 1, Status: "done", Model: "claude-opus-5-5", CostUSD: &cost, Prompt: "do it", Reply: "kept",
		Usage: agent.Usage{InputTokens: 10, OutputTokens: 20},
	}); err != nil {
		t.Fatal(err)
	}
	tc, err := st.CreateToolCall(ctx, 1, 0, "Bash", `{"command":"ls"}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetToolCallResult(ctx, tc.ID, "a.txt\n"); err != nil {
		t.Fatal(err)
	}

	status, body := do(t, http.MethodGet, ts.URL+"/spaces/1/inspector", "")
	if status != 200 {
		t.Fatalf("status = %d, body = %s", status, body)
	}
	wantRole := `"roles":[{"role":"architect","turns":1,"usage":{"inputTokens":10,"outputTokens":20,` +
		`"cacheReadTokens":0,"cacheWriteTokens":0},"costUsd":0.05}]`
	wantTurn := `"status":"done","model":"claude-opus-5-5",` +
		`"usage":{"inputTokens":10,"outputTokens":20,"cacheReadTokens":0,"cacheWriteTokens":0},` +
		`"costUsd":0.05,"prompt":"do it","reply":"kept","toolCalls":[{"name":"Bash","input":"{\"command\":\"ls\"}",` +
		`"result":"a.txt\n"}]`
	if !strings.Contains(body, wantRole) || !strings.Contains(body, `"turns":[{"id":1,"role":"architect","startedAt":"`) ||
		!strings.Contains(body, wantTurn) {
		t.Errorf("body = %s, want it to contain %s and %s", body, wantRole, wantTurn)
	}

	status, body = do(t, http.MethodGet, ts.URL+"/spaces/9/inspector", "")
	if status != 404 {
		t.Errorf("unknown Space: status = %d, body = %s", status, body)
	}
}

func TestGetInspectorNestsSubagentToolCalls(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	h := New(st, testMemory(t, st), t.TempDir(), "/bin/reef")
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	t.Cleanup(h.Close)

	do(t, http.MethodPost, ts.URL+"/projects", `{"name":"reef","path":"/tmp"}`)
	do(t, http.MethodPost, ts.URL+"/spaces", `{"projectId":1,"name":"c","backend":"claude"}`)
	if _, err := st.CreateTurn(ctx, store.Turn{SpaceID: 1, Role: "developer", Backend: "claude"}); err != nil {
		t.Fatal(err)
	}
	agentCall, err := st.CreateToolCall(ctx, 1, 0, "Agent", `{"subagent_type":"comment-sicko"}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateToolCall(ctx, 1, agentCall.ID, "Read", `{"file_path":"a.go"}`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateToolCall(ctx, 1, 0, "Bash", `{"command":"ls"}`); err != nil {
		t.Fatal(err)
	}

	status, body := do(t, http.MethodGet, ts.URL+"/spaces/1/inspector", "")
	if status != 200 {
		t.Fatalf("status = %d, body = %s", status, body)
	}
	want := `"toolCalls":[{"name":"Agent","input":"{\"subagent_type\":\"comment-sicko\"}","result":"",` +
		`"children":[{"name":"Read","input":"{\"file_path\":\"a.go\"}","result":""}]},` +
		`{"name":"Bash","input":"{\"command\":\"ls\"}","result":""}]`
	if !strings.Contains(body, want) {
		t.Errorf("body = %s, want it to contain %s", body, want)
	}
}

func TestGetInspectorNullCostForCodex(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	h := New(st, testMemory(t, st), t.TempDir(), "/bin/reef")
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	t.Cleanup(h.Close)

	do(t, http.MethodPost, ts.URL+"/projects", `{"name":"reef","path":"/tmp"}`)
	do(t, http.MethodPost, ts.URL+"/spaces", `{"projectId":1,"name":"c","backend":"codex"}`)
	if _, err := st.CreateTurn(ctx, store.Turn{SpaceID: 1, Role: "developer", Backend: "codex"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.EndTurn(ctx, store.Turn{
		ID: 1, Status: "done", Model: "gpt-6-codex", Prompt: "do it",
		Usage: agent.Usage{InputTokens: 10, OutputTokens: 20},
	}); err != nil {
		t.Fatal(err)
	}

	status, body := do(t, http.MethodGet, ts.URL+"/spaces/1/inspector", "")
	if status != 200 {
		t.Fatalf("status = %d, body = %s", status, body)
	}
	wantRole := `"roles":[{"role":"developer","turns":1,"usage":{"inputTokens":10,"outputTokens":20,` +
		`"cacheReadTokens":0,"cacheWriteTokens":0},"costUsd":null}]`
	if !strings.Contains(body, wantRole) {
		t.Errorf("body = %s, want it to contain %s", body, wantRole)
	}
}
