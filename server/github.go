package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/bhanuprakaash/reef/store"
	"github.com/bhanuprakaash/reef/worktree"
)

const (
	_ghTimeout = 30 * time.Second
	// _noPRText is what gh pr view says on stderr for a branch with no PR.
	_noPRText = "no pull requests found"
)

// ghPR is the gh pr view output the pull request panel and Status read.
type ghPR struct {
	Number            int       `json:"number"`
	Title             string    `json:"title"`
	URL               string    `json:"url"`
	State             string    `json:"state"`
	IsDraft           bool      `json:"isDraft"`
	Mergeable         string    `json:"mergeable"`
	ReviewDecision    string    `json:"reviewDecision"`
	HeadRefName       string    `json:"headRefName"`
	BaseRefName       string    `json:"baseRefName"`
	StatusCheckRollup []ghCheck `json:"statusCheckRollup"`
	Body              string    `json:"body"`
	Additions         int       `json:"additions"`
	Deletions         int       `json:"deletions"`
	Files             []ghFile  `json:"files"`
}

type ghFile struct {
	Path      string `json:"path"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
}

// ghCheck is one item of statusCheckRollup: a CheckRun (name, status, conclusion, detailsUrl,
// completedAt) or a StatusContext (context, state, targetUrl).
type ghCheck struct {
	Typename    string `json:"__typename"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	Conclusion  string `json:"conclusion"`
	Context     string `json:"context"`
	State       string `json:"state"`
	DetailsURL  string `json:"detailsUrl"`
	TargetURL   string `json:"targetUrl"`
	StartedAt   string `json:"startedAt"`
	CompletedAt string `json:"completedAt"`
}

// checkName is the name a note calls the check by.
func (c ghCheck) checkName() string {
	if c.Typename == "StatusContext" {
		return c.Context
	}
	return c.Name
}

// runKey identifies the check's run, so the same check name on a later run is noted and
// woken again. It is the smallest value gh gives that distinguishes one run of a check.
func (c ghCheck) runKey() string {
	if c.Typename == "StatusContext" {
		if c.TargetURL != "" {
			return c.TargetURL
		}
		return c.StartedAt
	}
	if c.DetailsURL != "" {
		return c.DetailsURL
	}
	if c.CompletedAt != "" {
		return c.CompletedAt
	}
	return c.StartedAt
}

// outcome reports whether the check has finished, and the word a note uses for it.
func (c ghCheck) outcome() (finished bool, word string) {
	if c.Typename == "StatusContext" {
		state := strings.ToUpper(c.State)
		if state == "" || state == "PENDING" || state == "EXPECTED" {
			return false, ""
		}
		return true, resultWord(state)
	}
	if !strings.EqualFold(c.Status, "COMPLETED") {
		return false, ""
	}
	return true, resultWord(strings.ToUpper(c.Conclusion))
}

func resultWord(v string) string {
	switch v {
	case "SUCCESS":
		return "succeeded"
	case "FAILURE", "ERROR", "TIMED_OUT", "STARTUP_FAILURE":
		return "failed"
	default:
		return strings.ToLower(v)
	}
}

func runGh(ctx context.Context, ghPath, dir, slug string) (ghPR, error) {
	cctx, cancel := context.WithTimeout(ctx, _ghTimeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, ghPath, "pr", "view", worktree.Branch(slug),
		"--json", "number,title,url,state,isDraft,mergeable,reviewDecision,headRefName,"+
			"baseRefName,statusCheckRollup,body,additions,deletions,files")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ghPR{}, ghFailure("gh pr view", err)
	}
	var pr ghPR
	if err := json.Unmarshal(out, &pr); err != nil {
		return ghPR{}, fmt.Errorf("parse gh pr view output: %w", err)
	}
	return pr, nil
}

// ghFailure is err with gh's own message, when it printed one, so a caller can show it.
func ghFailure(what string, err error) error {
	var ee *exec.ExitError
	if errors.As(err, &ee) && len(ee.Stderr) > 0 {
		return fmt.Errorf("%s: %s", what, strings.TrimSpace(string(ee.Stderr)))
	}
	return fmt.Errorf("%s: %w", what, err)
}

