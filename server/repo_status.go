package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"strconv"
	"strings"

	"github.com/bhanuprakaash/reef/store"
)

// repoStatus is what a provider says about one pull request or merge request. State is
// store.RequestOpened, RequestClosed or RequestMerged.
type repoStatus struct {
	State  string
	Review string
	Checks []store.RepoCheck
}

func (c repoCLI) Status(ctx context.Context, remote repoRemote, number int64) (repoStatus, error) {
	path, err := exec.LookPath(c.bin)
	if err != nil {
		return repoStatus{}, fmt.Errorf("%s is not on PATH", c.bin)
	}
	cctx, cancel := context.WithTimeout(ctx, _ghTimeout)
	defer cancel()
	args := []string{"pr", "view", strconv.FormatInt(number, 10), "--repo", remote.Host + "/" + remote.Path,
		"--json", "state,reviewDecision,statusCheckRollup"}
	if c.bin == "glab" {
		args = []string{"api", "--hostname", remote.Host,
			"projects/" + url.PathEscape(remote.Path) + "/merge_requests/" + strconv.FormatInt(number, 10)}
	}
	out, err := exec.CommandContext(cctx, path, args...).Output()
	if err != nil {
		if cctx.Err() != nil {
			return repoStatus{}, fmt.Errorf("%s did not answer in %s", c.bin, _ghTimeout)
		}
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return repoStatus{}, c.statusFailure(remote.Host, string(ee.Stderr))
		}
		return repoStatus{}, fmt.Errorf("run %s: %w", c.bin, err)
	}
	if c.bin == "glab" {
		return gitLabStatus(out)
	}
	return gitHubStatus(out)
}

// statusFailure maps what the CLI said on stderr to a fixed phrase.
func (c repoCLI) statusFailure(host, stderr string) error {
	s := strings.ToLower(stderr)
	switch {
	case strings.Contains(s, "404") || strings.Contains(s, "not found") || strings.Contains(s, "no pull requests"):
		return errors.New("request not found on the provider")
	case strings.Contains(s, "dial tcp") || strings.Contains(s, "no such host") || strings.Contains(s, "error connecting"):
		return errors.New("provider unreachable")
	}
	return c.loginFailure(host, stderr)
}

func gitHubStatus(out []byte) (repoStatus, error) {
	var pr ghPR
	if err := json.Unmarshal(out, &pr); err != nil {
		return repoStatus{}, errors.New("gh did not return the request status")
	}
	st := repoStatus{State: map[string]string{"MERGED": store.RequestMerged, "CLOSED": store.RequestClosed}[pr.State],
		Review: strings.ToLower(pr.ReviewDecision)}
	if st.State == "" {
		st.State = store.RequestOpened
	}
	for _, c := range pr.StatusCheckRollup {
		state := "pending"
		if finished, word := c.outcome(); finished {
			state = map[string]string{"succeeded": "pass", "failed": "fail"}[word]
			if state == "" {
				state = "neutral"
			}
		}
		st.Checks = append(st.Checks, store.RepoCheck{Name: c.checkName(), State: state, Key: c.runKey()})
	}
	return st, nil
}

func gitLabStatus(out []byte) (repoStatus, error) {
	var mr struct {
		State        string `json:"state"`
		MergeStatus  string `json:"detailed_merge_status"`
		HeadPipeline *struct {
			ID     int64  `json:"id"`
			Status string `json:"status"`
		} `json:"head_pipeline"`
	}
	if err := json.Unmarshal(out, &mr); err != nil || mr.State == "" {
		return repoStatus{}, errors.New("glab did not return the request status")
	}
	st := repoStatus{State: map[string]string{"merged": store.RequestMerged, "closed": store.RequestClosed}[mr.State],
		Review: map[string]string{"not_approved": "review_required", "requested_changes": "changes_requested"}[mr.MergeStatus]}
	if st.State == "" {
		st.State = store.RequestOpened
	}
	if p := mr.HeadPipeline; p != nil {
		state := map[string]string{"success": "pass", "failed": "fail", "canceled": "neutral", "skipped": "neutral"}[p.Status]
		if state == "" {
			state = "pending"
		}
		st.Checks = []store.RepoCheck{{Name: "pipeline", State: state, Key: strconv.FormatInt(p.ID, 10)}}
	}
	return st, nil
}
