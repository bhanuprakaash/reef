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
	"slices"
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
	case "FAILURE", "ERROR", "TIMED_OUT", "STARTUP_FAILURE":
		return "failed"
	default:
		return strings.ToLower(v)
	}
}

// ghInterval is how often reef serve polls GitHub, REEF_GH_INTERVAL or 60s. A zero or
// negative value, which would panic time.NewTicker, also defaults.
func ghInterval() time.Duration {
	if v := os.Getenv("REEF_GH_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return 60 * time.Second
}

// startGitHubPoll polls every feature with a worktree for its PR and CI status once right
// away, then every ghInterval until the server closes, so a restart does not sit on a stale
// note for a whole interval. Both the first poll and the ticker run on the same background
// goroutine, so construction never blocks on gh.
func (s *server) startGitHubPoll() {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.pollGitHub(s.ctx)
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
		s.logGhNoPathOnce()
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
	return s.noteFinishedChecks(ctx, f, pr, ghPath, p.Path)
}

// logGhNoPathOnce logs that gh is not on PATH once, guarded by ghMu since the first poll
// runs on its own goroutine and can overlap a caller that polls or inspects the server right
// after construction, such as a test.
func (s *server) logGhNoPathOnce() {
	s.ghMu.Lock()
	defer s.ghMu.Unlock()
	if s.ghNoGhLogged {
		return
	}
	log.Print("github poll: gh is not on PATH")
	s.ghNoGhLogged = true
}

// logGhErrOnce logs a gh error for the feature once, and again only once its text changes,
// so a cause such as "no PR for the branch" does not fill the log on every poll. Guarded by
// ghMu for the same reason as logGhNoPathOnce.
func (s *server) logGhErrOnce(featureID int64, err error) {
	text := err.Error()
	s.ghMu.Lock()
	defer s.ghMu.Unlock()
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

// noteFinishedChecks posts a reef note for each check run that finished and was not yet
// noted, once per run: the same check name run again, such as a rerun on a later push, is its
// own run, noted on its own. A run that finished failed also wakes the developer; noting and
// waking are independent, so a wake that could not run (the channel is busy or paused) is
// retried on a later poll even though the run was already noted.
func (s *server) noteFinishedChecks(ctx context.Context, f store.Feature, pr ghPR, ghPath, dir string) error {
	for _, c := range pr.StatusCheckRollup {
		finished, word := c.outcome()
		if !finished {
			continue
		}
		name, key := c.checkName(), c.runKey()
		alreadyNoted, err := s.st.NoteGitHubCheck(ctx, f.ID, name, key)
		if err != nil {
			return err
		}
		if !alreadyNoted {
			if err := s.noteFeature(ctx, f.ID, fmt.Sprintf("%s %s", name, word)); err != nil {
				return err
			}
		}
		if word != "failed" {
			continue
		}
		if err := s.wakeDeveloper(ctx, f, pr, c, key, ghPath, dir); err != nil {
			return err
		}
	}
	return nil
}

// noteFeature posts a reef note in the feature's most recent channel, and tells an open
// stream on it that a message arrived, so the window refreshes without a reload.
func (s *server) noteFeature(ctx context.Context, featureID int64, body string) error {
	ch, err := s.st.FeatureLatestChannel(ctx, featureID)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err = s.st.CreateMessage(ctx, store.Message{
		ChannelID: ch.ID, Author: store.ReefAuthor, Body: body,
	}); err != nil {
		return err
	}
	s.notifyMessage(ch.ID)
	return nil
}

// wakeDeveloper wakes the developer role in the feature's most recent channel when a check
// run finishes failed, once per run, also across restarts. A channel whose team has no
// developer gets the note only. A channel with a turn running, or paused, cannot take the
// wake now; that is logged once and retried on a later poll, once the channel is idle. The
// wake runs as a new chain of its own, the way a retried turn does (decision 39): no gate
// applies, since no role sent it, but the channel's caps still do.
func (s *server) wakeDeveloper(
	ctx context.Context, f store.Feature, pr ghPR, c ghCheck, key, ghPath, dir string,
) error {
	name := c.checkName()
	woken, err := s.st.WokenGitHubCheck(ctx, f.ID, name, key)
	if err != nil || woken {
		return err
	}
	ch, err := s.st.FeatureLatestChannel(ctx, f.ID)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	team, err := s.st.Team(ctx, ch.Team)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(team.Roles, func(r store.Role) bool { return r.Name == "developer" }) {
		return nil
	}
	chain, ch, err := s.claim(ctx, ch.ID)
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			s.logGhErrOnce(f.ID, fmt.Errorf("wake developer: %w", err))
			return nil
		}
		return err
	}
	text := s.ghWakeText(ctx, pr, c, ghPath, dir)
	w := wake{from: _userAuthor, role: "developer", text: text}
	if err := chain.resume(ctx, ch, []wake{w}); err != nil {
		if errors.Is(err, store.ErrConflict) {
			s.logGhErrOnce(f.ID, fmt.Errorf("wake developer: %w", err))
			return nil
		}
		return err
	}
	return s.st.MarkGitHubCheckWoken(ctx, f.ID, name, key)
}

// ghWakeText is the wake text for a failed check run: the check name and the PR, plus the
// last 50 lines of the run's failed log when the run id can be read from the check.
func (s *server) ghWakeText(ctx context.Context, pr ghPR, c ghCheck, ghPath, dir string) string {
	text := fmt.Sprintf("%s failed on PR #%d: %s", c.checkName(), pr.Number, pr.URL)
	runID := c.runID()
	if runID == "" {
		return text
	}
	tail, err := ghLogTail(ctx, ghPath, dir, runID)
	if err != nil {
		log.Printf("github poll: run %s log: %v", runID, err)
		return text
	}
	return text + "\n\n" + tail
}

// ghLogTail is the last n lines of `gh run view id --log-failed`, run in dir.
func ghLogTail(ctx context.Context, ghPath, dir, runID string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, _ghTimeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, ghPath, "run", "view", runID, "--log-failed")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("gh run view: %w", err)
	}
	return lastLines(string(out), 50), nil
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
