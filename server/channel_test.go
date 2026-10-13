package server

import (
	"net/http"
	"strings"
	"testing"
)

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
			give:       `{"projectId":9,"name":"c"}`,
			wantStatus: 400,
			wantBody:   `{"error":"invalid: unknown project 9"}`,
		},
		{
			name:       "feature of another project",
			method:     http.MethodPost,
			path:       "/channels",
			give:       `{"projectId":2,"featureId":1,"name":"c"}`,
			wantStatus: 400,
			wantBody:   `{"error":"invalid: feature 1 is not in project 2"}`,
		},
		{
			name:       "unknown feature",
			method:     http.MethodPost,
			path:       "/channels",
			give:       `{"projectId":1,"featureId":9,"name":"c"}`,
			wantStatus: 400,
			wantBody:   `{"error":"invalid: unknown feature 9"}`,
		},
		{
			name:       "missing name",
			method:     http.MethodPost,
			path:       "/channels",
			give:       `{"projectId":1}`,
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
			wantStatus: 404,
			wantBody:   `{"error":"channel 9: not found"}`,
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
			do(t, http.MethodPost, ts.URL+"/projects", `{"name":"reef","path":"/tmp/reef"}`)
			do(t, http.MethodPost, ts.URL+"/projects", `{"name":"other","path":"/tmp/other"}`)
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

func TestChannelRoundTrip(t *testing.T) {
	ts := newTestServer(t)
	do(t, http.MethodPost, ts.URL+"/projects", `{"name":"reef","path":"/tmp/reef"}`)
	do(t, http.MethodPost, ts.URL+"/features", `{"projectId":1,"slug":"auth","name":"Auth"}`)

	status, body := do(t, http.MethodPost, ts.URL+"/channels", `{"projectId":1,"name":"scratch"}`)
	if status != 201 || !strings.HasPrefix(body,
		`{"id":1,"projectId":1,"featureId":null,"name":"scratch","createdAt":"`) {
		t.Fatalf("quick session = %d %s", status, body)
	}
	status, body = do(t, http.MethodPost, ts.URL+"/channels", `{"projectId":1,"featureId":1,"name":"auth"}`)
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

	status, body = do(t, http.MethodPut, ts.URL+"/channels/1", `{"projectId":9,"featureId":1,"name":"promoted"}`)
	if status != 200 || !strings.Contains(body, `"projectId":1,"featureId":1,"name":"promoted"`) {
		t.Fatalf("promote = %d %s", status, body)
	}
	status, body = do(t, http.MethodPut, ts.URL+"/channels/1", `{"name":"promoted"}`)
	if status != 200 || !strings.Contains(body, `"featureId":null`) {
		t.Fatalf("clear feature = %d %s", status, body)
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
