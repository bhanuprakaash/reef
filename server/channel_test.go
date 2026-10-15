package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestUpdateChannelPermissionModeUsedWhenSessionOpens(t *testing.T) {
	env := newLiveEnv(t)
	var openedMode string
	env.open = func(ctx context.Context) (session, error) {
		ch, err := env.st.Channel(ctx, 1)
		if err != nil {
			return nil, err
		}
		openedMode = ch.PermissionMode
		return env.fake, nil
	}

	if status, body := do(t, http.MethodPut, env.url+"/channels/1",
		`{"name":"scratch","permissionMode":"acceptEdits"}`); status != 200 {
		t.Fatalf("update channel = %d %s, want 200", status, body)
	}
	if status, body := env.send(t, "hello"); status != 202 {
		t.Fatalf("send = %d %s, want 202", status, body)
	}
	if openedMode != "acceptEdits" {
		t.Errorf("permission mode at session open = %q, want acceptEdits", openedMode)
	}
}

func TestChannelStatus(t *testing.T) {
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
			path:       "/channels",
			give:       `{"projectId":9,"name":"c","backend":"claude"}`,
			wantStatus: 400,
			wantBody:   `{"error":"invalid: unknown project 9"}`,
		},
		{
			name:       "feature of another project",
			method:     http.MethodPost,
			path:       "/channels",
			give:       `{"projectId":2,"featureId":1,"name":"c","backend":"claude"}`,
			wantStatus: 400,
			wantBody:   `{"error":"invalid: feature 1 is not in project 2"}`,
		},
		{
			name:       "unknown feature",
			method:     http.MethodPost,
			path:       "/channels",
			give:       `{"projectId":1,"featureId":9,"name":"c","backend":"claude"}`,
			wantStatus: 400,
			wantBody:   `{"error":"invalid: unknown feature 9"}`,
		},
		{
			name:       "unknown field",
			method:     http.MethodPost,
			path:       "/channels",
			give:       `{"projectId":1,"feature_id":1,"name":"c"}`,
			wantStatus: 400,
			wantBody:   `{"error":"invalid: bad JSON: json: unknown field \"feature_id\""}`,
		},
		{
			name:       "role other than lead",
			method:     http.MethodPost,
			path:       "/channels",
			give:       `{"projectId":1,"name":"c","role":"architect","backend":"claude"}`,
			wantStatus: 400,
			wantBody:   `{"error":"invalid: role must be lead"}`,
		},
		{
			name:       "missing name",
			method:     http.MethodPost,
			path:       "/channels",
			give:       `{"projectId":1,"backend":"claude"}`,
			wantStatus: 400,
			wantBody:   `{"error":"invalid: channel name is required"}`,
		},
		{
			name:       "messages need a channel",
			method:     http.MethodGet,
			path:       "/messages",
			wantStatus: 400,
			wantBody:   `{"error":"invalid: channel is required"}`,
		},
		{
			name:       "message to unknown channel",
			method:     http.MethodPost,
			path:       "/messages",
			give:       `{"channelId":9,"author":"user","body":"hi"}`,
			wantStatus: 400,
			wantBody:   `{"error":"invalid: unknown channel 9"}`,
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
			do(t, http.MethodPost, ts.URL+"/features", `{"projectId":1,"slug":"auth","name":"Auth"}`)
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

func TestListChannelsLastMessage(t *testing.T) {
	ts := newTestServer(t)
	do(t, http.MethodPost, ts.URL+"/projects", `{"name":"reef","path":"/tmp"}`)
	do(t, http.MethodPost, ts.URL+"/channels", `{"projectId":1,"name":"talk","backend":"claude"}`)
	do(t, http.MethodPost, ts.URL+"/channels", `{"projectId":1,"name":"empty","backend":"claude"}`)
	do(t, http.MethodPost, ts.URL+"/channels", `{"projectId":1,"name":"long","backend":"claude"}`)
	for _, m := range []string{
		`{"channelId":1,"author":"user","body":"hi"}`,
		`{"channelId":1,"author":"lead","body":"hello there"}`,
		`{"channelId":1,"parentId":1,"author":"user","body":"in a thread"}`,
		`{"channelId":3,"author":"user","body":"` + strings.Repeat("a", 250) + `"}`,
	} {
		if status, body := do(t, http.MethodPost, ts.URL+"/messages", m); status != 201 {
			t.Fatalf("post message = %d %s", status, body)
		}
	}

	status, body := do(t, http.MethodGet, ts.URL+"/channels", "")
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
		t.Fatalf("channels = %s", body)
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

func TestChannelRoundTrip(t *testing.T) {
	ts := newTestServer(t)
	do(t, http.MethodPost, ts.URL+"/projects", `{"name":"reef","path":"/tmp"}`)
	do(t, http.MethodPost, ts.URL+"/features", `{"projectId":1,"slug":"auth","name":"Auth"}`)

	status, body := do(t, http.MethodPost, ts.URL+"/channels", `{"projectId":1,"name":"scratch","role":"lead","backend":"claude","permissionMode":"acceptEdits"}`)
	if status != 201 || !strings.HasPrefix(body,
		`{"id":1,"projectId":1,"featureId":null,"name":"scratch","role":"lead","backend":"claude",`+
			`"permissionMode":"acceptEdits","paused":"","tokenBudget":1000000,"team":"solo","createdAt":"`) {
		t.Fatalf("quick session = %d %s", status, body)
	}
	status, body = do(t, http.MethodPost, ts.URL+"/channels", `{"projectId":1,"featureId":1,"name":"auth","backend":"claude"}`)
	if status != 201 || !strings.Contains(body, `"featureId":1,"name":"auth"`) {
		t.Fatalf("feature channel = %d %s", status, body)
	}

	status, body = do(t, http.MethodGet, ts.URL+"/channels?project=1", "")
	if status != 200 || strings.Count(body, `"projectId":1`) != 2 {
		t.Fatalf("by project = %d %s", status, body)
	}
	status, body = do(t, http.MethodGet, ts.URL+"/channels?feature=1", "")
	if status != 200 || strings.Count(body, `"projectId":1`) != 1 || !strings.Contains(body, `"name":"auth"`) {
		t.Fatalf("by feature = %d %s", status, body)
	}

	status, body = do(t, http.MethodPut, ts.URL+"/channels/1",
		`{"name":"renamed","permissionMode":"default"}`)
	if status != 200 || !strings.Contains(body,
		`"projectId":1,"featureId":null,"name":"renamed"`) ||
		!strings.Contains(body, `"permissionMode":"default"`) {
		t.Fatalf("rename quick session = %d %s", status, body)
	}
	if status, body = do(t, http.MethodPut, ts.URL+"/channels/1",
		`{"name":"renamed","permissionMode":"workspace-write"}`); status != 400 {
		t.Fatalf("bad permission mode = %d %s, want 400", status, body)
	}
	status, body = do(t, http.MethodPut, ts.URL+"/channels/2", `{"name":"renamed"}`)
	if status != 200 || !strings.Contains(body, `"projectId":1,"featureId":1,"name":"renamed"`) {
		t.Fatalf("rename feature channel = %d %s", status, body)
	}
	if status, body = do(t, http.MethodPut, ts.URL+"/channels/2", `{"projectId":9,"name":"x"}`); status != 400 {
		t.Fatalf("ignored field update = %d %s, want 400", status, body)
	}
	if status, body = do(t, http.MethodPut, ts.URL+"/channels/9", `{"name":"x"}`); status != 404 {
		t.Fatalf("rename unknown = %d %s, want 404", status, body)
	}

	for _, b := range []string{"one", "two", "three"} {
		status, body = do(t, http.MethodPost, ts.URL+"/messages", `{"channelId":1,"author":"user","body":"`+b+`"}`)
		if status != 201 {
			t.Fatalf("post %s = %d %s", b, status, body)
		}
	}
	status, body = do(t, http.MethodGet, ts.URL+"/messages?channel=1&after=1", "")
	two, three := strings.Index(body, `"body":"two"`), strings.Index(body, `"body":"three"`)
	if status != 200 || strings.Contains(body, `"body":"one"`) || two < 0 || three < two {
		t.Fatalf("after = %d %s", status, body)
	}

	if status, _ = do(t, http.MethodDelete, ts.URL+"/channels/1", ""); status != 204 {
		t.Fatalf("delete channel = %d", status)
	}
	if status, body = do(t, http.MethodGet, ts.URL+"/messages?channel=1", ""); status != 404 {
		t.Errorf("messages after channel delete = %d %s, want 404", status, body)
	}
}
