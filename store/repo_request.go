package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"
)

type RepoRequest struct {
	ID          int64     `json:"id"`
	Provider    string    `json:"provider"`
	Host        string    `json:"host"`
	Path        string    `json:"path"`
	Number      int64     `json:"number"`
	URL         string    `json:"url"`
	State       string    `json:"state"`
	Head        string    `json:"head"`
	Base        string    `json:"base"`
	WorkspaceID int64     `json:"workspaceId"`
	WorkItemID  *int64    `json:"workItemId"`
	SpaceID     *int64    `json:"spaceId"`
	DependsOn   []int64   `json:"dependsOn"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	// Review, Checks and CheckedAt are the last status read from the provider. CheckError is why
	// the last read failed, a fixed phrase, and is empty after a good read.
	Review     string      `json:"review"`
	Checks     []RepoCheck `json:"checks"`
	CheckedAt  *time.Time  `json:"checkedAt"`
	CheckError string      `json:"checkError"`
	// Failure is the failed checks of the last read and FailureHandled the failure Reef already
	// acted on; they differ only while a newly failed check waits to be routed.
	Failure        string `json:"-"`
	FailureHandled string `json:"-"`
}

const _repoRequestCols = "id, provider, host, path, number, url, state, head, base, workspace_id, work_item_id, space_id, depends_on, created_at, updated_at, review, checks, failure, failure_handled, checked_at, check_error"

func (s *Store) PutRepoRequest(ctx context.Context, r RepoRequest) (RepoRequest, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `INSERT INTO repo_request (provider, host, path, number, url, state, head, base, workspace_id, work_item_id, space_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(host, path, number) DO UPDATE SET url=excluded.url, state=excluded.state, updated_at=excluded.updated_at WHERE repo_request.workspace_id=excluded.workspace_id`, r.Provider, r.Host, r.Path, r.Number, r.URL, r.State, r.Head, r.Base, r.WorkspaceID, r.WorkItemID, r.SpaceID, now, now)
	if err != nil {
		return RepoRequest{}, fmt.Errorf("save repository request: %w", err)
	}
	got, err := scanRepoRequest(s.db.QueryRowContext(ctx, "SELECT "+_repoRequestCols+" FROM repo_request WHERE host = ? AND path = ? AND number = ?", r.Host, r.Path, r.Number))
	if err != nil {
		return RepoRequest{}, err
	}
	if got.WorkspaceID != r.WorkspaceID {
		return RepoRequest{}, fmt.Errorf("%w: repository request is linked to another workspace", ErrConflict)
	}
	return got, nil
}

// SetRepoRequestDependsOn records which other requests of the same fix (the quick fix of that
// name in its Space, or the Work item) must merge before request id. It replaces the earlier
// list and refuses the request itself, a request outside the fix and a cycle. Reef never
// merges: the list is only the order shown to the user.
func (s *Store) SetRepoRequestDependsOn(ctx context.Context, id int64, deps []int64) (RepoRequest, error) {
	r, err := scanRepoRequest(s.db.QueryRowContext(ctx, "SELECT "+_repoRequestCols+" FROM repo_request WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return RepoRequest{}, fmt.Errorf("repository request %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return RepoRequest{}, err
	}
	w, err := s.Workspace(ctx, r.WorkspaceID)
	if err != nil {
		return RepoRequest{}, err
	}
	rows, err := s.db.QueryContext(ctx, "SELECT "+_repoRequestCols+" FROM repo_request WHERE workspace_id IN "+
		"(SELECT id FROM workspace WHERE (? IS NOT NULL AND work_item_id = ?) OR "+
		"(? IS NULL AND space_id = ? AND name = ?))", w.WorkItemID, w.WorkItemID, w.WorkItemID, w.SpaceID, w.Name)
	if err != nil {
		return RepoRequest{}, fmt.Errorf("list repository requests: %w", err)
	}
	fix, err := scanAll(rows, scanRepoRequest)
	if err != nil {
		return RepoRequest{}, err
	}
	next := make(map[int64][]int64, len(fix))
	for _, f := range fix {
		next[f.ID] = f.DependsOn
	}
	slices.Sort(deps)
	deps = slices.Compact(deps)
	for _, d := range deps {
		if _, ok := next[d]; !ok || d == id {
			return RepoRequest{}, fmt.Errorf("%w: request %d is not another request of this fix", ErrInvalid, d)
		}
	}
	next[id] = deps
	for stack, seen := slices.Clone(deps), map[int64]bool{}; len(stack) > 0; {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if n == id {
			return RepoRequest{}, fmt.Errorf("%w: request %d would wait on itself", ErrInvalid, id)
		}
		if !seen[n] {
			seen[n] = true
			stack = append(stack, next[n]...)
		}
	}
	out, err := json.Marshal(deps)
	if err != nil {
		return RepoRequest{}, err
	}
	if _, err := s.db.ExecContext(ctx, "UPDATE repo_request SET depends_on = ? WHERE id = ?", string(out), id); err != nil {
		return RepoRequest{}, fmt.Errorf("save repository request order: %w", err)
	}
	r.DependsOn = deps
	return r, nil
}

func (s *Store) WorkspaceRequests(ctx context.Context, id int64) ([]RepoRequest, error) {
	return s.repoRequests(ctx, "workspace_id", id)
}
func (s *Store) WorkItemRequests(ctx context.Context, id int64) ([]RepoRequest, error) {
	return s.repoRequests(ctx, "work_item_id", id)
}
func (s *Store) SpaceRequests(ctx context.Context, id int64) ([]RepoRequest, error) {
	return s.repoRequests(ctx, "space_id", id)
}
func (s *Store) repoRequests(ctx context.Context, column string, id int64) ([]RepoRequest, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+_repoRequestCols+" FROM repo_request WHERE "+column+" = ? ORDER BY id", id)
	if err != nil {
		return nil, fmt.Errorf("list repository requests: %w", err)
	}
	result, err := scanAll(rows, scanRepoRequest)
	if err != nil {
		return nil, err
	}
	if result == nil {
		result = []RepoRequest{}
	}
	return result, nil
}
func scanRepoRequest(row interface{ Scan(...any) error }) (RepoRequest, error) {
	var r RepoRequest
	var created, updated, deps, checks, checked string
	if err := row.Scan(&r.ID, &r.Provider, &r.Host, &r.Path, &r.Number, &r.URL, &r.State, &r.Head, &r.Base, &r.WorkspaceID, &r.WorkItemID, &r.SpaceID, &deps, &created, &updated, &r.Review, &checks, &r.Failure, &r.FailureHandled, &checked, &r.CheckError); err != nil {
		return r, err
	}
	if err := json.Unmarshal([]byte(checks), &r.Checks); err != nil {
		return r, fmt.Errorf("repository request %d: bad checks: %w", r.ID, err)
	}
	var err error
	if err = json.Unmarshal([]byte(deps), &r.DependsOn); err != nil {
		return r, err
	}
	if checked != "" {
		t, err := time.Parse(time.RFC3339Nano, checked)
		if err != nil {
			return r, err
		}
		r.CheckedAt = &t
	}
	if r.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return r, err
	}
	r.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated)
	return r, err
}
