package channel

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/agent/claude"
	"github.com/bhanuprakaash/reef/agent/codex"
	"github.com/bhanuprakaash/reef/skill"
	"github.com/bhanuprakaash/reef/store"
	"github.com/bhanuprakaash/reef/worktree"
)

// Session is a channel with one agent session per teammate in use. Each is opened for its
// first turn. The lead's stays open until Close; a specialist's is closed after its turn.
type Session struct {
	st       *store.Store
	dataDir  string
	exe      string
	ch       store.Channel
	project  store.Project
	mk       func(backend string) (agent.Agent, error)
	lock     *os.File
	onPrompt func(prompt string)
	members  map[string]*member
	trees    *Trees
}

type member struct {
	run   *Runner
	ag    agent.Agent
	token string
	// write and dir say which tree the role's turns hold, one at a time.
	write bool
	dir   string
}

// TokenEnv carries the token to the channel's reef mcp. WrapCommand is the text that asks
// for a wrap-up; PromoteCommand is the stored message of a promote turn.
const (
	TokenEnv       = "REEF_TOKEN"
	WrapCommand    = "/wrap"
	PromoteCommand = "/promote"

	_wrapPrompt = "Wrap up this channel. Call the get_brief tool first. Then call the " +
		"propose_wrap tool once with the full brief rewritten as the latest brief plus what " +
		"this channel achieved (goal, status, key decisions, open questions): keep what still " +
		"holds, make status where the feature is now, not a history, drop what is done or no " +
		"longer true, and keep it short. Add your own notes on this feature, and the decisions " +
		"made in this channel that are not recorded yet. Review the decision index: pass " +
		"retire with the ids of decisions that are stale or duplicate, and replaces on a " +
		"new decision that amends older ones. " +
		"Then reply with one line saying it is proposed."
	_promotePrompt = "This quick session is now feature %q (%s). Call the propose_wrap tool " +
		"once with the first brief (goal, status, key decisions, open questions) written from " +
		"this session, your own notes on the feature, and the decisions made so far. " +
		"Then reply with one line saying it is proposed."
	_promoteIntoBriefPrompt = "This quick session is now part of feature %q (%s), which " +
		"already has a brief. Call the get_brief tool first. Then call the propose_wrap tool " +
		"once with the full brief rewritten to merge this session into the existing brief: " +
		"keep its content and add this session's, your notes on the feature merged the same " +
		"way, and the decisions made so far. Then reply with one line saying it is proposed."
)

// ErrNoProposal is returned by Wrap when the agent made no proposal.
var ErrNoProposal = errors.New("no proposal made")

// Open opens the channel; no agent starts until a turn needs it. The caller must Close
// the Session. Sessions that share trees run one write turn at a time on a working tree.
func Open(
	ctx context.Context, st *store.Store, dataDir, exe string, channelID int64, trees *Trees,
) (*Session, error) {
	return open(ctx, st, dataDir, exe, channelID, trees, newAgent)
}

func open(
	ctx context.Context, st *store.Store, dataDir, exe string, channelID int64, trees *Trees,
	mk func(backend string) (agent.Agent, error),
) (_ *Session, err error) {
	ch, err := st.Channel(ctx, channelID)
	if err != nil {
		return nil, err
	}
	p, err := st.Project(ctx, ch.ProjectID)
	if err != nil {
		return nil, err
	}
	lock, err := lockChat(dataDir, channelID)
	if err != nil {
		return nil, err
	}
	s := &Session{
		st: st, dataDir: dataDir, exe: exe, ch: ch, project: p, mk: mk, lock: lock,
		members: map[string]*member{}, trees: trees,
	}
	defer func() {
		if err != nil {
			_ = s.Close()
		}
	}()

	if err := failStaleTurns(ctx, st, channelID); err != nil {
		return nil, err
	}
	return s, nil
}

// Start starts or resumes the lead now instead of at its first turn.
func (s *Session) Start(ctx context.Context) error {
	_, err := s.member(ctx, s.ch.Role, "", nil)
	return err
}

// SetOnPrompt sets a hook that sees every prompt before it is sent to an agent.
func (s *Session) SetOnPrompt(f func(prompt string)) {
	s.onPrompt = f
	for _, m := range s.members {
		m.run.OnPrompt = f
	}
}

// Say stores body as the user message, a reply in the thread of parent when it is set, and
// returns it.
func (s *Session) Say(ctx context.Context, body string, parent *int64) (store.Message, error) {
	return s.st.CreateMessage(ctx, store.Message{
		ChannelID: s.ch.ID, ParentID: parent, Author: _userAuthor, Body: body,
	})
}

