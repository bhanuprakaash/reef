package server

import (
	"net/http"
	"strings"
	"testing"
)

func TestFeatureStatus(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		give       string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "unknown project",
			method:     http.MethodPost,
			path:       "/features",
			give:       `{"projectId":9,"slug":"a","name":"A"}`,
			wantStatus: 400,
			wantBody:   `{"error":"invalid: unknown project 9"}`,
		},
		{
			name:       "missing slug",
			method:     http.MethodPost,
			path:       "/features",
			give:       `{"projectId":1,"name":"A"}`,
			wantStatus: 400,
			wantBody:   `{"error":"invalid: feature slug is required"}`,
		},
		{
			name:       "bad project filter",
			method:     http.MethodGet,
			path:       "/features?project=x",
			wantStatus: 400,
			wantBody:   `{"error":"invalid: project \"x\" is not a number"}`,
		},
		{
			name:       "unknown feature",
			method:     http.MethodGet,
			path:       "/features/9",
			wantStatus: 404,
			wantBody:   `{"error":"feature 9: not found"}`,
		},
		{
			name:       "brief of unknown feature",
			method:     http.MethodPut,
			path:       "/features/9/brief",
			give:       `{"body":"b","author":"me"}`,
			wantStatus: 404,
			wantBody:   `{"error":"feature 9: not found"}`,
		},
		{
			name:       "no brief yet",
			method:     http.MethodGet,
			path:       "/features/1/brief",
			wantStatus: 404,
			wantBody:   `{"error":"brief of feature 1: not found"}`,
		},
		{
			name:       "no notes for role",
			method:     http.MethodGet,
			path:       "/features/1/notes/tester",
			wantStatus: 404,
			wantBody:   `{"error":"notes of tester on feature 1: not found"}`,
		},
		{
			name:       "wrong method on notes",
			method:     http.MethodDelete,
			path:       "/features/1/notes/tester",
			wantStatus: 405,
			wantBody:   `{"error":"method not allowed"}`,
		},
		{
			name:       "wrong method on versions",
			method:     http.MethodPut,
			path:       "/features/1/brief/versions",
			wantStatus: 405,
			wantBody:   `{"error":"method not allowed"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := newTestServer(t)
			do(t, http.MethodPost, ts.URL+"/projects", `{"name":"reef","path":"/tmp/reef"}`)
			do(t, http.MethodPost, ts.URL+"/features", `{"projectId":1,"slug":"first","name":"First"}`)
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

func TestFeatureRoundTrip(t *testing.T) {
	ts := newTestServer(t)
	do(t, http.MethodPost, ts.URL+"/projects", `{"name":"reef","path":"/tmp/reef"}`)
	do(t, http.MethodPost, ts.URL+"/projects", `{"name":"other","path":"/tmp/other"}`)

	status, body := do(t, http.MethodPost, ts.URL+"/features", `{"projectId":1,"slug":"auth","name":"Auth"}`)
	if status != 201 || !strings.HasPrefix(body,
		`{"id":1,"projectId":1,"slug":"auth","name":"Auth","goal":"","stage":"ideation","createdAt":"`) {
		t.Fatalf("create = %d %s", status, body)
	}
	do(t, http.MethodPost, ts.URL+"/features", `{"projectId":2,"slug":"auth","name":"Auth"}`)

	status, body = do(t, http.MethodPost, ts.URL+"/features", `{"projectId":1,"slug":"auth","name":"Again"}`)
	if status != 409 || body != `{"error":"feature slug \"auth\" is taken in its project: conflict"}` {
		t.Fatalf("duplicate slug = %d %s", status, body)
	}

	status, body = do(t, http.MethodGet, ts.URL+"/features?project=1", "")
	if status != 200 || strings.Count(body, `"slug":"auth"`) != 1 || !strings.Contains(body, `"projectId":1`) {
		t.Fatalf("filtered list = %d %s", status, body)
	}

	status, body = do(t, http.MethodPut, ts.URL+"/features/1", `{"slug":"auth","name":"Auth","goal":"sign in","stage":"build"}`)
	if status != 200 || !strings.Contains(body, `"goal":"sign in","stage":"build"`) {
		t.Fatalf("update = %d %s", status, body)
	}

	status, body = do(t, http.MethodPut, ts.URL+"/features/1/brief", `{"body":"v1","author":"lead"}`)
	if status != 201 || !strings.HasPrefix(body, `{"id":1,"featureId":1,"body":"v1","author":"lead","createdAt":"`) {
		t.Fatalf("brief v1 = %d %s", status, body)
	}
	do(t, http.MethodPut, ts.URL+"/features/1/brief", `{"body":"v2","author":"lead"}`)
	status, body = do(t, http.MethodGet, ts.URL+"/features/1/brief", "")
	if status != 200 || !strings.Contains(body, `"body":"v2"`) {
		t.Fatalf("latest brief = %d %s", status, body)
	}
	status, body = do(t, http.MethodGet, ts.URL+"/features/1/brief/versions", "")
	if status != 200 || !strings.Contains(body, `"body":"v1"`) || !strings.Contains(body, `"body":"v2"`) {
		t.Fatalf("versions = %d %s", status, body)
	}

	do(t, http.MethodPut, ts.URL+"/features/1/notes/architect", `{"body":"one"}`)
	status, body = do(t, http.MethodPut, ts.URL+"/features/1/notes/architect", `{"body":"two"}`)
	if status != 200 || !strings.HasPrefix(body, `{"featureId":1,"role":"architect","body":"two","updatedAt":"`) {
		t.Fatalf("notes = %d %s", status, body)
	}

	if status, _ = do(t, http.MethodDelete, ts.URL+"/projects/1", ""); status != 204 {
		t.Fatalf("delete project = %d", status)
	}
	for _, path := range []string{"/features/1", "/features/1/brief", "/features/1/notes/architect"} {
		if status, body = do(t, http.MethodGet, ts.URL+path, ""); status != 404 {
			t.Errorf("GET %s after project delete = %d %s, want 404", path, status, body)
		}
	}
}
