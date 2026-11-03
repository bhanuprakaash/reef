package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"sync"

	"github.com/bhanuprakaash/reef/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var _requestMu sync.Mutex

type requestInput struct {
	WorkspaceID int64    `json:"workspaceId,omitempty" jsonschema:"workspace id from open_workspace; omit for a Work item and give projectId"`
	ProjectID   int64    `json:"projectId,omitempty" jsonschema:"Project id for a Work item workspace"`
	Title       string   `json:"title" jsonschema:"pull request or merge request title"`
	Description string   `json:"description" jsonschema:"pull request or merge request description"`
	DependsOn   *[]int64 `json:"dependsOn,omitempty" jsonschema:"ids of other requests of this same fix that must merge before this one, as returned by earlier calls; replaces the earlier list, [] clears it, omitted keeps it"`
}

func (c repoCLI) OpenRequest(ctx context.Context, remote repoRemote, dir, head, base, title, description string) (store.RepoRequest, error) {
	cctx, cancel := context.WithTimeout(ctx, _ghTimeout)
	defer cancel()
	run := func(bin string, args ...string) ([]byte, error) {
		cmd := exec.CommandContext(cctx, bin, args...)
		cmd.Dir = dir
		return cmd.Output()
	}
	origin, err := run("git", "remote", "get-url", "--push", "--all", "origin")
	if err != nil {
		return store.RepoRequest{}, fmt.Errorf("%w: cannot read workspace origin", store.ErrConflict)
	}
	for _, address := range strings.Split(strings.TrimSpace(string(origin)), "\n") {
		actual, err := parseRepoRemote(address)
		if err != nil || actual != remote {
			return store.RepoRequest{}, fmt.Errorf("%w: workspace origin does not match the bound repository", store.ErrConflict)
		}
	}
	if _, err := run("git", "push", "-u", "origin", head); err != nil {
		return store.RepoRequest{}, fmt.Errorf("%w: could not push workspace branch; check the connection and retry", store.ErrConflict)
	}
	if c.bin == "gh" {
		return openGitHubRequest(run, remote, head, base, title, description)
	}
	return openGitLabRequest(run, remote, head, base, title, description)
}

func openGitLabRequest(run func(string, ...string) ([]byte, error), remote repoRemote, head, base, title, description string) (store.RepoRequest, error) {
	repo := "https://" + remote.Host + "/" + remote.Path
	type mr struct {
		IID           int64  `json:"iid"`
		URL           string `json:"web_url"`
		State         string `json:"state"`
		Source        string `json:"source_branch"`
		Target        string `json:"target_branch"`
		SourceProject int64  `json:"source_project_id"`
		TargetProject int64  `json:"target_project_id"`
	}
	list := func() ([]mr, error) {
		out, err := run("glab", "mr", "list", "--repo", repo, "--source-branch", head, "-F", "json")
		if err != nil {
			return nil, fmt.Errorf("%w: GitLab merge request lookup failed; check the connection and retry", store.ErrConflict)
		}
		var ms []mr
		if json.Unmarshal(out, &ms) != nil {
			return nil, fmt.Errorf("%w: GitLab returned an invalid merge request list", store.ErrConflict)
		}
		return ms, nil
	}
	find := func(ms []mr) (store.RepoRequest, bool) {
		for _, m := range ms {
			if m.IID > 0 && m.URL != "" && m.State == "opened" && m.Source == head && m.Target == base && m.SourceProject != 0 && m.SourceProject == m.TargetProject {
				return store.RepoRequest{Provider: remote.Provider, Host: remote.Host, Path: remote.Path, Number: m.IID, URL: m.URL, State: m.State, Head: head, Base: base}, true
			}
		}
		return store.RepoRequest{}, false
	}
	ms, err := list()
	if err != nil {
		return store.RepoRequest{}, err
	}
	if r, ok := find(ms); ok {
		return r, nil
	}
	if len(ms) != 0 {
		return store.RepoRequest{}, fmt.Errorf("%w: an open merge request for this branch has a different source repository or target", store.ErrConflict)
	}
	if _, err := run("glab", "mr", "create", "--repo", repo, "--source-branch", head, "--target-branch", base, "--title", title, "--description", description, "--yes"); err != nil {
		return store.RepoRequest{}, fmt.Errorf("%w: GitLab could not create the merge request; check the connection and retry", store.ErrConflict)
	}
	ms, err = list()
	if err != nil {
		return store.RepoRequest{}, err
	}
	if r, ok := find(ms); ok {
		return r, nil
	}
	return store.RepoRequest{}, fmt.Errorf("%w: GitLab did not return the created merge request; retry", store.ErrConflict)
}