// Run runs one turn of the lead with text. A cancelled ctx wraps ctx.Err(). The reply is a
// reply in the thread of parent when it is set.
func (s *Session) Run(
	ctx context.Context, text string, parent *int64, onEvent func(agent.Event),
) error {
	return s.RunAs(ctx, s.ch.Role, text, parent, onEvent)
}

// RunAs is Run for any role of the channel's team. A specialist's agent is closed when
// its turn ends; its next turn resumes the session. A failure to close it is logged, and
// does not fail the turn. A role that writes waits for the turns of other writers on its
// tree, and its turn starts when they end.
func (s *Session) RunAs(
	ctx context.Context, role, text string, parent *int64, onEvent func(agent.Event),
) error {
	m, err := s.member(ctx, role, text, parent)
	if err != nil {
		return err
	}
	err = s.run(ctx, m, text, parent, onEvent)
	if role == s.ch.Role {
		return err
	}
	delete(s.members, role)
	if cerr := m.close(s.st); cerr != nil {
		log.Printf("close %s of channel %d: %v", role, s.ch.ID, cerr)
	}
	return err
}

// run runs a turn of m, holding m's tree when it writes and counting as a reader of it when
// it does not. A ctx cancelled while it waits leaves an interrupted turn.
func (s *Session) run(
	ctx context.Context, m *member, text string, parent *int64, onEvent func(agent.Event),
) error {
	if !m.write {
		defer s.trees.Read(m.dir)()
		return m.run.Run(ctx, text, parent, onEvent)
	}
	unlock, err := s.trees.Lock(ctx, m.dir)
	if err != nil {
		reason := err.Error()
		if ctx.Err() != nil {
			reason = "interrupted"
		}
		ferr := s.failTurn(ctx, m.run.role, m.run.backend, reason, m.run.wake(text), parent)
		return errors.Join(err, ferr)
	}
	defer unlock()
	return m.run.Run(ctx, text, parent, onEvent)
}

// failTurn stores a failed turn of role for a turn that never started.
func (s *Session) failTurn(
	ctx context.Context, role, backend, reason, text string, parent *int64,
) error {
	ctx = context.WithoutCancel(ctx)
	t, err := s.st.CreateTurn(ctx, store.Turn{
		ChannelID: s.ch.ID, Role: role, Backend: backend, WakeText: text, WakeParent: parent,
	})
	if err != nil {
		return err
	}
	t.Status, t.Error = "failed", reason
	_, err = s.st.EndTurn(ctx, t)
	return err
}

// Wrap runs a wrap-up turn and returns the proposal the agent made, or ErrNoProposal. Like
// Run, a cancelled ctx still stores the turn.
func (s *Session) Wrap(ctx context.Context, onEvent func(agent.Event)) (store.Proposal, error) {
	return s.propose(ctx, _wrapPrompt, onEvent)
}

// Promote runs a promote turn and returns the proposal the agent made, or ErrNoProposal.
// The channel must already have its feature. Like Run, a cancelled ctx still stores the turn.
// Every agent is stopped afterwards, so the next turn of each role runs in the feature's
// worktree.
func (s *Session) Promote(ctx context.Context, onEvent func(agent.Event)) (store.Proposal, error) {
	rctx := context.WithoutCancel(ctx)
	ch, err := s.st.Channel(rctx, s.ch.ID)
	if err != nil {
		return store.Proposal{}, err
	}
	if ch.FeatureID == nil {
		return store.Proposal{}, fmt.Errorf("%w: channel %d has no feature", store.ErrInvalid, ch.ID)
	}
	f, err := s.st.Feature(rctx, *ch.FeatureID)
	if err != nil {
		return store.Proposal{}, err
	}
	prompt := _promotePrompt
	if _, err := s.st.Brief(rctx, f.ID); err == nil {
		prompt = _promoteIntoBriefPrompt
	} else if !errors.Is(err, store.ErrNotFound) {
		return store.Proposal{}, err
	}
	if err := s.noteWorktree(rctx, f); err != nil {
		return store.Proposal{}, err
	}
	p, err := s.propose(ctx, fmt.Sprintf(prompt, f.Name, f.Slug), onEvent)
	return p, errors.Join(err, s.closeMembers())
}

