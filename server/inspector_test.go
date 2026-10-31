package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
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
	do(t, http.MethodPost, ts.URL+"/spaces", `{"projectId":1,"name":"c"}`)
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
	wantRole := `"roles":[{"role":"architect","botId":null,"turns":1,"usage":{"inputTokens":10,"outputTokens":20,` +
		`"cacheReadTokens":0,"cacheWriteTokens":0},"costUsd":0.05}]`
	wantTurn := `"status":"done","model":"claude-opus-5-5",` +
		`"usage":{"inputTokens":10,"outputTokens":20,"cacheReadTokens":0,"cacheWriteTokens":0},` +
		`"costUsd":0.05,"prompt":"do it","reply":"kept","toolCalls":[{"name":"Bash","input":"{\"command\":\"ls\"}",` +
		`"result":"a.txt\n"}]`
	if !strings.Contains(body, wantRole) || !strings.Contains(body, `"turns":[{"id":1,"role":"architect","botId":null,"startedAt":"`) ||
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
	do(t, http.MethodPost, ts.URL+"/spaces", `{"projectId":1,"name":"c"}`)
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

func TestGetInspectorKeysRolesByBotID(t *testing.T) {
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
	do(t, http.MethodPost, ts.URL+"/spaces", `{"projectId":1,"name":"c"}`)
	sam, err := st.CreateBot(ctx, store.Bot{Name: "sam", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateTurn(ctx, store.Turn{SpaceID: 1, BotID: &sam.ID, Role: "sam", Backend: "claude"}); err != nil {
		t.Fatal(err)
	}
	sam.Name = "sally"
	if _, err := st.UpdateBot(ctx, sam); err != nil {
		t.Fatal(err)
	}
	other, err := st.CreateBot(ctx, store.Bot{Name: "sam", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range []struct {
		id   int64
		role string
	}{{sam.ID, "sally"}, {other.ID, "sam"}} {
		if _, err := st.CreateTurn(ctx, store.Turn{SpaceID: 1, BotID: &b.id, Role: b.role, Backend: "claude"}); err != nil {
			t.Fatal(err)
		}
	}

	status, body := do(t, http.MethodGet, ts.URL+"/spaces/1/inspector", "")
	if status != 200 {
		t.Fatalf("status = %d, body = %s", status, body)
	}
	for _, want := range []string{
		`"roles":[{"role":"sally","botId":` + strconv.FormatInt(sam.ID, 10) + `,"turns":2,`,
		`{"role":"sam","botId":` + strconv.FormatInt(other.ID, 10) + `,"turns":1,`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body = %s, want it to contain %s", body, want)
		}
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
	do(t, http.MethodPost, ts.URL+"/spaces", `{"projectId":1,"name":"c"}`)
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
	wantRole := `"roles":[{"role":"developer","botId":null,"turns":1,"usage":{"inputTokens":10,"outputTokens":20,` +
		`"cacheReadTokens":0,"cacheWriteTokens":0},"costUsd":null}]`
	if !strings.Contains(body, wantRole) {
		t.Errorf("body = %s, want it to contain %s", body, wantRole)
	}
}