func openGitHubRequest(run func(string, ...string) ([]byte, error), remote repoRemote, head, base, title, description string) (store.RepoRequest, error) {
	repo := remote.Host + "/" + remote.Path
	type pr struct {
		Number          int64  `json:"number"`
		URL             string `json:"url"`
		State           string `json:"state"`
		Head            string `json:"headRefName"`
		Base            string `json:"baseRefName"`
		CrossRepository bool   `json:"isCrossRepository"`
	}
	list := func() ([]pr, error) {
		out, err := run("gh", "pr", "list", "--repo", repo, "--head", head, "--state", "open", "--json", "number,url,state,headRefName,baseRefName,isCrossRepository")
		if err != nil {
			return nil, fmt.Errorf("%w: GitHub pull request lookup failed; check the connection and retry", store.ErrConflict)
		}
		var prs []pr
		if json.Unmarshal(out, &prs) != nil {
			return nil, fmt.Errorf("%w: GitHub returned an invalid pull request list", store.ErrConflict)
		}
		return prs, nil
	}
	find := func(prs []pr) (store.RepoRequest, bool) {
		for _, p := range prs {
			if p.Number > 0 && p.URL != "" && p.State == "OPEN" && p.Head == head && p.Base == base && !p.CrossRepository {
				return store.RepoRequest{Provider: remote.Provider, Host: remote.Host, Path: remote.Path, Number: p.Number, URL: p.URL, State: "opened", Head: head, Base: base}, true
			}
		}
		return store.RepoRequest{}, false
	}
	prs, err := list()
	if err != nil {
		return store.RepoRequest{}, err
	}
	if r, ok := find(prs); ok {
		return r, nil
	}
	if len(prs) != 0 {
		return store.RepoRequest{}, fmt.Errorf("%w: an open pull request for this branch has a different source repository or target", store.ErrConflict)
	}
	if _, err := run("gh", "pr", "create", "--repo", repo, "--head", head, "--base", base, "--title", title, "--body", description); err != nil {
		return store.RepoRequest{}, fmt.Errorf("%w: GitHub could not create the pull request; check the connection and retry", store.ErrConflict)
	}
	prs, err = list()
	if err != nil {
		return store.RepoRequest{}, err
	}
	if r, ok := find(prs); ok {
		return r, nil
	}
	return store.RepoRequest{}, fmt.Errorf("%w: GitHub did not return the created pull request; retry", store.ErrConflict)
}

