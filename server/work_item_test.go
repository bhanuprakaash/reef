package server

import (
	"net/http"
	"strings"
	"testing"
)

func TestWorkItemStatus(t *testing.T) {
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
			path:       "/work-items",
			give:       `{"projectId":9,"title":"A"}`,
			wantStatus: 400,
			wantBody:   `{"error":"invalid: unknown project 9"}`,
		},
		{
			name:       "missing title",
			method:     http.MethodPost,
			path:       "/work-items",
			give:       `{"projectId":1}`,
			wantStatus: 400,
			wantBody:   `{"error":"invalid: work item title is required"}`,
		},
		{
			name:       "unknown type",
			method:     http.MethodPost,
			path:       "/work-items",
			give:       `{"projectId":1,"title":"A","type":"epic"}`,
			wantStatus: 400,
			wantBody:   `{"error":"invalid: unknown type \"epic\""}`,
		},
		{
			name:       "queued status on create",
			method:     http.MethodPost,
			path:       "/work-items",
			give:       `{"projectId":1,"title":"A","ownerBotId":1,"status":"queued","queuePosition":3}`,
			wantStatus: 201,
			wantBody:   `"status":"backlog","proposedBy":"","queuePosition":null`,
		},
		{
			name:       "review status on create",
			method:     http.MethodPost,
			path:       "/work-items",
			give:       `{"projectId":1,"title":"A","status":"review"}`,
			wantStatus: 201,
			wantBody:   `"status":"backlog"`,
		},
		{
			name:       "unknown parent",
			method:     http.MethodPost,
			path:       "/work-items",
			give:       `{"projectId":1,"title":"A","parentId":9}`,
			wantStatus: 400,
			wantBody:   `{"error":"invalid: unknown work item 9"}`,
		},
		{
			name:       "empty acceptance line",
			method:     http.MethodPost,
			path:       "/work-items",
			give:       `{"projectId":1,"title":"A","acceptanceLines":[{"text":""}]}`,
			wantStatus: 400,
			wantBody:   `{"error":"invalid: acceptance line 1 has no text"}`,
		},
		{
			name:       "bad project filter",
			method:     http.MethodGet,
			path:       "/work-items?project=x",
			wantStatus: 400,
			wantBody:   `{"error":"invalid: project \"x\" is not a number"}`,
		},
		{
			name:       "bad status filter",
			method:     http.MethodGet,
			path:       "/work-items?status=open",
			wantStatus: 400,
			wantBody:   `{"error":"invalid: unknown status \"open\""}`,
		},
		{
			name:       "unknown work item",
			method:     http.MethodGet,
			path:       "/work-items/9",
			wantStatus: 404,
			wantBody:   `{"error":"work item 9: not found"}`,
		},
		{
			name:       "brief of unknown work item",
			method:     http.MethodPut,
			path:       "/work-items/9/brief",
			give:       `{"body":"b","author":"me"}`,
			wantStatus: 404,
			wantBody:   `{"error":"work item 9: not found"}`,
		},
		{
			name:       "put notes on unknown work item",
			method:     http.MethodPut,
			path:       "/work-items/9/notes/tester",
			give:       `{"body":"n"}`,
			wantStatus: 404,
			wantBody:   `{"error":"work item 9: not found"}`,
		},
		{
			name:       "no brief yet",
			method:     http.MethodGet,
			path:       "/work-items/1/brief",
			wantStatus: 404,
			wantBody:   `{"error":"brief of work item 1: not found"}`,
		},
		{
			name:       "no notes for role",
			method:     http.MethodGet,
			path:       "/work-items/1/notes/tester",
			wantStatus: 404,
			wantBody:   `{"error":"notes of tester on work item 1: not found"}`,
		},
		{
			name:       "delete missing notes",
			method:     http.MethodDelete,
			path:       "/work-items/1/notes/tester",
			wantStatus: 404,
			wantBody:   `{"error":"notes of tester on work item 1: not found"}`,
		},
		{
			name:       "wrong method on notes",
			method:     http.MethodPatch,
			path:       "/work-items/1/notes/tester",
			wantStatus: 405,
			wantBody:   `{"error":"method not allowed"}`,
		},
		{
			name:       "wrong method on versions",
			method:     http.MethodPut,
			path:       "/work-items/1/brief/versions",
			wantStatus: 405,
			wantBody:   `{"error":"method not allowed"}`,
		},
		{
			name:       "features route is gone",
			method:     http.MethodGet,
			path:       "/features",
			wantStatus: 404,
			wantBody:   `{"error":"not found"}`,
		},
		{
			name:       "tasks route is gone",
			method:     http.MethodGet,
			path:       "/tasks",
			wantStatus: 404,
			wantBody:   `{"error":"not found"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := newTestServer(t)
			do(t, http.MethodPost, ts.URL+"/projects", `{"name":"reef","path":"/tmp"}`)
			do(t, http.MethodPost, ts.URL+"/work-items", `{"projectId":1,"title":"First"}`)
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

func TestWorkItemBugWithLinesAndSubItems(t *testing.T) {
	ts := newTestServer(t)
	do(t, http.MethodPost, ts.URL+"/projects", `{"name":"reef","path":"/tmp"}`)

	status, body := do(t, http.MethodPost, ts.URL+"/work-items",
		`{"projectId":1,"type":"bug","title":"Sign-in fails","description":"500 on submit",`+
			`"acceptanceLines":[{"text":"no 500"},{"text":"error shown"},{"text":"test added"}]}`)
	if status != 201 || !strings.HasPrefix(body,
		`{"id":1,"projectId":1,"parentId":null,"type":"bug","title":"Sign-in fails",`+
			`"description":"500 on submit","status":"backlog","proposedBy":"",`+
			`"queuePosition":null,"tokenBudget":1000000,"createdAt":"`) ||
		!strings.Contains(body, `"closedAt":null,"approvedAt":null,"approvalKey":null,"sentBack":false,"acceptanceLines":[`+
			`{"position":1,"text":"no 500","checked":false,"note":""},`+
			`{"position":2,"text":"error shown","checked":false,"note":""},`+
			`{"position":3,"text":"test added","checked":false,"note":""}],`+
			`"ownerBotId":null,"pendingOwnerBotId":null,"participantBotIds":[],"targets":[{"projectId":1,"projectName":"reef","proposedBy":"","authorizedBy":"user",`) {
		t.Fatalf("create bug = %d %s", status, body)
	}
	do(t, http.MethodPost, ts.URL+"/work-items", `{"projectId":1,"parentId":1,"type":"task","title":"Reproduce"}`)
	do(t, http.MethodPost, ts.URL+"/work-items", `{"projectId":1,"parentId":1,"type":"task","title":"Fix it","status":"review","queuePosition":4}`)
	do(t, http.MethodPatch, ts.URL+"/work-items/3", `{"status":"queued"}`)

	status, body = do(t, http.MethodGet, ts.URL+"/work-items?parent=1", "")
	if status != 200 || strings.Count(body, `"parentId":1`) != 2 ||
		!strings.Contains(body, `"id":2,"projectId":1,"parentId":1,"type":"task","title":"Reproduce"`) ||
		!strings.Contains(body, `"id":3,"projectId":1,"parentId":1,"type":"task","title":"Fix it"`) {
		t.Errorf("sub-items = %d %s, want items 2 and 3", status, body)
	}
	status, body = do(t, http.MethodGet, ts.URL+"/work-items?parent=1&status=queued&type=task", "")
	if status != 200 || strings.Count(body, `"title"`) != 1 || !strings.Contains(body, `"title":"Fix it"`) {
		t.Errorf("filtered sub-items = %d %s, want only Fix it", status, body)
	}

	status, body = do(t, http.MethodPost, ts.URL+"/work-items", `{"projectId":1,"parentId":2,"title":"Nested"}`)
	if status != 400 || body != `{"error":"invalid: work item 2 is a sub-item and cannot have sub-items"}` {
		t.Errorf("sub-item of a sub-item = %d %s, want 400 invalid", status, body)
	}
	status, body = do(t, http.MethodPut, ts.URL+"/work-items/2/brief", `{"body":"b","author":"lead"}`)
	if status != 400 || body != `{"error":"invalid: work item 2 is a sub-item and has no brief"}` {
		t.Errorf("brief on a sub-item = %d %s, want 400 invalid", status, body)
	}
}

func TestWorkItemRoundTrip(t *testing.T) {
	ts := newTestServer(t)
	do(t, http.MethodPost, ts.URL+"/projects", `{"name":"reef","path":"/tmp"}`)
	do(t, http.MethodPost, ts.URL+"/projects", `{"name":"other","path":"/"}`)

	status, body := do(t, http.MethodPost, ts.URL+"/work-items", `{"projectId":1,"title":"Auth"}`)
	if status != 201 || !strings.HasPrefix(body, `{"id":1,"projectId":1,"parentId":null,"type":"feature","title":"Auth"`) {
		t.Fatalf("create = %d %s", status, body)
	}
	do(t, http.MethodPost, ts.URL+"/work-items", `{"projectId":2,"title":"Auth"}`)

	status, body = do(t, http.MethodGet, ts.URL+"/work-items?project=1", "")
	if status != 200 || strings.Count(body, `"title":"Auth"`) != 1 || !strings.Contains(body, `"projectId":1`) {
		t.Fatalf("filtered list = %d %s", status, body)
	}

	status, body = do(t, http.MethodPut, ts.URL+"/work-items/1",
		`{"title":"Auth","description":"sign in","status":"queued","acceptanceLines":[{"text":"works","checked":true,"note":"ran it"}]}`)
	if status != 200 || !strings.Contains(body, `"description":"sign in","status":"backlog"`) ||
		!strings.Contains(body, `"acceptanceLines":[{"position":1,"text":"works","checked":true,"note":"ran it"}]`) {
		t.Fatalf("update = %d %s, want the status in the body ignored", status, body)
	}
	status, body = do(t, http.MethodPost, ts.URL+"/work-items/1/approve", `{"key":"k1"}`)
	if status != 400 || body != `{"error":"invalid: work item 1 needs an owner before it is approved"}` {
		t.Fatalf("patch to queued = %d %s, want 400 invalid", status, body)
	}
	status, body = do(t, http.MethodPatch, ts.URL+"/work-items/1", `{"status":"in_progress"}`)
	if status != 400 || body != `{"error":"invalid: work item 1: user cannot move an item from backlog to in_progress"}` {
		t.Fatalf("patch to in_progress = %d %s, want 400 invalid", status, body)
	}
	status, body = do(t, http.MethodPatch, ts.URL+"/work-items/1", `{"status":"open"}`)
	if status != 400 || body != `{"error":"invalid: work item 1: unknown status \"open\""}` {
		t.Fatalf("patch to open = %d %s, want 400 invalid", status, body)
	}
	status, body = do(t, http.MethodPut, ts.URL+"/work-items/1", `{"title":"Auth","type":"improvement"}`)
	if status != 200 || !strings.Contains(body, `"type":"improvement"`) || !strings.Contains(body, `"status":"backlog"`) ||
		!strings.Contains(body, `"checked":true`) {
		t.Fatalf("update type = %d %s, want type kept status and lines", status, body)
	}
	if status, body = do(t, http.MethodPut, ts.URL+"/work-items/1", `{"title":"Auth","type":"epic"}`); status != 400 {
		t.Fatalf("update bad type = %d %s, want 400", status, body)
	}

	status, body = do(t, http.MethodPut, ts.URL+"/work-items/1/brief", `{"body":"v1","author":"lead"}`)
	if status != 201 || !strings.HasPrefix(body, `{"id":1,"workItemId":1,"body":"v1","author":"lead","createdAt":"`) {
		t.Fatalf("brief v1 = %d %s", status, body)
	}
	do(t, http.MethodPut, ts.URL+"/work-items/1/brief", `{"body":"v2","author":"lead"}`)
	status, body = do(t, http.MethodGet, ts.URL+"/work-items/1/brief", "")
	if status != 200 || !strings.Contains(body, `"body":"v2"`) {
		t.Fatalf("latest brief = %d %s", status, body)
	}
	status, body = do(t, http.MethodGet, ts.URL+"/work-items/1/brief/versions", "")
	if status != 200 || !strings.Contains(body, `"body":"v1"`) || !strings.Contains(body, `"body":"v2"`) {
		t.Fatalf("versions = %d %s", status, body)
	}

	do(t, http.MethodPut, ts.URL+"/work-items/1/notes/architect", `{"body":"one"}`)
	status, body = do(t, http.MethodPut, ts.URL+"/work-items/1/notes/architect", `{"body":"two"}`)
	if status != 200 || !strings.HasPrefix(body, `{"workItemId":1,"role":"architect","body":"two","updatedAt":"`) {
		t.Fatalf("notes = %d %s", status, body)
	}
	if status, body = do(t, http.MethodDelete, ts.URL+"/work-items/1/notes/architect", ""); status != 204 || body != "" {
		t.Fatalf("delete notes = %d %s", status, body)
	}
	if status, body = do(t, http.MethodGet, ts.URL+"/work-items/1/notes/architect", ""); status != 404 {
		t.Fatalf("notes after delete = %d %s, want 404", status, body)
	}
	do(t, http.MethodPut, ts.URL+"/work-items/1/notes/architect", `{"body":"three"}`)

	if status, _ = do(t, http.MethodDelete, ts.URL+"/projects/1", ""); status != 204 {
		t.Fatalf("delete project = %d", status)
	}
	for _, path := range []string{"/work-items/1", "/work-items/1/brief", "/work-items/1/notes/architect"} {
		if status, body = do(t, http.MethodGet, ts.URL+path, ""); status != 200 {
			t.Errorf("GET %s after project delete = %d %s, want 200", path, status, body)
		}
	}
}