// noteWorktree tells the channel where its work goes from now on, and that uncommitted
// changes of the main checkout stay there.
func (s *Session) noteWorktree(ctx context.Context, f store.Feature) error {
	dirty, err := worktree.Dirty(ctx, s.project.Path)
	if err != nil {
		log.Printf("check %s for changes: %v", s.project.Path, err)
	}
	body := fmt.Sprintf("This feature works in %s on %s.",
		worktree.Path(s.dataDir, s.project.ID, f.Slug), worktree.Branch(f.Slug))
	if dirty {
		body += " Uncommitted changes stay on the main checkout."
	}
	_, err = s.st.CreateMessage(ctx,
		store.Message{ChannelID: s.ch.ID, Author: store.ReefAuthor, Body: body})
	return err
}

// Close ends the Session.
func (s *Session) Close() error {
	return errors.Join(s.closeMembers(), s.lock.Close())
}

// closeMembers ends every open agent, so the next turn of each role starts one again.
func (s *Session) closeMembers() error {
	var errs []error
	for role, m := range s.members {
		errs = append(errs, m.close(s.st))
		delete(s.members, role)
	}
	return errors.Join(errs...)
}

// member returns the agent session of role, starting or resuming it from the role's own
// newest turn with a session id on the role's backend. A new session reads the team from the
// store, so a team applied since the channel opened is the one it gets. An agent that fails
// to start leaves a failed turn for the role that keeps text and parent, the wake it was
// starting for, and so does a ctx cancelled before it starts: the turn is interrupted. The
// store is read even then, as Run stores its turn.
func (s *Session) member(
	ctx context.Context, role, text string, parent *int64,
) (_ *member, err error) {
	wctx := context.WithoutCancel(ctx)
	team, err := s.st.Team(wctx, s.ch.Team)
	if err != nil {
		return nil, err
	}
	r, err := s.roleSetup(team, role)
	if err != nil {
		return nil, err
	}
	if m, ok := s.members[role]; ok {
		if _, err := os.Stat(m.dir); !errors.Is(err, os.ErrNotExist) {
			// Before its first turn, m's preamble already carries the current intro, set
			// when it was started or resumed; nothing has been sent yet to compare against.
			if intro := roleIntro(r); intro != "" && m.run.sessionID != "" {
				sent, err := introSent(wctx, s.st, s.ch.ID, role, r.Backend, m.run.sessionID)
				if err != nil {
					return nil, err
				}
				if !strings.HasPrefix(sent, intro) {
					m.run.preamble = updatedRoleIntro(r)
				}
			}
			return m, nil
		}
		delete(s.members, role)
		if err := m.close(s.st); err != nil {
			log.Printf("close %s of channel %d: %v", role, s.ch.ID, err)
		}
	}
	ag, err := s.mk(r.Backend)
	if err != nil {
		return nil, err
	}
	m := &member{ag: ag, write: r.Write}
	defer func() {
		if err != nil {
			_ = m.close(s.st)
		}
	}()

	last, err := newestSessionTurn(wctx, s.st, s.ch.ID, role, r.Backend)
	if err != nil {
		return nil, err
	}
	tok, err := s.st.CreateToken(wctx, s.ch.ID, role)
	if err != nil {
		return nil, err
	}
	m.token = tok.Token
	servers := map[string]agent.MCPServer{"reef": {
		Command: s.exe,
		Args:    []string{"mcp", "--data", s.dataDir},
		Env:     map[string]string{TokenEnv: tok.Token},
	}}
	for i, srv := range r.MCPServers {
		servers["mcp"+strconv.Itoa(i+1)] = srv
	}
	cwd, slug, err := WorkDir(wctx, s.st, s.dataDir, s.ch.ID)
	if err != nil {
		return nil, err
	}
	m.dir = cwd
	mode, tools, denied := access(r, claudeSkills(r, cwd))
	cfg := agent.Config{
		Cwd:             cwd,
		PermissionMode:  mode,
		Model:           r.Model,
		Tools:           append(tools, "mcp__reef__*"),
		DisallowedTools: denied,
		MCPServers:      servers,
	}
	var preamble string
	if ctx.Err() == nil && slug != "" {
		var p store.Project
		if p, err = s.st.Project(wctx, s.project.ID); err == nil {
			err = EnsureWorktree(ctx, s.trees, p, cwd, slug)
		}
	}
	switch {
	case err != nil:
	case ctx.Err() != nil:
		err = ctx.Err()
	case last.SessionID != "":
		if intro := roleIntro(r); intro != "" {
			sent, serr := introSent(wctx, s.st, s.ch.ID, role, r.Backend, last.SessionID)
			if serr != nil {
				return nil, serr
			}
			if !strings.HasPrefix(sent, intro) {
				preamble = updatedRoleIntro(r)
			}
		}
		err = ag.Resume(ctx, cfg, last.SessionID)
	default:
		if preamble, err = startPrompt(wctx, s.st, s.ch.ID, s.project, team, r); err != nil {
			return nil, err
		}
		err = ag.Start(ctx, cfg)
	}
	if err != nil {
		reason := err.Error()
		if ctx.Err() != nil {
			reason = "interrupted"
		}
		return nil, errors.Join(err, s.failTurn(wctx, role, r.Backend, reason, text, parent))
	}
	m.run = NewRunner(s.st, ag, s.ch.ID, role, r.Backend, preamble)
	m.run.OnPrompt = s.onPrompt
	if r.Write && (slug != "" || worktree.IsRepo(wctx, cwd)) {
		m.run.CheckpointDir = cwd
	}
	s.members[role] = m
	return m, nil
}

