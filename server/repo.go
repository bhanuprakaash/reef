package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/bhanuprakaash/reef/store"
)

var (
	// _repoHosts are the hosts a repository connection can be on, and the provider of each.
	_repoHosts = map[string]string{"github.com": store.RepoGitHub, "gitlab.com": store.RepoGitLab}

	_repoSegment = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
)

// repoProvider is what Reef needs from a repository provider's CLI, gh or glab. Later
// methods, such as opening a pull request or merge request, join it here.
type repoProvider interface {
	// Account makes an authenticated API call on host as the CLI's login and returns the
	// account it is. The text of a failure is a fixed phrase, safe to store and show; it never
	// holds the CLI's own output, which can carry a token.
	Account(ctx context.Context, host string) (string, error)
	OpenRequest(ctx context.Context, remote repoRemote, dir, head, base, title, description string) (store.RepoRequest, error)
}

// repoCLI runs a provider's CLI. gh and glab both answer `api --hostname HOST user`.
type repoCLI struct{ bin string }

var _ repoProvider = repoCLI{}

func repoProviderFor(provider string) repoProvider {
	if provider == store.RepoGitLab {
		return repoCLI{bin: "glab"}
	}
	return repoCLI{bin: "gh"}
}

func (c repoCLI) Account(ctx context.Context, host string) (string, error) {
	path, err := exec.LookPath(c.bin)
	if err != nil {
		return "", fmt.Errorf("%s is not on PATH", c.bin)
	}
	cctx, cancel := context.WithTimeout(ctx, _ghTimeout)
	defer cancel()
	out, err := exec.CommandContext(cctx, path, "api", "--hostname", host, "user").Output()
	if err != nil {
		if cctx.Err() != nil {
			return "", fmt.Errorf("%s did not answer in %s", c.bin, _ghTimeout)
		}
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return "", c.loginFailure(host, string(ee.Stderr))
		}
		return "", fmt.Errorf("run %s: %w", c.bin, err)
	}
	var user struct {
		Login    string `json:"login"`
		Username string `json:"username"`
	}
	if err := json.Unmarshal(out, &user); err != nil || user.Login+user.Username == "" {
		return "", fmt.Errorf("%s did not name the account", c.bin)
	}
	return user.Login + user.Username, nil
}

// loginFailure maps what the CLI said on stderr to a fixed phrase.
func (c repoCLI) loginFailure(host, stderr string) error {
	s := strings.ToLower(stderr)
	switch {
	case strings.Contains(s, "auth login") || strings.Contains(s, "no token"):
		return fmt.Errorf("not logged in: run `%s auth login --hostname %s`", c.bin, host)
	case strings.Contains(s, "401") || strings.Contains(s, "bad credentials"):
		return errors.New("login rejected: the token is expired or revoked")
	case strings.Contains(s, "403") || strings.Contains(s, "suspended"):
		return errors.New("access denied: the account is suspended or blocked")
	}
	return errors.New("login check failed")
}

// repoRemote is a repository on a provider host, such as gitlab.com/group/sub/repo.
type repoRemote struct {
	Provider string `json:"provider"`
	Host     string `json:"host"`
	Path     string `json:"path"`
}

// parseRepoRemote reads a git remote URL: https://host/path.git, ssh://git@host/path.git or
// git@host:path.git. A credential in the URL is dropped, and no error repeats the URL.
func parseRepoRemote(raw string) (repoRemote, error) {
	var host, path string
	if u, err := url.Parse(strings.TrimSpace(raw)); err == nil && u.Host != "" {
		host, path = u.Hostname(), u.Path
	} else if left, right, ok := strings.Cut(strings.TrimSpace(raw), ":"); ok {
		host = left[strings.LastIndex(left, "@")+1:]
		path = right
	}
	host = strings.ToLower(host)
	provider, ok := _repoHosts[host]
	if !ok {
		return repoRemote{}, fmt.Errorf("%w: remote host %q is not github.com or gitlab.com", store.ErrInvalid, host)
	}
	path = cleanRepoPath(path)
	if !validRepoPath(provider, path) {
		return repoRemote{}, fmt.Errorf("%w: remote path %q is not owner/repo on %s", store.ErrInvalid, path, host)
	}
	return repoRemote{Provider: provider, Host: host, Path: path}, nil
}

func cleanRepoPath(path string) string {
	return strings.TrimSuffix(strings.Trim(path, "/"), ".git")
}

