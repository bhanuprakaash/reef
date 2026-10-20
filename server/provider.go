package server

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/bhanuprakaash/reef/store"
)

const _versionTimeout = 3 * time.Second

// providerView is one claude or codex install. Detected ones are found on PATH; the others are
// added by the user and have a folder. Version is left out when the binary does not answer
// --version in time, Path when an added install's binary is gone.
type providerView struct {
	Name      string     `json:"name"`
	Kind      string     `json:"kind"`
	Detected  bool       `json:"detected"`
	Folder    string     `json:"folder,omitempty"`
	Path      string     `json:"path,omitempty"`
	Version   string     `json:"version,omitempty"`
	CreatedAt *time.Time `json:"createdAt,omitempty"`
}

func (s *server) providerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /providers", s.listProviders)
	mux.HandleFunc("POST /providers", s.createProvider)
	mux.HandleFunc("/providers", methodNotAllowed)
	mux.HandleFunc("DELETE /providers/{name}", s.deleteProvider)
	mux.HandleFunc("/providers/{name}", methodNotAllowed)
}

func (s *server) listProviders(w http.ResponseWriter, r *http.Request) {
	added, err := s.st.Providers(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	views := make([]providerView, 0, 2+len(added))
	for _, kind := range []string{"claude", "codex"} {
		if path, err := exec.LookPath(kind); err == nil {
			views = append(views, providerView{Name: kind, Kind: kind, Detected: true, Path: path})
		}
	}
	for _, p := range added {
		views = append(views, viewProvider(p))
	}
	fillVersions(r.Context(), views)
	writeJSON(w, http.StatusOK, views)
}

func (s *server) createProvider(w http.ResponseWriter, r *http.Request) {
	var in store.Provider
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	p, err := s.st.CreateProvider(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	views := []providerView{viewProvider(p)}
	fillVersions(r.Context(), views)
	writeJSON(w, http.StatusCreated, views[0])
}

func (s *server) deleteProvider(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "claude" || name == "codex" {
		writeError(w, fmt.Errorf("%w: %s is the one found on PATH and cannot be removed",
			store.ErrInvalid, name))
		return
	}
	if err := s.st.DeleteProvider(r.Context(), name); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func viewProvider(p store.Provider) providerView {
	v := providerView{Name: p.Name, Kind: p.Kind, Folder: p.Folder, CreatedAt: &p.CreatedAt}
	v.Path, _ = p.Bin()
	return v
}

// fillVersions sets each view's Version from `PATH --version`, all at once, so one slow
// binary costs the time limit once.
func fillVersions(ctx context.Context, views []providerView) {
	var wg sync.WaitGroup
	for i := range views {
		if views[i].Path == "" {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			views[i].Version = binVersion(ctx, views[i].Path)
		}()
	}
	wg.Wait()
}

func binVersion(ctx context.Context, path string) string {
	ctx, cancel := context.WithTimeout(ctx, _versionTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--version")
	cmd.WaitDelay = time.Second
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