func openRequest(ctx context.Context, st *store.Store, id int64, in requestInput) (store.RepoRequest, error) {
	_requestMu.Lock()
	defer _requestMu.Unlock()
	if strings.TrimSpace(in.Title) == "" {
		return store.RepoRequest{}, fmt.Errorf("%w: title is required", store.ErrInvalid)
	}
	w, err := st.Workspace(ctx, id)
	if err != nil {
		return store.RepoRequest{}, err
	}
	if w.State != store.WorkspaceReady {
		return store.RepoRequest{}, fmt.Errorf("%w: workspace is not ready", store.ErrConflict)
	}
	p, err := st.Project(ctx, w.ProjectID)
	if err != nil {
		return store.RepoRequest{}, err
	}
	if p.RepoConnectionID == nil || p.RemotePath == "" {
		return store.RepoRequest{}, fmt.Errorf("%w: Project has no bound repository", store.ErrConflict)
	}
	c, err := st.RepoConnection(ctx, *p.RepoConnectionID)
	if err != nil {
		return store.RepoRequest{}, err
	}
	if c.State != store.RepoConnected {
		return store.RepoRequest{}, fmt.Errorf("%w: repository connection is disconnected", store.ErrConflict)
	}
	base := w.Base
	if base == "" {
		base = p.BaseBranch
	}
	if base == "" {
		base = strings.TrimPrefix(strings.TrimPrefix(w.BaseRef, "refs/remotes/origin/"), "refs/heads/")
	}
	if base == "" || w.Branch == "" {
		return store.RepoRequest{}, fmt.Errorf("%w: workspace branch and base are required", store.ErrInvalid)
	}
	r, err := repoProviderFor(c.Provider).OpenRequest(ctx, repoRemote{Provider: c.Provider, Host: c.Host, Path: p.RemotePath}, w.Dir, w.Branch, base, in.Title, in.Description)
	if err != nil {
		return store.RepoRequest{}, err
	}
	r.WorkspaceID, r.WorkItemID, r.SpaceID = w.ID, w.WorkItemID, w.SpaceID
	r, err = st.PutRepoRequest(ctx, r)
	if err != nil || in.DependsOn == nil {
		return r, err
	}
	ordered, err := st.SetRepoRequestDependsOn(ctx, r.ID, *in.DependsOn)
	if err != nil {
		return store.RepoRequest{}, fmt.Errorf("%w (the request is open: %s)", err, r.URL)
	}
	return ordered, nil
}
func (s *server) openRepoRequest(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var in requestInput
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	result, err := openRequest(r.Context(), s.st, id, in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (s *server) workspaceRequests(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if _, err = s.st.Workspace(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	rows, err := s.st.WorkspaceRequests(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}
func (s *server) workItemRequests(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	item, err := s.st.WorkItem(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	if item.ParentID != nil {
		id = *item.ParentID
	}
	rows, err := s.st.WorkItemRequests(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}
func (m *mcpServer) openRepoRequest(ctx context.Context, _ *mcp.CallToolRequest, in requestInput) (*mcp.CallToolResult, store.RepoRequest, error) {
	if _, err := m.currentName(ctx); err != nil {
		return nil, store.RepoRequest{}, err
	}
	sp, err := m.st.Space(ctx, m.tok.SpaceID)
	if err != nil {
		return nil, store.RepoRequest{}, err
	}
	if in.WorkspaceID == 0 && in.ProjectID != 0 && sp.WorkItemID != nil {
		item, err := m.st.WorkItem(ctx, *sp.WorkItemID)
		if err != nil {
			return nil, store.RepoRequest{}, err
		}
		top := item.ID
		if item.ParentID != nil {
			top = *item.ParentID
		}
		rows, err := m.st.WorkItemWorkspaces(ctx, top)
		if err != nil {
			return nil, store.RepoRequest{}, err
		}
		for _, row := range rows {
			if row.ProjectID == in.ProjectID {
				in.WorkspaceID = row.ID
				break
			}
		}
	}
	w, err := m.st.Workspace(ctx, in.WorkspaceID)
	if err != nil {
		return nil, store.RepoRequest{}, err
	}
	allowed := w.SpaceID != nil && *w.SpaceID == sp.ID
	if sp.WorkItemID != nil && w.WorkItemID != nil {
		item, e := m.st.WorkItem(ctx, *sp.WorkItemID)
		if e != nil {
			return nil, store.RepoRequest{}, e
		}
		top := item.ID
		if item.ParentID != nil {
			top = *item.ParentID
		}
		allowed = *w.WorkItemID == top
	}
	if !allowed {
		return nil, store.RepoRequest{}, fmt.Errorf("%w: workspace does not belong to this Space", store.ErrConflict)
	}
	result, err := openRequest(ctx, m.st, w.ID, in)
	return nil, result, err
}
