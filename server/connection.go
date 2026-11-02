package server

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/bhanuprakaash/reef/store"
)

func (s *server) connectionRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /connections", s.listConnections)
	mux.HandleFunc("POST /connections", s.createConnection)
	mux.HandleFunc("GET /connections/available", s.availableConnections)
	mux.HandleFunc("POST /connections/import", s.importConnection)
	mux.HandleFunc("/connections", methodNotAllowed)
	mux.HandleFunc("PUT /connections/{id}", s.putConnection)
	mux.HandleFunc("DELETE /connections/{id}", s.deleteConnection)
	mux.HandleFunc("/connections/{id}", methodNotAllowed)
}

func (s *server) listConnections(w http.ResponseWriter, r *http.Request) {
	cs, err := s.st.Connections(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	for i, c := range cs {
		cs[i] = redactConnection(c)
	}
	writeJSON(w, http.StatusOK, nonNil(cs))
}

func (s *server) createConnection(w http.ResponseWriter, r *http.Request) {
	var in store.Connection
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	in.ID = 0
	c, err := s.st.CreateConnection(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, redactConnection(c))
}

// availableConnection is an MCP server the user already has, with secrets masked like a
// connection's. Added says a connection with the same command and arguments, or the same url,
// exists.
type availableConnection struct {
	Source       string            `json:"source"`
	Name         string            `json:"name"`
	Kind         string            `json:"kind"`
	Command      string            `json:"command,omitempty"`
	Args         []string          `json:"args,omitempty"`
	Env          map[string]string `json:"env,omitempty"`
	URL          string            `json:"url,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	Added        bool              `json:"added"`
	ConnectionID int64             `json:"connectionId,omitempty"`
}

func (s *server) availableConnections(w http.ResponseWriter, r *http.Request) {
	projectID, err := queryID(r, "projectId")
	if err != nil {
		writeError(w, err)
		return
	}
	found, err := s.findMCP(r.Context(), projectID)
	if err != nil {
		writeError(w, err)
		return
	}
	have, err := s.st.Connections(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]availableConnection, 0, len(found))
	for _, f := range found {
		v := redactConnection(f.Conn)
		a := availableConnection{
			Source: f.Source, Name: f.Name, Kind: "command", Command: v.Command, Args: v.Args,
			Env: v.Env, URL: v.URL, Headers: v.Headers,
		}
		if f.Conn.URL != "" {
			a.Kind = "url"
		}
		if i := slices.IndexFunc(have, func(c store.Connection) bool { return sameServer(c, f.Conn) }); i >= 0 {
			a.Added, a.ConnectionID = true, have[i].ID
		}
		out = append(out, a)
	}
	writeJSON(w, http.StatusOK, out)
}

// importConnection reads the server again from its source and copies it whole, so the secrets
// never pass through the client.
func (s *server) importConnection(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Source    string `json:"source"`
		Name      string `json:"name"`
		ProjectID *int64 `json:"projectId"`
	}
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	found, err := s.findMCP(r.Context(), in.ProjectID)
	if err != nil {
		writeError(w, err)
		return
	}
	i := slices.IndexFunc(found, func(f foundMCP) bool { return f.Source == in.Source && f.Name == in.Name })
	if i < 0 {
		writeError(w, fmt.Errorf("%w: no %s server named %q", store.ErrNotFound, in.Source, in.Name))
		return
	}
	conn := found[i].Conn
	conn.Name = _badNameChars.ReplaceAllString(in.Name, "-")
	c, err := s.st.CreateConnection(r.Context(), conn)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, redactConnection(c))
}

// findMCP lists the user's MCP servers; the project's own are included when projectID is set.
func (s *server) findMCP(ctx context.Context, projectID *int64) ([]foundMCP, error) {
	var projectPath string
	if projectID != nil {
		p, err := s.st.Project(ctx, *projectID)
		if err != nil {
			return nil, err
		}
		projectPath = p.Path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("find home folder: %w", err)
	}
	return findMCPServers(ctx, home, projectPath), nil
}

func sameServer(a, b store.Connection) bool {
	if a.URL != "" || b.URL != "" {
		return a.URL == b.URL
	}
	return a.Command == b.Command && slices.Equal(a.Args, b.Args)
}

func (s *server) putConnection(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var in store.Connection
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	if in.ID != 0 && in.ID != id {
		writeError(w, fmt.Errorf("%w: body id %d does not match %d", store.ErrInvalid, in.ID, id))
		return
	}
	in.ID = id
	stored, err := s.st.Connection(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := keepSecrets(&in, stored); err != nil {
		writeError(w, err)
		return
	}
	c, err := s.st.UpdateConnection(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, redactConnection(c))
}

func (s *server) deleteConnection(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.st.DeleteConnection(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// _mask stands in for a secret in REST responses: env and header values, every argument, and the
// userinfo and query of a url.
const _mask = "****"

// _badNameChars are what an imported server's name loses to become a connection name.
var _badNameChars = regexp.MustCompile(`[^A-Za-z0-9_-]`)

func redactConnection(c store.Connection) store.Connection {
	c.Env = maskValues(c.Env)
	c.Headers = maskValues(c.Headers)
	if c.Args != nil {
		c.Args = slices.Repeat([]string{_mask}, len(c.Args))
	}
	c.URL = redactURL(c.URL)
	return c
}

func maskValues(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k := range m {
		out[k] = _mask
	}
	return out
}

func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || (u.User == nil && u.RawQuery == "") {
		return raw
	}
	hadUser := u.User != nil
	u.User = nil
	pairs := strings.Split(u.RawQuery, "&")
	for i, p := range pairs {
		if k, _, ok := strings.Cut(p, "="); ok {
			pairs[i] = k + "=" + _mask
		} else if p != "" {
			pairs[i] = _mask
		}
	}
	u.RawQuery = strings.Join(pairs, "&")
	out := u.String()
	if hadUser {
		out = strings.Replace(out, "://", "://"+_mask+"@", 1)
	}
	return out
}

// keepSecrets fills in from stored where the editor sent back a mask, or sent no env for a local
// process or no headers for a url, so saving needs no sight of the secrets. A mask with nothing
// stored behind it fails.
func keepSecrets(in *store.Connection, stored store.Connection) error {
	if in.Env == nil && in.Command != "" {
		in.Env = stored.Env
	}
	if in.Headers == nil && in.URL != "" {
		in.Headers = stored.Headers
	}
	for _, f := range []struct {
		name     string
		in, have map[string]string
	}{{"env", in.Env, stored.Env}, {"header", in.Headers, stored.Headers}} {
		for k, v := range f.in {
			if v != _mask {
				continue
			}
			sv, ok := f.have[k]
			if !ok {
				return fmt.Errorf("%w: %s %s is masked and has no stored value; enter it again",
					store.ErrInvalid, f.name, k)
			}
			f.in[k] = sv
		}
	}
	for i, v := range in.Args {
		if v != _mask {
			continue
		}
		if i >= len(stored.Args) {
			return fmt.Errorf("%w: argument %d is masked and has no stored value; enter it again", store.ErrInvalid, i+1)
		}
		in.Args[i] = stored.Args[i]
	}
	if in.URL == redactURL(stored.URL) {
		in.URL = stored.URL
	} else if strings.Contains(in.URL, _mask) {
		return fmt.Errorf("%w: url holds a masked secret; enter the whole url again", store.ErrInvalid)
	}
	return nil
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
