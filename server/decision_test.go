package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// decisionFixture has work items 1 and 2, each with a Space holding one message.
func decisionFixture(t *testing.T) *httptest.Server {
	t.Helper()
	ts := newTestServer(t)
	do(t, http.MethodPost, ts.URL+"/projects", `{"name":"reef","path":"/tmp"}`)
	do(t, http.MethodPost, ts.URL+"/work-items", `{"projectId":1,"title":"Auth"}`)
	do(t, http.MethodPost, ts.URL+"/work-items", `{"projectId":1,"title":"Billing"}`)
	do(t, http.MethodPost, ts.URL+"/spaces", `{"projectId":1,"workItemId":1,"name":"auth","backend":"claude"}`)
	do(t, http.MethodPost, ts.URL+"/spaces", `{"projectId":1,"workItemId":2,"name":"billing","backend":"claude"}`)
	do(t, http.MethodPost, ts.URL+"/messages", `{"spaceId":1,"author":"user","body":"use sessions"}`)
	do(t, http.MethodPost, ts.URL+"/messages", `{"spaceId":2,"author":"user","body":"use stripe"}`)
	return ts
}

func TestDecisionStatus(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		give       string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "message of another work item",
			method:     http.MethodPost,
			path:       "/decisions",
			give:       `{"workItemId":1,"messageId":2,"body":"d","author":"user"}`,
			wantStatus: 400,
			wantBody:   `{"error":"invalid: message 2 is not in a Space of work item 1"}`,
		},
		{
			name:       "unknown message",
			method:     http.MethodPost,
			path:       "/decisions",
			give:       `{"workItemId":1,"messageId":9,"body":"d","author":"user"}`,
			wantStatus: 400,
			wantBody:   `{"error":"invalid: message 9 is not in a Space of work item 1"}`,
		},
		{
			name:       "unknown work item",
			method:     http.MethodPost,
			path:       "/decisions",
			give:       `{"workItemId":9,"body":"d","author":"user"}`,
			wantStatus: 400,
			wantBody:   `{"error":"invalid: unknown work item 9"}`,
		},
		{
			name:       "missing work item",
			method:     http.MethodPost,
			path:       "/decisions",
			give:       `{"body":"d","author":"user"}`,
			wantStatus: 400,
			wantBody:   `{"error":"invalid: work item is required"}`,
		},
		{
			name:       "missing author",
			method:     http.MethodPost,
			path:       "/decisions",
			give:       `{"workItemId":1,"body":"d"}`,
			wantStatus: 400,
			wantBody:   `{"error":"invalid: decision author is required"}`,
		},
		{
			name:       "list needs a work item",
			method:     http.MethodGet,
			path:       "/decisions",
			wantStatus: 400,
			wantBody:   `{"error":"invalid: work item is required"}`,
		},
		{
			name:       "update",
			method:     http.MethodPut,
			path:       "/decisions/1",
			wantStatus: 405,
			wantBody:   `{"error":"method not allowed"}`,
		},
		{
			name:       "patch",
			method:     http.MethodPatch,
			path:       "/decisions/1",
			wantStatus: 405,
			wantBody:   `{"error":"method not allowed"}`,
		},
		{
			name:       "delete",
			method:     http.MethodDelete,
			path:       "/decisions/1",
			wantStatus: 405,
			wantBody:   `{"error":"method not allowed"}`,
		},
		{
			name:       "delete collection",
			method:     http.MethodDelete,
			path:       "/decisions",
			wantStatus: 405,
			wantBody:   `{"error":"method not allowed"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := decisionFixture(t)
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

func TestDecisionRoundTrip(t *testing.T) {
	ts := decisionFixture(t)

	status, body := do(t, http.MethodPost, ts.URL+"/decisions",
		`{"workItemId":1,"messageId":1,"body":"sessions, not JWT","why":"revocation","author":"architect"}`)
	if status != 201 || !strings.HasPrefix(body,
		`{"id":1,"workItemId":1,"messageId":1,"body":"sessions, not JWT","why":"revocation","author":"architect","createdAt":"`) {
		t.Fatalf("with message = %d %s", status, body)
	}
	status, body = do(t, http.MethodPost, ts.URL+"/decisions", `{"workItemId":1,"body":"ship behind a flag","author":"user"}`)
	if status != 201 || !strings.HasPrefix(body,
		`{"id":2,"workItemId":1,"messageId":null,"body":"ship behind a flag","why":"","author":"user","createdAt":"`) {
		t.Fatalf("without message = %d %s", status, body)
	}

	status, body = do(t, http.MethodGet, ts.URL+"/decisions?workItem=1", "")
	first, second := strings.Index(body, `"id":1,`), strings.Index(body, `"id":2,`)
	if status != 200 || first < 0 || second < first {
		t.Fatalf("list = %d %s", status, body)
	}
	status, body = do(t, http.MethodGet, ts.URL+"/decisions?workItem=2", "")
	if status != 200 || body != "[]" {
		t.Fatalf("other work item = %d %s", status, body)
	}
	status, body = do(t, http.MethodGet, ts.URL+"/decisions/2", "")
	if status != 200 || !strings.Contains(body, `"body":"ship behind a flag"`) {
		t.Fatalf("get = %d %s", status, body)
	}

	if status, _ = do(t, http.MethodDelete, ts.URL+"/work-items/1", ""); status != 204 {
		t.Fatalf("delete work item = %d", status)
	}
	if status, body = do(t, http.MethodGet, ts.URL+"/decisions/1", ""); status != 404 {
		t.Errorf("decision after work item delete = %d %s, want 404", status, body)
	}
}
