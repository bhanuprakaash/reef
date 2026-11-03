package store

import (
	"context"
	"fmt"
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
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

const _repoRequestCols = "id, provider, host, path, number, url, state, head, base, workspace_id, work_item_id, space_id, created_at, updated_at"

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

func (s *Store) WorkspaceRequests(ctx context.Context, id int64) ([]RepoRequest, error) {
	return s.repoRequests(ctx, "workspace_id", id)
}
func (s *Store) WorkItemRequests(ctx context.Context, id int64) ([]RepoRequest, error) {
	return s.repoRequests(ctx, "work_item_id", id)
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
	var created, updated string
	if err := row.Scan(&r.ID, &r.Provider, &r.Host, &r.Path, &r.Number, &r.URL, &r.State, &r.Head, &r.Base, &r.WorkspaceID, &r.WorkItemID, &r.SpaceID, &created, &updated); err != nil {
		return r, err
	}
	var err error
	if r.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return r, err
	}
	r.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated)
	return r, err
}