// WorkDir is where an agent of channel channelID runs: the worktree of its feature, or the
// project for a quick session. The feature is read now, as a promote may have given it one
// since the channel opened. slug is the feature's, empty for a quick session.
func WorkDir(
	ctx context.Context, st *store.Store, dataDir string, channelID int64,
) (dir, slug string, err error) {
	ch, err := st.Channel(ctx, channelID)
	if err != nil {
		return "", "", err
	}
	p, err := st.Project(ctx, ch.ProjectID)
	if err != nil {
		return "", "", err
	}
	if ch.FeatureID == nil {
		return p.Path, "", nil
	}
	f, err := st.Feature(ctx, *ch.FeatureID)
	if err != nil {
		return "", "", err
	}
	return worktree.Path(dataDir, p.ID, f.Slug), f.Slug, nil
}

// roleSetup is the team's role called name. The lead is the channel's role; it takes the
// channel's own backend and permission mode.
func (s *Session) roleSetup(team store.Team, name string) (store.Role, error) {
	if name == s.ch.Role {
		lead := team.Roles[0]
		lead.Name, lead.Backend, lead.PermissionMode = name, s.ch.Backend, s.ch.PermissionMode
		return lead, nil
	}
	for _, r := range team.Roles {
		if r.Name == name {
			return r, nil
		}
	}
	return store.Role{}, fmt.Errorf("%w: team %s has no role %s", store.ErrInvalid, team.Name, name)
}

// _editTools change files on claude. A role without write never gets them, is denied them
// (a deny beats an allow in the user's or project's settings), and runs in the default
// permission mode: acceptEdits would let it edit and run file commands unasked.
var _editTools = []string{"Edit", "Write", "MultiEdit", "NotebookEdit"}

// claudeSkills names the skills claude finds in dir, for a claude role that picked some. A role
// with none keeps every skill, so there is nothing to scan.
func claudeSkills(r store.Role, dir string) []string {
	if r.Backend != "claude" || len(r.Skills) == 0 {
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		log.Printf("find home folder to list skills: %v", err)
		return nil
	}
	var names []string
	for _, s := range skill.Claude(home, dir) {
		names = append(names, s.Name)
	}
	return names
}

// access maps a role's write flag to the permission mode, allowed tools and denied tools of its backend.
// A write role on claude also gets Task to run parallel subagents. On codex a permission
// mode the role sets stays when the role writes, so it may narrow it. A claude role may use only
// the skills it picked, if it picked any: onDisk names the skills on disk, and each one not picked is denied.
func access(r store.Role, onDisk []string) (mode string, tools, denied []string) {
	mode, tools = r.PermissionMode, slices.Clone(r.Tools)
	switch {
	case r.Backend == "codex" && !r.Write:
		mode = "read-only"
	case r.Backend == "codex" && mode == "":
		mode = "workspace-write"
	case r.Backend == "claude" && r.Write:
		tools = append(tools, "Task")
	case r.Backend == "claude":
		mode = "default"
		denied = _editTools
		tools = slices.DeleteFunc(tools, func(t string) bool {
			name, _, _ := strings.Cut(t, "(")
			return slices.Contains(_editTools, name) || store.UnrestrictedBash(t)
		})
	}
	if r.Backend == "claude" && len(r.Skills) > 0 {
		for _, name := range r.Skills {
			tools = append(tools, "Skill("+name+")")
		}
		denied = slices.Clone(denied)
		for _, name := range onDisk {
			if !slices.Contains(r.Skills, name) {
				denied = append(denied, "Skill("+name+")")
			}
		}
	}
	return mode, tools, denied
}

