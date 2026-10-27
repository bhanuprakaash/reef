package server

import (
	"net/http"
	"strings"
	"testing"
)

func TestMemoryRoundTrip(t *testing.T) {
	ts := newTestServer(t)

	status, body := do(t, http.MethodGet, ts.URL+"/memory?scope=work_item:1", "")
	if status != 200 || body != `[]` {
		t.Fatalf("empty = %d %s", status, body)
	}

	status, body = do(t, http.MethodPost, ts.URL+"/memory",
		`{"kind":"decision","body":"sessions, not JWT","why":"revocation","scope":"work_item:1"}`)
	if status != 201 || !strings.HasPrefix(body, `{"id":1,"kind":"decision","body":"sessions, not JWT","why":"revocation","scope":"work_item:1","author":"user","role":"user","status":"accepted","retiredAt":null,"replacedBy":null,"createdAt":"`) ||
		!strings.HasSuffix(body, `,"links":[]}`) {
		t.Fatalf("create = %d %s", status, body)
	}
	do(t, http.MethodPost, ts.URL+"/memory", `{"kind":"convention","body":"tabs","scope":"work_item:1"}`)
	do(t, http.MethodPost, ts.URL+"/memory", `{"kind":"fact","body":"other feature","scope":"work_item:2"}`)

	status, body = do(t, http.MethodGet, ts.URL+"/memory?scope=work_item:1", "")
	newest, oldest := strings.Index(body, `"body":"tabs"`), strings.Index(body, `"body":"sessions, not JWT"`)
	if status != 200 || newest < 0 || oldest < newest || strings.Contains(body, "other feature") {
		t.Fatalf("list = %d %s", status, body)
	}

	status, body = do(t, http.MethodPost, ts.URL+"/memory/1/retire", "")
	if status != 204 || body != "" {
		t.Fatalf("retire = %d %s", status, body)
	}
	status, body = do(t, http.MethodGet, ts.URL+"/memory?scope=work_item:1", "")
	if status != 200 || strings.Contains(body, "sessions, not JWT") || !strings.Contains(body, `"body":"tabs"`) {
		t.Fatalf("list after retire = %d %s", status, body)
	}
}

func TestMemoryStatus(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		give       string
		wantStatus int
		wantBody   string
	}{
		{
			name: "list needs a scope", method: http.MethodGet, path: "/memory",
			wantStatus: 400, wantBody: `{"error":"invalid: scope \"\" is not kind:id"}`,
		},
		{
			name: "create with a bad scope", method: http.MethodPost, path: "/memory",
			give:       `{"kind":"fact","body":"b","scope":"7"}`,
			wantStatus: 400, wantBody: `{"error":"invalid: scope \"7\" is not kind:id"}`,
		},
		{
			name: "create with an unknown kind", method: http.MethodPost, path: "/memory",
			give:       `{"kind":"idea","body":"b","scope":"project:1"}`,
			wantStatus: 400, wantBody: `{"error":"invalid: unknown kind \"idea\""}`,
		},
		{
			name: "create without a body", method: http.MethodPost, path: "/memory",
			give:       `{"kind":"fact","scope":"project:1"}`,
			wantStatus: 400, wantBody: `{"error":"invalid: body is required"}`,
		},
		{
			name: "retire an unknown item", method: http.MethodPost, path: "/memory/9/retire",
			wantStatus: 404, wantBody: `{"error":"memory 9: not found"}`,
		},
		{
			name: "retire with a bad id", method: http.MethodPost, path: "/memory/x/retire",
			wantStatus: 400, wantBody: `{"error":"invalid: id \"x\" is not a number"}`,
		},
		{
			name: "delete the collection", method: http.MethodDelete, path: "/memory",
			wantStatus: 405, wantBody: `{"error":"method not allowed"}`,
		},
		{
			name: "get a retire", method: http.MethodGet, path: "/memory/1/retire",
			wantStatus: 405, wantBody: `{"error":"method not allowed"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := newTestServer(t)
			status, body := do(t, tt.method, ts.URL+tt.path, tt.give)
			if status != tt.wantStatus {
				t.Errorf("status = %d, want %d", status, tt.wantStatus)
			}
			if body != tt.wantBody {
				t.Errorf("body = %s, want %s", body, tt.wantBody)
			}
		})
	}
}

func TestMemoryRetireTwice(t *testing.T) {
	ts := newTestServer(t)
	do(t, http.MethodPost, ts.URL+"/memory", `{"kind":"fact","body":"b","scope":"project:1"}`)
	do(t, http.MethodPost, ts.URL+"/memory/1/retire", "")

	status, body := do(t, http.MethodPost, ts.URL+"/memory/1/retire", "")
	if status != 400 || body != `{"error":"invalid: memory 1 is already retired"}` {
		t.Fatalf("second retire = %d %s", status, body)
	}
}
