package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// States of a RepoRequest. A request closed without a merge is not merged, and may reopen.
const (
	RequestOpened = "opened"
	RequestClosed = "closed"
	RequestMerged = "merged"
)

// RepoCheck is one check of a request: pass, fail, pending or neutral (finished, neither). Key
// tells one run of a check from the next, so a rerun that fails again is a new failure.
type RepoCheck struct {
	Name  string `json:"name"`
	State string `json:"state"`
	Key   string `json:"key"`
}

// TrackedRequests lists the requests not yet merged, other than those of a Project that is
// being or has been deleted.
func (s *Store) TrackedRequests(ctx context.Context) ([]RepoRequest, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+_repoRequestCols+" FROM repo_request WHERE state != ? "+
		"AND workspace_id IN (SELECT w.id FROM workspace w JOIN project p ON p.id = w.project_id WHERE "+
		"p.deleting_at IS NULL AND p.deleted_at IS NULL) ORDER BY id", RequestMerged)
	if err != nil {
		return nil, fmt.Errorf("list tracked requests: %w", err)
	}
	return scanAll(rows, scanRepoRequest)
}

// RecordRepoStatus stores what the provider said about request id and clears the last read's
// error, and returns the request. A failure that has gone is forgotten, so the same failing run
// seen again later routes again.
func (s *Store) RecordRepoStatus(ctx context.Context, id int64, state, review string, checks []RepoCheck) (RepoRequest, error) {
	if !slices.Contains([]string{RequestOpened, RequestClosed, RequestMerged}, state) {
		return RepoRequest{}, fmt.Errorf("%w: repository request state %q", ErrInvalid, state)
	}
	encoded, err := json.Marshal(append([]RepoCheck{}, checks...))
	if err != nil {
		return RepoRequest{}, fmt.Errorf("encode checks: %w", err)
	}
	var failed []string
	for _, c := range checks {
		if c.State == "fail" {
			failed = append(failed, c.Name+" "+c.Key)
		}
	}
	slices.Sort(failed)
	failure := strings.Join(failed, "\n")
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = s.db.ExecContext(ctx, "UPDATE repo_request SET state = ?, review = ?, checks = ?, failure = ?, "+
		"failure_handled = CASE WHEN ? = '' THEN '' ELSE failure_handled END, checked_at = ?, check_error = '', "+
		"updated_at = ? WHERE id = ?", state, review, string(encoded), failure, failure, now, now, id)
	if err != nil {
		return RepoRequest{}, fmt.Errorf("record request status: %w", err)
	}
	return s.repoRequest(ctx, id)
}

// RecordRepoError keeps why the last read of request id failed, a fixed phrase, and when.
func (s *Store) RecordRepoError(ctx context.Context, id int64, reason string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE repo_request SET check_error = ?, checked_at = ? WHERE id = ?",
		reason, time.Now().UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return fmt.Errorf("record request error: %w", err)
	}
	return nil
}

// HandleRepoFailure marks the failure of request id as acted on, unless it has changed since.
func (s *Store) HandleRepoFailure(ctx context.Context, id int64, failure string) error {
	_, err := s.db.ExecContext(ctx,
		"UPDATE repo_request SET failure_handled = ? WHERE id = ? AND failure = ?", failure, id, failure)
	if err != nil {
		return fmt.Errorf("handle request failure: %w", err)
	}
	return nil
}

func (s *Store) repoRequest(ctx context.Context, id int64) (RepoRequest, error) {
	r, err := scanRepoRequest(s.db.QueryRowContext(ctx, "SELECT "+_repoRequestCols+" FROM repo_request WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return RepoRequest{}, fmt.Errorf("repository request %d: %w", id, ErrNotFound)
	}
	return r, err
}
