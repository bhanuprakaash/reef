// Package web embeds the built React app and serves it in front of the API.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var _dist embed.FS

// Dist returns the built app: the contents of web/dist.
func Dist() (fs.FS, error) {
	return fs.Sub(_dist, "dist")
}

// Handler serves the app from fsys and sends everything else to api. A GET or
// HEAD for a file in fsys gets that file. Any other GET or HEAD whose Accept
// header includes text/html is a page load of a client route, such as /tasks,
// and gets index.html. The rest, including every API call, goes to api.
func Handler(fsys fs.FS, api http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			api.ServeHTTP(w, r)
			return
		}
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if info, err := fs.Stat(fsys, name); name != "" && err == nil && !info.IsDir() {
			http.ServeFileFS(w, r, fsys, name)
			return
		}
		if !strings.Contains(r.Header.Get("Accept"), "text/html") {
			api.ServeHTTP(w, r)
			return
		}
		if _, err := fs.Stat(fsys, "index.html"); err != nil {
			http.Error(w, "UI not built: run npm run build in web/", http.StatusServiceUnavailable)
			return
		}
		http.ServeFileFS(w, r, fsys, "index.html")
	})
}
