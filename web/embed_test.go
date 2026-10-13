package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestHandler(t *testing.T) {
	api := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[]`)
	})
	built := fstest.MapFS{
		"index.html":    {Data: []byte("<html>app</html>")},
		"assets/app.js": {Data: []byte("console.log(1)")},
	}
	tests := []struct {
		name        string
		fsys        fstest.MapFS
		method      string
		path        string
		accept      string
		wantStatus  int
		wantType    string
		wantContent string
	}{
		{
			name:   "client route with html accept gets index",
			fsys:   built,
			method: http.MethodGet, path: "/tasks", accept: "text/html,application/xhtml+xml",
			wantStatus: 200, wantType: "text/html", wantContent: "<html>app</html>",
		},
		{
			name:   "root with html accept gets index",
			fsys:   built,
			method: http.MethodGet, path: "/", accept: "text/html",
			wantStatus: 200, wantType: "text/html", wantContent: "<html>app</html>",
		},
		{
			name:   "json get goes to the api",
			fsys:   built,
			method: http.MethodGet, path: "/projects", accept: "application/json",
			wantStatus: 200, wantType: "application/json", wantContent: "[]",
		},
		{
			name:   "no accept goes to the api",
			fsys:   built,
			method: http.MethodGet, path: "/tasks",
			wantStatus: 200, wantType: "application/json", wantContent: "[]",
		},
		{
			name:   "post with html accept goes to the api",
			fsys:   built,
			method: http.MethodPost, path: "/projects", accept: "text/html",
			wantStatus: 200, wantType: "application/json", wantContent: "[]",
		},
		{
			name:   "asset",
			fsys:   built,
			method: http.MethodGet, path: "/assets/app.js", accept: "*/*",
			wantStatus: 200, wantType: "text/javascript", wantContent: "console.log(1)",
		},
		{
			name:   "ui not built",
			fsys:   fstest.MapFS{".keep": {}},
			method: http.MethodGet, path: "/tasks", accept: "text/html",
			wantStatus: 503, wantType: "text/plain", wantContent: "UI not built: run npm run build in web/",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			if tt.accept != "" {
				req.Header.Set("Accept", tt.accept)
			}
			rec := httptest.NewRecorder()
			Handler(tt.fsys, api).ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, tt.wantType) {
				t.Errorf("content type = %q, want prefix %q", got, tt.wantType)
			}
			if got := strings.TrimSpace(rec.Body.String()); got != tt.wantContent {
				t.Errorf("body = %q, want %q", got, tt.wantContent)
			}
		})
	}
}
