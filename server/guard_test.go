package server

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestGuard(t *testing.T) {
	tests := []struct {
		name        string
		method      string
		host        string
		contentType string
		wantStatus  int
		wantBody    string
	}{
		{
			name:        "other host",
			method:      http.MethodGet,
			host:        "evil.example",
			contentType: "application/json",
			wantStatus:  403,
			wantBody:    `{"error":"forbidden host"}`,
		},
		{
			name:        "other host with port",
			method:      http.MethodGet,
			host:        "evil.example:7700",
			contentType: "application/json",
			wantStatus:  403,
			wantBody:    `{"error":"forbidden host"}`,
		},
		{
			name:        "localhost with port",
			method:      http.MethodGet,
			host:        "localhost:7700",
			contentType: "application/json",
			wantStatus:  200,
			wantBody:    `{"status":"ok"}`,
		},
		{
			name:        "ipv6 loopback",
			method:      http.MethodGet,
			host:        "[::1]:7700",
			contentType: "application/json",
			wantStatus:  200,
			wantBody:    `{"status":"ok"}`,
		},
		{
			name:        "post as text",
			method:      http.MethodPost,
			host:        "127.0.0.1:7700",
			contentType: "text/plain",
			wantStatus:  415,
			wantBody:    `{"error":"content type must be application/json"}`,
		},
		{
			name:       "patch without a type",
			method:     http.MethodPatch,
			host:       "127.0.0.1:7700",
			wantStatus: 415,
			wantBody:   `{"error":"content type must be application/json"}`,
		},
		{
			name:        "post as json with charset",
			method:      http.MethodPost,
			host:        "127.0.0.1:7700",
			contentType: "application/json; charset=utf-8",
			wantStatus:  400,
			wantBody:    `{"error":"invalid: bad JSON: EOF"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := newTestServer(t)
			path := "/health"
			if tt.method != http.MethodGet {
				path = "/projects"
			}
			req, err := http.NewRequest(tt.method, ts.URL+path, nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Host = tt.host
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			b, err := io.ReadAll(res.Body)
			if err != nil {
				t.Fatal(err)
			}
			if res.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d", res.StatusCode, tt.wantStatus)
			}
			if got := strings.TrimSpace(string(b)); got != tt.wantBody {
				t.Errorf("body = %s, want %s", got, tt.wantBody)
			}
		})
	}
}
