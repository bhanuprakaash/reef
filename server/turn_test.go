package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/store"
)

func TestTurnStatus(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		give       string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "unknown channel",
			method:     http.MethodPost,
			path:       "/turns",
			give:       `{"channelId":9,"role":"architect","backend":"claude"}`,
			wantStatus: 400,
			wantBody:   `{"error":"invalid: unknown channel 9"}`,
		},
		{
			name:       "missing role",
			method:     http.MethodPost,
			path:       "/turns",
			give:       `{"channelId":1,"backend":"claude"}`,
			wantStatus: 400,
			wantBody:   `{"error":"invalid: turn role is required"}`,
		},
		{
			name:       "unknown backend",
			method:     http.MethodPost,
			path:       "/turns",
			give:       `{"channelId":1,"role":"architect","backend":"gpt"}`,
			wantStatus: 400,
			wantBody:   `{"error":"invalid: unknown backend \"gpt\""}`,
		},
		{
			name:       "patch to running",
			method:     http.MethodPatch,
			path:       "/turns/1",
			give:       `{"status":"running"}`,
			wantStatus: 400,
			wantBody:   `{"error":"invalid: turn status must be done or failed, got \"running\""}`,
		},
		{
			name:       "patch session only",
			method:     http.MethodPatch,
			path:       "/turns/1",
			give:       `{"sessionId":"abc"}`,
			wantStatus: 200,
			wantBody:   `"sessionId":"abc","status":"running","error":""`,
		},
		{
			name:       "patch error without status",
			method:     http.MethodPatch,
			path:       "/turns/1",
			give:       `{"sessionId":"s","error":"boom"}`,
			wantStatus: 400,
			wantBody:   `{"error":"invalid: error and usage need a status"}`,
		},
		{
			name:       "patch nothing",
			method:     http.MethodPatch,
			path:       "/turns/1",
			give:       `{}`,
			wantStatus: 400,
			wantBody:   `{"error":"invalid: sessionId is required without a status"}`,
		},
		{
			name:       "patch session of unknown turn",
			method:     http.MethodPatch,
			path:       "/turns/9",
			give:       `{"sessionId":"abc"}`,
			wantStatus: 404,
			wantBody:   `{"error":"turn 9: not found"}`,
		},
		{
			name:       "negative usage",
			method:     http.MethodPatch,
			path:       "/turns/1",
			give:       `{"status":"done","usage":{"inputTokens":-5}}`,
			wantStatus: 400,
			wantBody:   `{"error":"invalid: usage counts cannot be negative"}`,
		},
		{
			name:       "patch unknown turn",
			method:     http.MethodPatch,
			path:       "/turns/9",
			give:       `{"status":"done"}`,
			wantStatus: 404,
			wantBody:   `{"error":"turn 9: not found"}`,
		},
		{
			name:       "bad status filter",
			method:     http.MethodGet,
			path:       "/turns?status=stuck",
			wantStatus: 400,
			wantBody:   `{"error":"invalid: unknown status \"stuck\""}`,
		},
		{
			name:       "delete",
			method:     http.MethodDelete,
			path:       "/turns/1",
			wantStatus: 405,
			wantBody:   `{"error":"method not allowed"}`,
		},
		{
			name:       "put on list",
			method:     http.MethodPut,
			path:       "/turns",
			wantStatus: 405,
			wantBody:   `{"error":"method not allowed"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := newTestServer(t)
			do(t, http.MethodPost, ts.URL+"/projects", `{"name":"reef","path":"/tmp"}`)
			do(t, http.MethodPost, ts.URL+"/channels", `{"projectId":1,"name":"c","backend":"claude"}`)
			do(t, http.MethodPost, ts.URL+"/turns", `{"channelId":1,"role":"architect","backend":"claude"}`)
			status, body := do(t, tt.method, ts.URL+tt.path, tt.give)
			if status != tt.wantStatus {
				t.Errorf("status = %d, want %d", status, tt.wantStatus)
			}
			if !strings.Contains(body, tt.wantBody) {
				t.Errorf("body = %s, want it to contain %s", body, tt.wantBody)
			}
		})
	}
}

func TestTurnRoundTrip(t *testing.T) {
	ts := newTestServer(t)
	do(t, http.MethodPost, ts.URL+"/projects", `{"name":"reef","path":"/tmp"}`)
	do(t, http.MethodPost, ts.URL+"/channels", `{"projectId":1,"name":"c","backend":"claude"}`)

	status, body := do(t, http.MethodPost, ts.URL+"/turns", `{"channelId":1,"role":"architect","backend":"claude","status":"done"}`)
	if status != 201 || !strings.HasPrefix(body,
		`{"id":1,"channelId":1,"role":"architect","backend":"claude","sessionId":"","status":"running","error":"","usage":{"inputTokens":0,"outputTokens":0,"cacheReadTokens":0,"cacheWriteTokens":0},"startedAt":"`) ||
		!strings.HasSuffix(body, `"endedAt":null,"wakeText":"","wakeParent":null,"retried":false,"sent":false,"checkpoint":null,"model":"","costUsd":null}`) {
		t.Fatalf("create = %d %s", status, body)
	}

	status, body = do(t, http.MethodPatch, ts.URL+"/turns/1",
		`{"status":"done","sessionId":"s-1","usage":{"inputTokens":10,"outputTokens":20,"cacheReadTokens":30,"cacheWriteTokens":40}}`)
	if status != 200 || !strings.Contains(body,
		`"sessionId":"s-1","status":"done","error":"","usage":{"inputTokens":10,"outputTokens":20,"cacheReadTokens":30,"cacheWriteTokens":40}`) ||
		strings.Contains(body, `"endedAt":null`) {
		t.Fatalf("patch = %d %s", status, body)
	}

	status, body = do(t, http.MethodPatch, ts.URL+"/turns/1", `{"status":"failed","error":"late"}`)
	if status != 400 || body != `{"error":"invalid: turn 1 already ended"}` {
		t.Fatalf("second patch = %d %s", status, body)
	}

	status, body = do(t, http.MethodPatch, ts.URL+"/turns/1", `{"sessionId":"abc"}`)
	if status != 400 || body != `{"error":"invalid: turn 1 already ended"}` {
		t.Fatalf("session patch on ended turn = %d %s", status, body)
	}

	status, body = do(t, http.MethodGet, ts.URL+"/turns/1", "")
	if status != 200 || !strings.Contains(body, `"status":"done"`) || !strings.Contains(body, `"inputTokens":10`) {
		t.Fatalf("get = %d %s", status, body)
	}

	do(t, http.MethodPost, ts.URL+"/turns", `{"channelId":1,"role":"developer","backend":"codex"}`)
	for _, tt := range []struct {
		query string
		want  int
	}{
		{"", 2},
		{"?channel=1", 2},
		{"?channel=2", 0},
		{"?status=running", 1},
		{"?channel=1&status=done", 1},
	} {
		status, body = do(t, http.MethodGet, ts.URL+"/turns"+tt.query, "")
		if status != 200 || strings.Count(body, `"role"`) != tt.want {
			t.Errorf("list %q = %d %s, want %d turns", tt.query, status, body, tt.want)
		}
	}
}

func TestTurnHasNoPrompt(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	h := New(st, t.TempDir(), "/bin/reef")
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	t.Cleanup(h.Close)

	do(t, http.MethodPost, ts.URL+"/projects", `{"name":"reef","path":"/tmp"}`)
	do(t, http.MethodPost, ts.URL+"/channels", `{"projectId":1,"name":"c","backend":"claude"}`)
	if _, err := st.CreateTurn(ctx, store.Turn{ChannelID: 1, Role: "architect", Backend: "claude"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.EndTurn(ctx, store.Turn{ID: 1, Status: "done", Prompt: "do it"}); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{"/turns", "/turns/1"} {
		status, body := do(t, http.MethodGet, ts.URL+path, "")
		if status != 200 || strings.Contains(body, "prompt") {
			t.Errorf("get %s = %d %s, want no prompt field", path, status, body)
		}
	}

	status, body := do(t, http.MethodGet, ts.URL+"/channels/1/inspector", "")
	if status != 200 || !strings.Contains(body, `"prompt":"do it"`) {
		t.Errorf("inspector = %d %s, want prompt \"do it\"", status, body)
	}
}
