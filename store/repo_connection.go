package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const (
	RepoGitHub = "github"
	RepoGitLab = "gitlab"

	RepoConnected    = "connected"
	RepoDisconnected = "disconnected"
)

// RepoConnection is the login of a repository provider (GitHub or GitLab) on one host, as the
// gh or glab CLI holds it. Reef keeps the provider, host and account the CLI is logged in as,
// and the last check's state; never a token. It is separate from Connection, which is MCP.
type RepoConnection struct {
	ID        int64     `json:"id"`
	Provider  string    `json:"provider"`
	Host      string    `json:"host"`
	Account   string    `json:"account"`
	State     string    `json:"state"`
	Reason    string    `json:"reason"`
	CheckedAt time.Time `json:"checkedAt"`
}

const _repoConnectionCols = "id, provider, host, account, state, reason, checked_at"

// PutRepoConnection adds the connection for c.Provider and c.Host, or replaces the account,
// state, reason and check time of the one there.
func (s *Store) PutRepoConnection(ctx context.Context, c RepoConnection) (RepoConnection, error) {
	if c.State != RepoConnected && c.State != RepoDisconnected {
		return RepoConnection{}, fmt.Errorf("%w: repository connection state %q", ErrInvalid, c.State)
	}
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO repo_connection (provider, host, account, state, reason, checked_at) "+
			"VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT (provider, host) DO UPDATE SET "+
			"account = excluded.account, state = excluded.state, reason = excluded.reason, "+
			"checked_at = excluded.checked_at",
		c.Provider, c.Host, c.Account, c.State, c.Reason, c.CheckedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return RepoConnection{}, fmt.Errorf("put repository connection: %w", err)
	}
	return scanRepoConnection(s.db.QueryRowContext(ctx,
		"SELECT "+_repoConnectionCols+" FROM repo_connection WHERE provider = ? AND host = ?",
		c.Provider, c.Host))
}

// RepoConnection returns the connection with the id, or ErrNotFound.
func (s *Store) RepoConnection(ctx context.Context, id int64) (RepoConnection, error) {
	c, err := scanRepoConnection(s.db.QueryRowContext(ctx,
		"SELECT "+_repoConnectionCols+" FROM repo_connection WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return RepoConnection{}, fmt.Errorf("repository connection %d: %w", id, ErrNotFound)
	}
	return c, err
}

// RepoConnections lists every connection by id.
func (s *Store) RepoConnections(ctx context.Context) ([]RepoConnection, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT "+_repoConnectionCols+" FROM repo_connection ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("list repository connections: %w", err)
	}
	return scanAll(rows, scanRepoConnection)
}

// SetProjectRemote binds the project to a repository connection and the repository path on its
// host, such as owner/repo.
func (s *Store) SetProjectRemote(ctx context.Context, projectID, connectionID int64, path string) (Project, error) {
	res, err := s.db.ExecContext(ctx,
		"UPDATE project SET repo_connection_id = ?, remote_path = ? WHERE id = ?",
		connectionID, path, projectID)
	if err != nil {
		return Project{}, fmt.Errorf("set project remote: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return Project{}, fmt.Errorf("set project remote: %w", err)
	}
	if n == 0 {
		return Project{}, fmt.Errorf("project %d: %w", projectID, ErrNotFound)
	}
	return s.Project(ctx, projectID)
}

func scanRepoConnection(r interface{ Scan(...any) error }) (RepoConnection, error) {
	var c RepoConnection
	var checked string
	if err := r.Scan(&c.ID, &c.Provider, &c.Host, &c.Account, &c.State, &c.Reason, &checked); err != nil {
		return RepoConnection{}, err
	}
	t, err := time.Parse(time.RFC3339Nano, checked)
	if err != nil {
		return RepoConnection{}, fmt.Errorf("repository connection %d: bad checked_at: %w", c.ID, err)
	}
	c.CheckedAt = t
	return c, nil
}
