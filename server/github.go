package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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

// ghPR is the gh pr view output this poll reads.
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

// prView is a feature's PR as the window shows it. Each check's state is pass, fail or
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

// featurePR finds the feature in the request and the project folder to run gh in. It writes
// the error and reports ok = false when it cannot.
func (s *server) featurePR(w http.ResponseWriter, r *http.Request) (
	f store.Feature, p store.Project, ghPath string, ok bool,
) {
	id, err := pathID(r)
	if err == nil {
		f, err = s.st.Feature(r.Context(), id)
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

// getFeaturePR answers with the feature's PR, or {"pr": null} when it has none.
func (s *server) getFeaturePR(w http.ResponseWriter, r *http.Request) {
	f, p, ghPath, ok := s.featurePR(w, r)
	if !ok {
		return
	}
	pr, err := runGh(r.Context(), ghPath, p.Path, f.Slug)
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

// mergeFeaturePR merges the feature's PR with a merge commit and answers with its new state.
// gh's own message is the error when it refuses.
func (s *server) mergeFeaturePR(w http.ResponseWriter, r *http.Request) {
	f, p, ghPath, ok := s.featurePR(w, r)
	if !ok {
		return
	}
	pr, err := runGh(r.Context(), ghPath, p.Path, f.Slug)
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
	if pr, err = runGh(r.Context(), ghPath, p.Path, f.Slug); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	view := newPRView(pr)
	writeJSON(w, http.StatusOK, map[string]*prView{"pr": &view})
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
	posted, err := s.noteFeature(ctx, f.ID, body)
	if err != nil || !posted {
		return err
	}
	return s.st.SetFeatureGitHub(ctx, store.FeatureGitHub{
		FeatureID: f.ID, PRNumber: int64(pr.Number), PRState: pr.State, PRURL: pr.URL,
	})
}

// noteFinishedChecks posts a reef note for each check run that finished and was not yet
// noted, once per run: the same check name run again, such as a rerun on a later push, is its
// own run, noted on its own. A check run is recorded as noted only once its note was actually
// posted, so a feature with no channel yet, or a failed CreateMessage, is retried on a later
// poll instead of losing the note. Every failed run in the rollup wakes the developer in one
// wake; noting and waking are independent, so a wake that could not run (the channel is busy
// or paused) is retried on a later poll even though the run was already noted.
func (s *server) noteFinishedChecks(ctx context.Context, f store.Feature, pr ghPR, ghPath, dir string) error {
	var failed []ghCheck
	for _, c := range pr.StatusCheckRollup {
		finished, word := c.outcome()
		if !finished {
			continue
		}
		name, key := c.checkName(), c.runKey()
		noted, err := s.st.GitHubCheckNoted(ctx, f.ID, name, key)
		if err != nil {
			return err
		}
		if !noted {
			posted, err := s.noteFeature(ctx, f.ID, fmt.Sprintf("%s %s", name, word))
			if err != nil {
				return err
			}
			if posted {
				if err := s.st.RecordGitHubCheck(ctx, f.ID, name, key); err != nil {
					return err
				}
			}
		}
		if word == "failed" {
			failed = append(failed, c)
		}
	}
	if len(failed) == 0 {
		return nil
	}
	return s.wakeDeveloper(ctx, f, pr, failed, ghPath, dir)
}

// noteFeature posts a reef note in the feature's most recent channel, and tells an open
// stream on it that a message arrived, so the window refreshes without a reload. It reports
// posted = false, without error, when the feature has no channel yet, so the caller does not
// record the note as sent.
func (s *server) noteFeature(ctx context.Context, featureID int64, body string) (posted bool, err error) {
	ch, err := s.st.FeatureLatestChannel(ctx, featureID)
	if errors.Is(err, store.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if _, err = s.st.CreateMessage(ctx, store.Message{
		ChannelID: ch.ID, Author: store.ReefAuthor, Body: body,
	}); err != nil {
		return false, err
	}
	s.notifyMessage(ch.ID)
	return true, nil
}

// wakeDeveloper wakes the developer role in the feature's most recent channel with one wake
// naming every run in checks not already woken, each with its log tail, once per run, also
// across restarts. Several failed runs in the same rollup are one wake, not one each. A
// channel whose team has no developer gets the note only. The wake text is built, including
// any log fetch, before the channel is claimed, so a user send during that fetch is not held
// to a 409. A channel with a turn running, or paused, cannot take the wake now; that is logged
// once and retried on a later poll, once the channel is idle. The wake runs as a new chain of
// its own, the way a retried turn does (decision 39), attributed to store.ReefAuthor rather
// than the user: no gate applies, since no role sent it, but it counts as a hop like any
// agent-originated wake, so a loop of failing checks still pauses at the hop cap instead of
// resetting it every time.
func (s *server) wakeDeveloper(
	ctx context.Context, f store.Feature, pr ghPR, checks []ghCheck, ghPath, dir string,
) error {
	var unwoken []ghCheck
	for _, c := range checks {
		woken, err := s.st.WokenGitHubCheck(ctx, f.ID, c.checkName(), c.runKey())
		if err != nil {
			return err
		}
		if !woken {
			unwoken = append(unwoken, c)
		}
	}
	if len(unwoken) == 0 {
		return nil
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
	text := s.ghWakeText(ctx, pr, unwoken, ghPath, dir)
	chain, ch, err := s.claim(ctx, ch.ID)
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			s.logGhErrOnce(f.ID, fmt.Errorf("wake developer: %w", err))
			return nil
		}
		return err
	}
	w := wake{from: store.ReefAuthor, role: "developer", text: text}
	if err := chain.resume(ctx, ch, []wake{w}); err != nil {
		if errors.Is(err, store.ErrConflict) {
			s.logGhErrOnce(f.ID, fmt.Errorf("wake developer: %w", err))
			return nil
		}
		return err
	}
	for _, c := range unwoken {
		if err := s.st.MarkGitHubCheckWoken(ctx, f.ID, c.checkName(), c.runKey()); err != nil {
			return err
		}
	}
	return nil
}

// ghWakeText is the wake text for one or more failed check runs, joined with a blank line:
// each run's check name and the PR, plus the last 50 lines of its failed log when the run id
// can be read from the check.
func (s *server) ghWakeText(ctx context.Context, pr ghPR, checks []ghCheck, ghPath, dir string) string {
	parts := make([]string, len(checks))
	for i, c := range checks {
		parts[i] = ghCheckWakeText(ctx, pr, c, ghPath, dir)
	}
	return strings.Join(parts, "\n\n")
}

// ghCheckWakeText is the wake text for one failed check run: the check name and the PR, plus
// the last 50 lines of the run's failed log when the run id can be read from the check.
func ghCheckWakeText(ctx context.Context, pr ghPR, c ghCheck, ghPath, dir string) string {
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
