package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestUpdateSpacePermissionModeUsedWhenSessionOpens(t *testing.T) {
	env := newLiveEnv(t)
	var openedMode string
	env.open = func(ctx context.Context) (session, error) {
		ch, err := env.st.Space(ctx, 1)
		if err != nil {
			return nil, err
		}
		openedMode = ch.PermissionMode
		return env.fake, nil
	}

	if status, body := do(t, http.MethodPut, env.url+"/spaces/1",
		`{"name":"scratch","permissionMode":"acceptEdits"}`); status != 200 {
		t.Fatalf("update Space = %d %s, want 200", status, body)
	}
	if status, body := env.send(t, "hello"); status != 202 {
		t.Fatalf("send = %d %s, want 202", status, body)
	}
	if openedMode != "acceptEdits" {
		t.Errorf("permission mode at session open = %q, want acceptEdits", openedMode)
	}
}

func TestSpaceStatus(t *testing.T) {
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
			path:       "/spaces",
			give:       `{"projectId":9,"name":"c","backend":"claude"}`,
			wantStatus: 400,
			wantBody:   `{"error":"invalid: unknown project 9"}`,
		},
		{
			name:       "work item of another project",
			method:     http.MethodPost,
			path:       "/spaces",
			give:       `{"projectId":2,"workItemId":1,"name":"c","backend":"claude"}`,
			wantStatus: 400,
			wantBody:   `{"error":"invalid: work item 1 is not in project 2"}`,
		},
		{
			name:       "unknown work item",
			method:     http.MethodPost,
			path:       "/spaces",
			give:       `{"projectId":1,"workItemId":9,"name":"c","backend":"claude"}`,
			wantStatus: 400,
			wantBody:   `{"error":"invalid: unknown work item 9"}`,
		},
		{
			name:       "unknown field",
			method:     http.MethodPost,
			path:       "/spaces",
			give:       `{"projectId":1,"feature_id":1,"name":"c"}`,
			wantStatus: 400,
			wantBody:   `{"error":"invalid: bad JSON: json: unknown field \"feature_id\""}`,
		},
		{
			name:       "role other than lead",
			method:     http.MethodPost,
			path:       "/spaces",
			give:       `{"projectId":1,"name":"c","role":"architect","backend":"claude"}`,
			wantStatus: 400,
			wantBody:   `{"error":"invalid: role must be lead"}`,
		},
		{
			name:       "missing name",
			method:     http.MethodPost,
			path:       "/spaces",
			give:       `{"projectId":1,"backend":"claude"}`,
			wantStatus: 400,
			wantBody:   `{"error":"invalid: Space name is required"}`,
		},
		{
			name:       "messages need a space",
			method:     http.MethodGet,
			path:       "/messages",
			wantStatus: 400,
			wantBody:   `{"error":"invalid: Space is required"}`,
		},
		{
			name:       "message to unknown space",
			method:     http.MethodPost,
			path:       "/messages",
			give:       `{"spaceId":9,"author":"user","body":"hi"}`,
			wantStatus: 400,
			wantBody:   `{"error":"invalid: unknown space 9"}`,
		},
		{
			name:       "message update",
			method:     http.MethodPut,
			path:       "/messages/1",
			wantStatus: 405,
			wantBody:   `{"error":"method not allowed"}`,
		},
		{
			name:       "message delete",
			method:     http.MethodDelete,
			path:       "/messages/1",
			wantStatus: 405,
			wantBody:   `{"error":"method not allowed"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := newTestServer(t)
			do(t, http.MethodPost, ts.URL+"/projects", `{"name":"reef","path":"/tmp"}`)
			do(t, http.MethodPost, ts.URL+"/projects", `{"name":"other","path":"/"}`)
			do(t, http.MethodPost, ts.URL+"/work-items", `{"projectId":1,"title":"Auth"}`)
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

func TestListSpacesLastMessage(t *testing.T) {
	ts := newTestServer(t)
	do(t, http.MethodPost, ts.URL+"/projects", `{"name":"reef","path":"/tmp"}`)
	do(t, http.MethodPost, ts.URL+"/spaces", `{"projectId":1,"name":"talk","backend":"claude"}`)
	do(t, http.MethodPost, ts.URL+"/spaces", `{"projectId":1,"name":"empty","backend":"claude"}`)
	do(t, http.MethodPost, ts.URL+"/spaces", `{"projectId":1,"name":"long","backend":"claude"}`)
	for _, m := range []string{
		`{"spaceId":1,"author":"user","body":"hi"}`,
		`{"spaceId":1,"author":"lead","body":"hello there"}`,
		`{"spaceId":1,"parentId":1,"author":"user","body":"in a thread"}`,
		`{"spaceId":3,"author":"user","body":"` + strings.Repeat("a", 250) + `"}`,
	} {
		if status, body := do(t, http.MethodPost, ts.URL+"/messages", m); status != 201 {
			t.Fatalf("post message = %d %s", status, body)
		}
	}

	status, body := do(t, http.MethodGet, ts.URL+"/spaces", "")
	if status != 200 {
		t.Fatalf("list = %d %s", status, body)
	}
	var got []struct {
		Name        string `json:"name"`
		LastMessage *struct {
			Author    string `json:"author"`
			Body      string `json:"body"`
			CreatedAt string `json:"createdAt"`
		} `json:"lastMessage"`
	}
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Name != "talk" || got[1].Name != "empty" || got[2].Name != "long" {
		t.Fatalf("spaces = %s", body)
	}
	if m := got[0].LastMessage; m == nil || m.Author != "user" || m.Body != "in a thread" || m.CreatedAt == "" {
		t.Errorf("talk lastMessage = %+v, want user: in a thread", m)
	}
	if m := got[2].LastMessage; m == nil || m.Body != strings.Repeat("a", 200) {
		t.Errorf("long lastMessage = %+v, want the first 200 characters", m)
	}
	if got[1].LastMessage != nil || !strings.Contains(body, `"lastMessage":null`) {
		t.Errorf("empty lastMessage = %+v, want null in %s", got[1].LastMessage, body)
	}
}

func TestSpaceRoundTrip(t *testing.T) {
	ts := newTestServer(t)
	do(t, http.MethodPost, ts.URL+"/projects", `{"name":"reef","path":"/tmp"}`)
	do(t, http.MethodPost, ts.URL+"/work-items", `{"projectId":1,"title":"Auth"}`)

	status, body := do(t, http.MethodPost, ts.URL+"/spaces", `{"projectId":1,"name":"scratch","role":"lead","backend":"claude","permissionMode":"acceptEdits"}`)
	if status != 201 || !strings.HasPrefix(body,
		`{"id":1,"projectId":1,"workItemId":null,"name":"scratch","role":"lead","backend":"claude",`+
			`"permissionMode":"acceptEdits","paused":"","tokenBudget":1000000,"team":"solo","createdAt":"`) {
		t.Fatalf("quick session = %d %s", status, body)
	}
	status, body = do(t, http.MethodPost, ts.URL+"/spaces", `{"projectId":1,"workItemId":1,"name":"auth","backend":"claude"}`)
	if status != 201 || !strings.Contains(body, `"workItemId":1,"name":"auth"`) {
		t.Fatalf("work item Space = %d %s", status, body)
	}

	status, body = do(t, http.MethodGet, ts.URL+"/spaces?project=1", "")
	if status != 200 || strings.Count(body, `"projectId":1`) != 2 {
		t.Fatalf("by project = %d %s", status, body)
	}
	status, body = do(t, http.MethodGet, ts.URL+"/spaces?workItem=1", "")
	if status != 200 || strings.Count(body, `"projectId":1`) != 1 || !strings.Contains(body, `"name":"auth"`) {
		t.Fatalf("by work item = %d %s", status, body)
	}

	status, body = do(t, http.MethodPut, ts.URL+"/spaces/1",
		`{"name":"renamed","permissionMode":"default"}`)
	if status != 200 || !strings.Contains(body,
		`"projectId":1,"workItemId":null,"name":"renamed"`) ||
		!strings.Contains(body, `"permissionMode":"default"`) {
		t.Fatalf("rename quick session = %d %s", status, body)
	}
	if status, body = do(t, http.MethodPut, ts.URL+"/spaces/1",
		`{"name":"renamed","permissionMode":"workspace-write"}`); status != 400 {
		t.Fatalf("bad permission mode = %d %s, want 400", status, body)
	}
	status, body = do(t, http.MethodPut, ts.URL+"/spaces/2", `{"name":"renamed"}`)
	if status != 200 || !strings.Contains(body, `"projectId":1,"workItemId":1,"name":"renamed"`) {
		t.Fatalf("rename work item Space = %d %s", status, body)
	}
	if status, body = do(t, http.MethodPut, ts.URL+"/spaces/2", `{"projectId":9,"name":"x"}`); status != 400 {
		t.Fatalf("ignored field update = %d %s, want 400", status, body)
	}
	if status, body = do(t, http.MethodPut, ts.URL+"/spaces/9", `{"name":"x"}`); status != 404 {
		t.Fatalf("rename unknown = %d %s, want 404", status, body)
	}

	for _, b := range []string{"one", "two", "three"} {
		status, body = do(t, http.MethodPost, ts.URL+"/messages", `{"spaceId":1,"author":"user","body":"`+b+`"}`)
		if status != 201 {
			t.Fatalf("post %s = %d %s", b, status, body)
		}
	}
	status, body = do(t, http.MethodGet, ts.URL+"/messages?space=1&after=1", "")
	two, three := strings.Index(body, `"body":"two"`), strings.Index(body, `"body":"three"`)
	if status != 200 || strings.Contains(body, `"body":"one"`) || two < 0 || three < two {
		t.Fatalf("after = %d %s", status, body)
	}

	if status, _ = do(t, http.MethodDelete, ts.URL+"/spaces/1", ""); status != 204 {
		t.Fatalf("delete Space = %d", status)
	}
	if status, body = do(t, http.MethodGet, ts.URL+"/messages?space=1", ""); status != 404 {
		t.Errorf("messages after Space delete = %d %s, want 404", status, body)
	}
}
