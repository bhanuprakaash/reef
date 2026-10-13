package server

import (
	"net/http"
	"strings"
	"testing"
)

func TestTaskStatus(t *testing.T) {
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
			path:       "/tasks",
			give:       `{"projectId":9,"title":"a"}`,
			wantStatus: 400,
			wantBody:   `{"error":"invalid: unknown project 9"}`,
		},
		{
			name:       "missing title",
			method:     http.MethodPost,
			path:       "/tasks",
			give:       `{"projectId":1}`,
			wantStatus: 400,
			wantBody:   `{"error":"invalid: task title is required"}`,
		},
		{
			name:       "unknown status on create",
			method:     http.MethodPost,
			path:       "/tasks",
			give:       `{"projectId":1,"title":"a","status":"blocked"}`,
			wantStatus: 400,
			wantBody:   `{"error":"invalid: unknown status \"blocked\""}`,
		},
		{
			name:       "unknown status on update",
			method:     http.MethodPut,
			path:       "/tasks/1",
			give:       `{"title":"a","status":"blocked"}`,
			wantStatus: 400,
			wantBody:   `{"error":"invalid: unknown status \"blocked\""}`,
		},
		{
			name:       "feature from another project on create",
			method:     http.MethodPost,
			path:       "/tasks",
			give:       `{"projectId":1,"featureId":1,"title":"a"}`,
			wantStatus: 400,
			wantBody:   `{"error":"invalid: feature 1 is not in project 1"}`,
		},
		{
			name:       "feature from another project on update",
			method:     http.MethodPut,
			path:       "/tasks/1",
			give:       `{"title":"a","featureId":1}`,
			wantStatus: 400,
			wantBody:   `{"error":"invalid: feature 1 is not in project 1"}`,
		},
		{
			name:       "unknown feature",
			method:     http.MethodPost,
			path:       "/tasks",
			give:       `{"projectId":1,"featureId":9,"title":"a"}`,
			wantStatus: 400,
			wantBody:   `{"error":"invalid: unknown feature 9"}`,
		},
		{
			name:       "bad status filter",
			method:     http.MethodGet,
			path:       "/tasks?status=blocked",
			wantStatus: 400,
			wantBody:   `{"error":"invalid: unknown status \"blocked\""}`,
		},
		{
			name:       "bad project filter",
			method:     http.MethodGet,
			path:       "/tasks?project=x",
			wantStatus: 400,
			wantBody:   `{"error":"invalid: project \"x\" is not a number"}`,
		},
		{
			name:       "unknown task",
			method:     http.MethodGet,
			path:       "/tasks/9",
			wantStatus: 404,
			wantBody:   `{"error":"task 9: not found"}`,
		},
		{
			name:       "update unknown task",
			method:     http.MethodPut,
			path:       "/tasks/9",
			give:       `{"title":"a"}`,
			wantStatus: 404,
			wantBody:   `{"error":"task 9: not found"}`,
		},
		{
			name:       "wrong method on list",
			method:     http.MethodDelete,
			path:       "/tasks",
			wantStatus: 405,
			wantBody:   `{"error":"method not allowed"}`,
		},
		{
			name:       "wrong method on one",
			method:     http.MethodPost,
			path:       "/tasks/1",
			wantStatus: 405,
			wantBody:   `{"error":"method not allowed"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := newTestServer(t)
			do(t, http.MethodPost, ts.URL+"/projects", `{"name":"reef","path":"/tmp"}`)
			do(t, http.MethodPost, ts.URL+"/projects", `{"name":"other","path":"/"}`)
			do(t, http.MethodPost, ts.URL+"/features", `{"projectId":2,"slug":"far","name":"Far"}`)
			do(t, http.MethodPost, ts.URL+"/tasks", `{"projectId":1,"title":"first"}`)
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

func TestTaskRoundTrip(t *testing.T) {
	ts := newTestServer(t)
	do(t, http.MethodPost, ts.URL+"/projects", `{"name":"reef","path":"/tmp"}`)
	do(t, http.MethodPost, ts.URL+"/projects", `{"name":"other","path":"/"}`)
	do(t, http.MethodPost, ts.URL+"/features", `{"projectId":1,"slug":"auth","name":"Auth"}`)

	status, body := do(t, http.MethodPost, ts.URL+"/tasks", `{"projectId":1,"featureId":1,"title":"login"}`)
	if status != 201 || !strings.HasPrefix(body,
		`{"id":1,"projectId":1,"featureId":1,"title":"login","body":"","status":"todo","createdAt":"`) {
		t.Fatalf("create = %d %s", status, body)
	}
	do(t, http.MethodPost, ts.URL+"/tasks", `{"projectId":1,"title":"plain","status":"done"}`)
	do(t, http.MethodPost, ts.URL+"/tasks", `{"projectId":2,"title":"elsewhere"}`)

	for _, tt := range []struct {
		query string
		want  []string
	}{
		{"", []string{"login", "plain", "elsewhere"}},
		{"?project=1", []string{"login", "plain"}},
		{"?project=1&feature=1", []string{"login"}},
		{"?status=done", []string{"plain"}},
	} {
		status, body = do(t, http.MethodGet, ts.URL+"/tasks"+tt.query, "")
		if status != 200 || strings.Count(body, `"title"`) != len(tt.want) {
			t.Errorf("list %q = %d %s", tt.query, status, body)
		}
		for _, title := range tt.want {
			if !strings.Contains(body, `"title":"`+title+`"`) {
				t.Errorf("list %q = %s, want %s", tt.query, body, title)
			}
		}
	}

	status, body = do(t, http.MethodPut, ts.URL+"/tasks/1", `{"title":"login","body":"oauth","status":"doing","featureId":1}`)
	if status != 200 || !strings.Contains(body, `"body":"oauth","status":"doing"`) {
		t.Fatalf("update = %d %s", status, body)
	}

	if status, _ = do(t, http.MethodDelete, ts.URL+"/features/1", ""); status != 204 {
		t.Fatalf("delete feature = %d", status)
	}
	status, body = do(t, http.MethodGet, ts.URL+"/tasks/1", "")
	if status != 200 || !strings.Contains(body, `"projectId":1,"featureId":null,"title":"login"`) {
		t.Fatalf("task after feature delete = %d %s", status, body)
	}
	status, body = do(t, http.MethodGet, ts.URL+"/tasks?project=1", "")
	if status != 200 || !strings.Contains(body, `"title":"login"`) {
		t.Fatalf("project list after feature delete = %d %s", status, body)
	}

	if status, _ = do(t, http.MethodDelete, ts.URL+"/tasks/1", ""); status != 204 {
		t.Fatalf("delete task = %d", status)
	}
	if status, _ = do(t, http.MethodGet, ts.URL+"/tasks/1", ""); status != 404 {
		t.Fatalf("task after delete = %d, want 404", status)
	}
}
