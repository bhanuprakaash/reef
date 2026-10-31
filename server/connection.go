package server

import (
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/bhanuprakaash/reef/store"
)

func (s *server) connectionRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /connections", s.listConnections)
	mux.HandleFunc("POST /connections", s.createConnection)
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

// _mask stands in for a secret in REST responses: env values, every argument, and the userinfo and
// query of a url.
const _mask = "****"

func redactConnection(c store.Connection) store.Connection {
	if c.Env != nil {
		env := make(map[string]string, len(c.Env))
		for k := range c.Env {
			env[k] = _mask
		}
		c.Env = env
	}
	if c.Args != nil {
		c.Args = slices.Repeat([]string{_mask}, len(c.Args))
	}
	c.URL = redactURL(c.URL)
	return c
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
// process, so saving needs no sight of the secrets. A mask with nothing stored behind it fails.
func keepSecrets(in *store.Connection, stored store.Connection) error {
	if in.Env == nil && in.Command != "" {
		in.Env = stored.Env
	}
	for k, v := range in.Env {
		if v != _mask {
			continue
		}
		sv, ok := stored.Env[k]
		if !ok {
			return fmt.Errorf("%w: env %s is masked and has no stored value; enter it again", store.ErrInvalid, k)
		}
		in.Env[k] = sv
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
