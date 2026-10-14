package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/bhanuprakaash/reef/store"
	"github.com/bhanuprakaash/reef/worktree"
)

const _ghTimeout = 30 * time.Second

// ghPR is the gh pr view output this poll reads.
type ghPR struct {
	Number            int       `json:"number"`
	URL               string    `json:"url"`
	State             string    `json:"state"`
	StatusCheckRollup []ghCheck `json:"statusCheckRollup"`
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

// _runID finds the run id in a CheckRun's detailsUrl, such as
// https://github.com/x/y/actions/runs/123/job/456.
var _runID = regexp.MustCompile(`/actions/runs/(\d+)/`)

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

// runID is the run id in a CheckRun's detailsUrl, or "" when there is none to find.
func (c ghCheck) runID() string {
	if c.Typename != "CheckRun" {
		return ""
	}
	m := _runID.FindStringSubmatch(c.DetailsURL)
	if m == nil {
		return ""
	}
	return m[1]
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
	case "FAILURE", "ERROR":
		return "failed"
	default:
		return strings.ToLower(v)
	}
}

// ghInterval is how often reef serve polls GitHub, REEF_GH_INTERVAL or 60s.
func ghInterval() time.Duration {
	if v := os.Getenv("REEF_GH_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return 60 * time.Second
}

// startGitHubPoll polls every feature with a worktree for its PR and CI status every
// ghInterval, until the server closes.
func (s *server) startGitHubPoll() {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		t := time.NewTicker(ghInterval())
		defer t.Stop()
		for {
			select {
			case <-s.ctx.Done():
				return
			case <-t.C:
				s.pollGitHub(s.ctx)
			}
		}
	}()
}

// pollGitHub checks every feature with a worktree against its PR, and posts a reef note in
// the feature's most recent channel when the PR's state changes or a check finishes. No gh
// on PATH: nothing is posted, logged once.
func (s *server) pollGitHub(ctx context.Context) {
	ghPath, err := exec.LookPath("gh")
	if err != nil {
		if !s.ghNoGhLogged {
			log.Print("github poll: gh is not on PATH")
			s.ghNoGhLogged = true
		}
		return
	}
	features, err := s.st.Features(ctx, nil)
	if err != nil {
		log.Printf("github poll: list features: %v", err)
		return
	}
	for _, f := range features {
		if err := s.pollFeatureGitHub(ctx, ghPath, f); err != nil {
			log.Printf("github poll: feature %d: %v", f.ID, err)
		}
	}
}

// pollFeatureGitHub polls the PR of one feature. A feature with no worktree yet, or no gh
// PR (no PR, or gh not logged in), posts nothing.
func (s *server) pollFeatureGitHub(ctx context.Context, ghPath string, f store.Feature) error {
	p, err := s.st.Project(ctx, f.ProjectID)
	if err != nil {
		return err
	}
	dir := worktree.Path(s.dataDir, p.ID, f.Slug)
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		return nil
	}
	pr, err := runGh(ctx, ghPath, p.Path, f.Slug)
	if err != nil {
		s.logGhErrOnce(f.ID, err)
		return nil
	}
	if err := s.notePRState(ctx, f, pr); err != nil {
		return err
	}
	return s.noteFinishedChecks(ctx, f, pr)
}

// logGhErrOnce logs a gh error for the feature once, and again only once its text changes,
// so a cause such as "no PR for the branch" does not fill the log on every poll.
func (s *server) logGhErrOnce(featureID int64, err error) {
	text := err.Error()
	if s.ghFeatureErrLogged[featureID] == text {
		return
	}
	log.Printf("github poll: feature %d: %v", featureID, err)
	s.ghFeatureErrLogged[featureID] = text
}

func runGh(ctx context.Context, ghPath, dir, slug string) (ghPR, error) {
	cctx, cancel := context.WithTimeout(ctx, _ghTimeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, ghPath, "pr", "view", worktree.Branch(slug),
		"--json", "number,url,state,statusCheckRollup")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return ghPR{}, fmt.Errorf("gh pr view: %s", strings.TrimSpace(string(ee.Stderr)))
		}
		return ghPR{}, fmt.Errorf("gh pr view: %w", err)
	}
	var pr ghPR
	if err := json.Unmarshal(out, &pr); err != nil {
		return ghPR{}, fmt.Errorf("parse gh pr view output: %w", err)
	}
	return pr, nil
}

// notePRState posts a reef note when the PR appeared or changed state since the last poll.
func (s *server) notePRState(ctx context.Context, f store.Feature, pr ghPR) error {
	stored, ok, err := s.st.FeatureGitHub(ctx, f.ID)
	if err != nil {
		return err
	}
	if ok && stored.PRNumber == int64(pr.Number) && stored.PRState == pr.State {
		return nil
	}
	body := fmt.Sprintf("PR #%d %s: %s", pr.Number, strings.ToLower(pr.State), pr.URL)
	if err := s.noteFeature(ctx, f.ID, body); err != nil {
		return err
	}
	return s.st.SetFeatureGitHub(ctx, store.FeatureGitHub{
		FeatureID: f.ID, PRNumber: int64(pr.Number), PRState: pr.State, PRURL: pr.URL,
	})
}

// noteFinishedChecks posts a reef note for each check run that finished since it was last
// notified, once per run: the same check name run again, such as a rerun on a later push,
// is noted again.
func (s *server) noteFinishedChecks(ctx context.Context, f store.Feature, pr ghPR) error {
	for _, c := range pr.StatusCheckRollup {
		finished, word := c.outcome()
		if !finished {
			continue
		}
		name, key := c.checkName(), c.runKey()
		notified, err := s.st.NotifiedGitHubCheck(ctx, f.ID, name, key)
		if err != nil {
			return err
		}
		if notified {
			continue
		}
		if err := s.noteFeature(ctx, f.ID, fmt.Sprintf("%s %s", name, word)); err != nil {
			return err
		}
		if err := s.st.MarkGitHubCheckNotified(ctx, f.ID, name, key); err != nil {
			return err
		}
	}
	return nil
}

// noteFeature posts a reef note in the feature's most recent channel.
func (s *server) noteFeature(ctx context.Context, featureID int64, body string) error {
	ch, err := s.st.FeatureLatestChannel(ctx, featureID)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = s.st.CreateMessage(ctx, store.Message{
		ChannelID: ch.ID, Author: store.ReefAuthor, Body: body,
	})
	return err
}