func (s *Session) propose(
	ctx context.Context, prompt string, onEvent func(agent.Event),
) (store.Proposal, error) {
	before, err := s.st.PendingProposal(context.WithoutCancel(ctx), s.ch.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return store.Proposal{}, err
	}
	m, err := s.member(ctx, s.ch.Role, "", nil)
	if err != nil {
		return store.Proposal{}, err
	}
	m.run.NoWake = true
	defer func() { m.run.NoWake = false }()
	if err := s.run(ctx, m, prompt, nil, onEvent); err != nil {
		return store.Proposal{}, err
	}
	p, err := s.st.PendingProposal(ctx, s.ch.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return store.Proposal{}, err
	}
	if err != nil || p.ID == before.ID || p.Kind != store.ProposalWrap {
		return store.Proposal{}, ErrNoProposal
	}
	return p, nil
}

func (m *member) close(st *store.Store) error {
	errs := []error{m.ag.Interrupt()}
	if m.token != "" {
		errs = append(errs, st.DeleteToken(context.Background(), m.token))
	}
	return errors.Join(errs...)
}

// InUse reports whether a Session holds the channel's chat lock.
func InUse(dataDir string, channelID int64) bool {
	lock, err := os.Open(chatLockPath(dataDir, channelID))
	if err != nil {
		return false
	}
	defer func() { _ = lock.Close() }()
	return errors.Is(syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB), syscall.EWOULDBLOCK)
}

func startPrompt(
	ctx context.Context, st *store.Store, channelID int64, p store.Project, team store.Team,
	r store.Role,
) (string, error) {
	ch, err := st.Channel(ctx, channelID)
	if err != nil {
		return "", err
	}
	if ch.FeatureID == nil {
		mem, err := st.ProjectMemory(ctx, p.ID)
		if err != nil {
			return "", err
		}
		who, err := profileLine(ctx, st)
		if err != nil {
			return "", err
		}
		return roleIntro(r) + projectContext(p, r.Name, mem.Body, who) + teamIntro(team, r), nil
	}
	body, err := Context(ctx, st, *ch.FeatureID, r.Name)
	return roleIntro(r) + body + teamIntro(team, r), err
}

func failStaleTurns(ctx context.Context, st *store.Store, channelID int64) error {
	turns, err := st.Turns(ctx, store.TurnFilter{ChannelID: &channelID, Status: "running"})
	if err != nil {
		return err
	}
	for _, t := range turns {
		t.Status, t.Error = "failed", "chat exited"
		if _, err := st.EndTurn(ctx, t); err != nil {
			return err
		}
	}
	return nil
}

func newestSessionTurn(
	ctx context.Context, st *store.Store, channelID int64, role, backend string,
) (store.Turn, error) {
	turns, err := st.Turns(ctx, store.TurnFilter{ChannelID: &channelID})
	if err != nil {
		return store.Turn{}, err
	}
	var last store.Turn
	for _, t := range turns {
		if t.Role == role && t.Backend == backend && t.SessionID != "" {
			last = t
		}
	}
	return last, nil
}

// introSent is the role intro its session (same session id) in the channel was last given:
// the session's first turn always carries one, from startPrompt, and a later turn carries one
// when the role's persona or skills changed since, from updatedRoleIntro. Empty when neither
// applies, so a prefix check against it never matches a non-empty roleIntro.
func introSent(
	ctx context.Context, st *store.Store, channelID int64, role, backend, sessionID string,
) (string, error) {
	turns, err := st.Turns(ctx, store.TurnFilter{ChannelID: &channelID})
	if err != nil {
		return "", err
	}
	var sent string
	first := true
	for _, t := range turns {
		if t.Role != role || t.Backend != backend || t.SessionID != sessionID {
			continue
		}
		if first {
			sent, first = t.Prompt, false
			continue
		}
		if after, ok := strings.CutPrefix(t.Prompt, _introChangedPrefix); ok {
			sent = after
		}
	}
	return sent, nil
}

func newAgent(backend string) (agent.Agent, error) {
	switch backend {
	case "claude":
		return claude.New(), nil
	case "codex":
		return codex.New(), nil
	default:
		return nil, fmt.Errorf("%w: unknown backend %q", store.ErrInvalid, backend)
	}
}

func chatLockPath(dataDir string, channelID int64) string {
	return filepath.Join(dataDir, fmt.Sprintf("chat-%d.lock", channelID))
}

func lockChat(dataDir string, channelID int64) (*os.File, error) {
	path := chatLockPath(dataDir, channelID)
	lock, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = lock.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf("channel %d is open elsewhere: %w", channelID, store.ErrConflict)
		}
		return nil, fmt.Errorf("lock %s: %w", path, err)
	}
	return lock, nil
}