// prView is a work item's PR as the window shows it. Each check's state is pass, fail or
// pending; canMerge is true only for an open PR gh calls mergeable.
type prView struct {
	Number         int       `json:"number"`
	Title          string    `json:"title"`
	State          string    `json:"state"`
	URL            string    `json:"url"`
	IsDraft        bool      `json:"isDraft"`
	ReviewDecision string    `json:"reviewDecision"`
	Base           string    `json:"base"`
	Head           string    `json:"head"`
	CanMerge       bool      `json:"canMerge"`
	Checks         []prCheck `json:"checks"`
	Body           string    `json:"body"`
	Additions      int       `json:"additions"`
	Deletions      int       `json:"deletions"`
	Files          []prFile  `json:"files"`
}

type prFile struct {
	Path      string `json:"path"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
}

type prCheck struct {
	Name  string `json:"name"`
	State string `json:"state"`
}

func newPRView(pr ghPR) prView {
	checks := make([]prCheck, len(pr.StatusCheckRollup))
	for i, c := range pr.StatusCheckRollup {
		state := "pending"
		if finished, word := c.outcome(); finished {
			state = "fail"
			if slices.Contains([]string{"succeeded", "skipped", "neutral"}, word) {
				state = "pass"
			}
		}
		checks[i] = prCheck{Name: c.checkName(), State: state}
	}
	files := make([]prFile, len(pr.Files))
	for i, f := range pr.Files {
		files[i] = prFile(f)
	}
	return prView{
		Number: pr.Number, Title: pr.Title, State: pr.State, URL: pr.URL, IsDraft: pr.IsDraft,
		ReviewDecision: pr.ReviewDecision, Base: pr.BaseRefName, Head: pr.HeadRefName,
		CanMerge: pr.State == "OPEN" && pr.Mergeable == "MERGEABLE", Checks: checks,
		Body: pr.Body, Additions: pr.Additions, Deletions: pr.Deletions, Files: files,
	}
}

// workItemPR finds the work item in the request and the project folder to run gh in. It writes
// the error and reports ok = false when it cannot.
func (s *server) workItemPR(w http.ResponseWriter, r *http.Request) (
	f store.WorkItem, p store.Project, ghPath string, ok bool,
) {
	id, err := pathID(r)
	if err == nil {
		f, err = s.st.WorkItem(r.Context(), id)
	}
	if err == nil {
		p, err = s.st.Project(r.Context(), f.ProjectID)
	}
	if err != nil {
		writeError(w, err)
		return f, p, "", false
	}
	if ghPath, err = exec.LookPath("gh"); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "gh is not on PATH"})
		return f, p, "", false
	}
	return f, p, ghPath, true
}

// getWorkItemPR answers with the work item's PR, or {"pr": null} when it has none.
func (s *server) getWorkItemPR(w http.ResponseWriter, r *http.Request) {
	f, p, ghPath, ok := s.workItemPR(w, r)
	if !ok {
		return
	}
	pr, err := runGh(r.Context(), ghPath, p.Path, f.TreeKey())
	if err != nil {
		if strings.Contains(err.Error(), _noPRText) {
			writeJSON(w, http.StatusOK, map[string]*prView{"pr": nil})
			return
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	view := newPRView(pr)
	writeJSON(w, http.StatusOK, map[string]*prView{"pr": &view})
}

// mergeWorkItemPR merges the work item's PR with a merge commit and answers with its new state.
// gh's own message is the error when it refuses.
func (s *server) mergeWorkItemPR(w http.ResponseWriter, r *http.Request) {
	f, p, ghPath, ok := s.workItemPR(w, r)
	if !ok {
		return
	}
	pr, err := runGh(r.Context(), ghPath, p.Path, f.TreeKey())
	if err != nil {
		status := http.StatusBadGateway
		if strings.Contains(err.Error(), _noPRText) {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	cctx, cancel := context.WithTimeout(r.Context(), _ghTimeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, ghPath, "pr", "merge", strconv.Itoa(pr.Number), "--merge")
	cmd.Dir = p.Path
	if _, err := cmd.Output(); err != nil {
		err = ghFailure("gh pr merge", err)
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	if pr, err = runGh(r.Context(), ghPath, p.Path, f.TreeKey()); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	view := newPRView(pr)
	writeJSON(w, http.StatusOK, map[string]*prView{"pr": &view})
}