// validRepoPath reports whether path is owner/repo on GitHub, or group[/subgroup...]/repo on GitLab.
func validRepoPath(provider, path string) bool {
	parts := strings.Split(path, "/")
	if len(parts) < 2 || provider == store.RepoGitHub && len(parts) != 2 {
		return false
	}
	return !slices.ContainsFunc(parts, func(p string) bool {
		return !_repoSegment.MatchString(p) || p == "." || p == ".."
	})
}

func (s *server) repoRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /workspaces/{id}/request", s.openRepoRequest)
	mux.HandleFunc("GET /workspaces/{id}/requests", s.workspaceRequests)
	mux.HandleFunc("GET /work-items/{id}/requests", s.workItemRequests)
	mux.HandleFunc("GET /repo-connections", s.listRepoConnections)
	mux.HandleFunc("POST /repo-connections", s.connectRepo)
	mux.HandleFunc("/repo-connections", methodNotAllowed)
	mux.HandleFunc("POST /repo-connections/{id}/check", s.checkRepoConnection)
	mux.HandleFunc("/repo-connections/{id}/check", methodNotAllowed)
	mux.HandleFunc("GET /projects/{id}/remote", s.detectProjectRemote)
	mux.HandleFunc("PUT /projects/{id}/remote", s.putProjectRemote)
	mux.HandleFunc("/projects/{id}/remote", methodNotAllowed)
}

func (s *server) listRepoConnections(w http.ResponseWriter, r *http.Request) {
	cs, err := s.st.RepoConnections(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	if cs == nil {
		cs = []store.RepoConnection{}
	}
	writeJSON(w, http.StatusOK, cs)
}

// connectRepo checks the CLI's login on the host and records the result, connected or not.
func (s *server) connectRepo(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Provider string `json:"provider"`
		Host     string `json:"host"`
	}
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	if in.Host == "" {
		in.Host = map[string]string{store.RepoGitHub: "github.com", store.RepoGitLab: "gitlab.com"}[in.Provider]
	}
	if in.Provider == "" || _repoHosts[in.Host] != in.Provider {
		writeError(w, fmt.Errorf("%w: %q on %q is not github on github.com or gitlab on gitlab.com",
			store.ErrInvalid, in.Provider, in.Host))
		return
	}
	c, err := s.recordRepoCheck(r.Context(), in.Provider, in.Host)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *server) checkRepoConnection(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	old, err := s.st.RepoConnection(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	c, err := s.recordRepoCheck(r.Context(), old.Provider, old.Host)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *server) recordRepoCheck(ctx context.Context, provider, host string) (store.RepoConnection, error) {
	c := store.RepoConnection{Provider: provider, Host: host, State: store.RepoConnected, CheckedAt: time.Now()}
	account, err := repoProviderFor(provider).Account(ctx, host)
	if err != nil {
		c.State, c.Reason = store.RepoDisconnected, err.Error()
	}
	c.Account = account
	return s.st.PutRepoConnection(ctx, c)
}

// detectedRemote is the repository a project's origin points at, with the connection on its
// host, when there is one.
type detectedRemote struct {
	repoRemote
	ConnectionID *int64 `json:"connectionId"`
}

func (s *server) detectProjectRemote(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	p, err := s.st.Project(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	cctx, cancel := context.WithTimeout(r.Context(), _ghTimeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, "git", "remote", "get-url", "origin")
	cmd.Dir = p.Path
	out, err := cmd.Output()
	if err != nil {
		writeError(w, fmt.Errorf("%w: project %s has no origin remote", store.ErrInvalid, p.Name))
		return
	}
	remote, err := parseRepoRemote(string(out))
	if err != nil {
		writeError(w, err)
		return
	}
	cs, err := s.st.RepoConnections(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	d := detectedRemote{repoRemote: remote}
	if i := slices.IndexFunc(cs, func(c store.RepoConnection) bool {
		return c.Provider == remote.Provider && c.Host == remote.Host
	}); i >= 0 {
		d.ConnectionID = &cs[i].ID
	}
	writeJSON(w, http.StatusOK, d)
}

// putProjectRemote binds the project to a connection and a repository path on its host. The
// path need not be what origin says: the user may override it.
func (s *server) putProjectRemote(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var in struct {
		ConnectionID int64  `json:"connectionId"`
		Path         string `json:"path"`
	}
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	c, err := s.st.RepoConnection(r.Context(), in.ConnectionID)
	if err != nil {
		writeError(w, err)
		return
	}
	path := cleanRepoPath(in.Path)
	if !validRepoPath(c.Provider, path) {
		writeError(w, fmt.Errorf("%w: remote path %q is not owner/repo on %s", store.ErrInvalid, path, c.Host))
		return
	}
	p, err := s.st.SetProjectRemote(r.Context(), id, c.ID, path)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}
