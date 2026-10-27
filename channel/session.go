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
	"github.com/bhanuprakaash/reef/memory"
	"github.com/bhanuprakaash/reef/skill"
	"github.com/bhanuprakaash/reef/store"
	"github.com/bhanuprakaash/reef/worktree"
)

// Session is a channel with one agent session per teammate in use. Each is opened for its
// first turn. The lead's stays open until Close; a specialist's is closed after its turn.
type Session struct {
	st       *store.Store
	ms       *memory.Store
	dataDir  string
	exe      string
	ch       store.Channel
	project  store.Project
	mk       func(backend string) (agent.Agent, error)
	lock     *os.File
	onPrompt func(prompt string)
	onNote   func()
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
	cfg   agent.Config
}

type runInput struct {
	text   string
	parent *int64
	direct *store.DirectMessage
	// scopes, when set, say the wake was routed: text is what it was routed with, which a turn
	// that never starts keeps, and the turn's own text is built from the unread messages of
	// scopes when it starts.
	scopes  store.Scopes
	onEvent func(agent.Event)
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
		"longer true, and keep it short. Add as items what this channel made that you have not " +
		"saved with remember yet: decisions, findings, facts, lessons and conventions, each " +
		"with its scope (work_item by default; project for what holds across the project; " +
		"teammate for what only you should recall). Skip what you already saved with " +
		"remember. If you are the team lead, you may add team lessons about how this team " +
		"works together; the user approves those. Use recall to find stale or duplicate " +
		"items: pass retire with their ids, and replaces on a new item that amends older " +
		"ones. Then reply with one line saying it is proposed."
	_maxDirectLines = 100
	_maxDirectRunes = 1000

	_directIntro = "Direct exchanges in this channel, private between two teammates:\n"
	_directAsk   = "Propose as items what these exchanges decided or found that is not saved yet, " +
		"and a brief that reflects the direct work that is complete.\n\n"

	_promotePrompt = "This quick session is now feature %q (%s). Call the propose_wrap tool " +
		"once with the first brief (goal, status, key decisions, open questions) written from " +
		"this session, and the items made so far (decisions, findings, facts, lessons, " +
		"conventions), each with its scope. " +
		"Then reply with one line saying it is proposed."
	_promoteIntoBriefPrompt = "This quick session is now part of feature %q (%s), which " +
		"already has a brief. Call the get_brief tool first. Then call the propose_wrap tool " +
		"once with the full brief rewritten to merge this session into the existing brief: " +
		"keep its content and add this session's, and the items made so far. Then reply with " +
		"one line saying it is proposed."
)

// ErrNoProposal is returned by Wrap when the agent made no proposal.
var ErrNoProposal = errors.New("no proposal made")

// Open opens the channel; no agent starts until a turn needs it. mk makes the agent of each
// backend, NewAgent in production. The caller must Close the Session. Sessions that share
// trees run one write turn at a time on a working tree.
func Open(
	ctx context.Context, st *store.Store, ms *memory.Store, dataDir, exe string, channelID int64,
	trees *Trees, mk func(backend string) (agent.Agent, error),
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
		st: st, ms: ms, dataDir: dataDir, exe: exe, ch: ch, project: p, mk: mk, lock: lock,
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
	_, err := s.member(ctx, s.ch.Role, runInput{})
	return err
}

// SetOnNote sets a hook called after the lead posts a background answer between turns. Set it
// before the first turn.
func (s *Session) SetOnNote(f func()) {
	s.onNote = f
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
	return s.runAs(ctx, role, runInput{text: text, parent: parent, onEvent: onEvent})
}

// WakeAs is RunAs for a wake routed with routed, in scopes. The turn's text is not routed: it
// is built when the turn starts, from the messages of scopes that the role has not been sent
// in its session. A wake with none runs no turn. A wake that never starts keeps routed.
func (s *Session) WakeAs(
	ctx context.Context, role, routed string, scopes store.Scopes, parent *int64,
	onEvent func(agent.Event),
) error {
	if s.allRead(ctx, role, scopes) {
		return nil
	}
	return s.runAs(ctx, role,
		runInput{text: routed, parent: parent, scopes: scopes, onEvent: onEvent})
}

// allRead reports whether role's session has been sent every message of scopes, so no agent
// need start. It is false when it cannot tell: the turn then starts and finds out.
func (s *Session) allRead(ctx context.Context, role string, scopes store.Scopes) bool {
	ctx = context.WithoutCancel(ctx)
	sessionID := ""
	if m, ok := s.members[role]; ok {
		if m.run.sessionGone {
			return false
		}
		sessionID = m.run.sessionID
	} else {
		team, err := s.st.Team(ctx, s.ch.Team)
		if err != nil {
			return false
		}
		r, err := s.roleSetup(team, role)
		if err != nil {
			return false
		}
		last, err := newestSessionTurn(ctx, s.st, s.ch.ID, role, r.Backend)
		if err != nil {
			return false
		}
		sessionID = last.SessionID
	}
	text, _, err := wakeText(ctx, s.st, s.ch.ID, role, sessionID, scopes)
	return err == nil && text == ""
}

func (s *Session) runAs(ctx context.Context, role string, in runInput) error {
	m, err := s.member(ctx, role, in)
	if err != nil {
		return err
	}
	err = s.run(ctx, m, in)
	if role == s.ch.Role {
		return err
	}
	delete(s.members, role)
	if cerr := m.close(s.st); cerr != nil {
		log.Printf("close %s of channel %d: %v", role, s.ch.ID, cerr)
	}
	return err
}

// RunDirectAs runs a role for a direct message, keeping the request/response boundary in
// Runner while applying the same role setup, worktree access and session lifetime as RunAs.
func (s *Session) RunDirectAs(
	ctx context.Context, role, text string, dm store.DirectMessage, onEvent func(agent.Event),
) error {
	m, err := s.member(ctx, role, runInput{text: text, direct: &dm})
	if err != nil {
		if _, rerr := s.st.CreateFailedDirectResponse(ctx, dm.ID, role, ""); rerr != nil &&
			!errors.Is(rerr, store.ErrConflict) {
			log.Printf("direct response for %d: %v", dm.ID, rerr)
		}
		return err
	}
	err = s.run(ctx, m, runInput{text: text, direct: &dm, onEvent: onEvent})
	if role != s.ch.Role {
		delete(s.members, role)
		if closeErr := m.close(s.st); closeErr != nil {
			log.Printf("close %s of channel %d: %v", role, s.ch.ID, closeErr)
		}
	}
	return err
}

// run runs a turn of m, holding m's tree when it writes and counting as a reader of it when
// it does not. A ctx cancelled while it waits leaves an interrupted turn.
func (s *Session) run(ctx context.Context, m *member, in runInput) error {
	if m.run.role == s.ch.Role {
		defer m.run.Watch()
	}
	if !m.write {
		defer s.trees.Read(m.dir)()
		return s.turn(ctx, m, in)
	}
	unlock, err := s.trees.Lock(ctx, m.dir)
	if err != nil {
		in.text = m.run.wake(in.text)
		err = s.failTurn(ctx, m.run.role, m.run.backend, in, err)
		if in.direct != nil {
			if _, rerr := s.st.CreateFailedDirectResponse(
				context.WithoutCancel(ctx), in.direct.ID, m.run.role, "",
			); rerr != nil && !errors.Is(rerr, store.ErrConflict) {
				err = errors.Join(err, rerr)
			}
		}
		return err
	}
	defer unlock()
	return s.turn(ctx, m, in)
}

// turn runs the turn of m. The text of a wake routed in scopes is built now, from what m's
// session has not been sent; a wake with none runs no turn. A failure to build it leaves a
// failed turn, as the agent got nothing.
func (s *Session) turn(ctx context.Context, m *member, in runInput) error {
	text, sent, parent := in.text, store.Scopes(nil), in.parent
	if in.direct == nil && len(in.scopes) > 0 {
		var err error
		text, sent, err = wakeText(
			context.WithoutCancel(ctx), s.st, s.ch.ID, m.run.role, m.run.sessionID, in.scopes)
		if err != nil {
			return s.failTurn(ctx, m.run.role, m.run.backend, in, err)
		}
		if text == "" {
			return nil
		}
		parent = nil
		if thread := sent[0].Thread; thread != 0 {
			parent = &thread
		}
	}
	m.run.Scopes = sent
	if in.direct != nil {
		return m.run.RunDirect(ctx, text, *in.direct, in.onEvent)
	}
	return m.run.Run(ctx, text, parent, in.onEvent)
}

// failTurn stores a failed turn of role for a turn that never started and returns cause
// joined with the error of storing it. The turn keeps the wake it was for, so retry can run
// it again; a ctx cancelled by then makes the turn interrupted.
func (s *Session) failTurn(
	ctx context.Context, role, backend string, in runInput, cause error,
) error {
	reason := cause.Error()
	if ctx.Err() != nil {
		reason = "interrupted"
	}
	ctx = context.WithoutCancel(ctx)
	t, err := s.st.CreateTurn(ctx, store.Turn{
		ChannelID: s.ch.ID, Role: role, Backend: backend, WakeText: in.text, WakeParent: in.parent,
		DirectMessageID: in.direct.IDRef(), Scopes: in.scopes,
	})
	if err != nil {
		log.Printf("store failed start of %s in channel %d: %v", role, s.ch.ID, err)
		return cause
	}
	t.Status, t.Error = "failed", reason
	_, err = s.st.EndTurn(ctx, t)
	return errors.Join(cause, err)
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
// store, so a team applied since the channel opened is the one it gets, and its first prompt
// reads the team and the role again when it is built. An agent that fails to start, for any
// reason, leaves a failed turn for the role that keeps the wake it was starting for, and so
// does a ctx cancelled before it starts: the turn is interrupted. The store is read even
// then, as Run stores its turn. A role the team no longer has leaves one too, on the
// channel's backend.
func (s *Session) member(ctx context.Context, role string, in runInput) (_ *member, err error) {
	wctx := context.WithoutCancel(ctx)
	team, err := s.st.Team(wctx, s.ch.Team)
	if err != nil {
		return nil, err
	}
	backend := s.ch.Backend
	defer func() {
		if err != nil {
			err = s.failTurn(ctx, role, backend, in, err)
		}
	}()
	r, err := s.roleSetup(team, role)
	if err != nil {
		return nil, err
	}
	backend = r.Backend
	if m, ok := s.members[role]; ok {
		m.run.endWatch()
		_, err := os.Stat(m.dir)
		if !errors.Is(err, os.ErrNotExist) && !m.run.sessionGone &&
			sameAccess(m.cfg, roleConfig(r, m.dir)) {
			// Before its first turn m has no session id and nothing sent to compare against: its
			// first prompt is built with the role as it is when the turn runs.
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
	m := &member{ag: ag, write: writes(r)}
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
	cfg := roleConfig(r, cwd)
	cfg.MCPServers = servers
	m.cfg = cfg
	var preamble string
	cfg.Bin, err = s.st.RoleBin(wctx, r)
	if err == nil && ctx.Err() == nil && slug != "" {
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
		err = ag.Start(ctx, cfg)
	}
	if err != nil {
		return nil, err
	}
	m.run = NewRunner(s.st, ag, s.ch.ID, role, r.Backend, preamble)
	m.run.sessionID = last.SessionID
	if last.SessionID != "" {
		compacted, err := s.st.SessionCompacted(wctx, s.ch.ID, role, last.SessionID)
		if err != nil {
			return nil, err
		}
		m.run.firstPromptSent = !compacted
	}
	m.run.FirstPrompt = func(ctx context.Context) (string, error) {
		ch, err := s.st.Channel(ctx, s.ch.ID)
		if err != nil {
			return "", err
		}
		team, err := s.st.Team(ctx, s.ch.Team)
		if err != nil {
			return "", err
		}
		r, err := s.roleSetup(team, role)
		if err != nil {
			return "", err
		}
		return firstPrompt(ctx, s.st, s.ms, ch.ProjectID, ch.FeatureID, ch.ID, team, r)
	}
	m.run.OnPrompt = s.onPrompt
	m.run.Memory = s.ms
	if m.write && (slug != "" || worktree.IsRepo(wctx, cwd)) {
		m.run.CheckpointDir = cwd
		m.run.DataDir = s.dataDir
	}
	m.run.OnNote = s.onNote
	s.members[role] = m
	return m, nil
}

func roleConfig(r store.Role, cwd string) agent.Config {
	mode, tools, denied := access(r, claudeSkills(r, cwd))
	return agent.Config{
		Cwd:             cwd,
		PermissionMode:  mode,
		Model:           r.Model,
		Tools:           append(tools, "mcp__reef__*"),
		DisallowedTools: denied,
	}
}

func sameAccess(a, b agent.Config) bool {
	return a.PermissionMode == b.PermissionMode && a.Model == b.Model &&
		slices.Equal(a.Tools, b.Tools) && slices.Equal(a.DisallowedTools, b.DisallowedTools)
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
		if lead.Backend != s.ch.Backend {
			lead.Provider = ""
		}
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

// git log, diff, show and blame are not here: --output=<file> makes them write a file.
var _readCommands = []string{
	"git status",
	"ls", "cat", "head", "tail", "grep", "wc", "pwd",
}

// writes reports whether a turn of r takes its tree's lock and gets a checkpoint: the role has
// write, or on claude the tools it keeps after access allow a shell command that is not in
// _readCommands. Codex without write runs in a read-only sandbox.
func writes(r store.Role) bool {
	if r.Write {
		return true
	}
	if r.Backend != "claude" {
		return false
	}
	_, tools, _ := access(r, nil)
	return slices.ContainsFunc(tools, changesFiles)
}

func changesFiles(tool string) bool {
	name, arg, _ := strings.Cut(tool, "(")
	if strings.TrimSpace(name) != "Bash" {
		return false
	}
	cmd := strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(arg, ")"), ":*"))
	return !slices.ContainsFunc(_readCommands, func(c string) bool {
		return cmd == c || strings.HasPrefix(cmd, c+" ")
	})
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
	direct, err := s.directExchanges(context.WithoutCancel(ctx))
	if err != nil {
		return store.Proposal{}, err
	}
	m, err := s.member(ctx, s.ch.Role, runInput{})
	if err != nil {
		return store.Proposal{}, err
	}
	m.run.NoWake = true
	defer func() { m.run.NoWake = false }()
	if err := s.run(ctx, m, runInput{text: direct + prompt, onEvent: onEvent}); err != nil {
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

// directExchanges is the channel's newest direct messages as plain text for a wrap-up or
// promote prompt, or empty when there are none.
func (s *Session) directExchanges(ctx context.Context) (string, error) {
	msgs, err := s.st.DirectMessages(ctx, s.ch.ID, 0)
	if err != nil || len(msgs) == 0 {
		return "", err
	}
	if len(msgs) > _maxDirectLines {
		msgs = msgs[len(msgs)-_maxDirectLines:]
	}
	var b strings.Builder
	b.WriteString(_directIntro)
	for _, m := range msgs {
		body := strings.TrimSpace(m.Body)
		if r := []rune(body); len(r) > _maxDirectRunes {
			body = string(r[:_maxDirectRunes]) + "…"
		}
		attempt := ""
		if m.Status == store.DirectFailed {
			attempt = " (failed)"
		}
		fmt.Fprintf(&b, "[direct %d] %s -> %s%s: %s\n", m.ChainID, m.From, m.To, attempt, body)
	}
	b.WriteString(_directAsk)
	return b.String(), nil
}

func (m *member) close(st *store.Store) error {
	if m.run != nil {
		m.run.endWatch()
	}
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
		switch {
		case t.Role != role || t.Backend != backend:
		case sessionMissing(t.Error):
			last = store.Turn{}
		case t.SessionID != "":
			last = t
		}
	}
	return last, nil
}

// sessionMissing reports whether msg says the backend has no session to resume: claude's
// error event on the first turn, or codex's error from thread/resume.
func sessionMissing(msg string) bool {
	return strings.Contains(msg, "No conversation found") ||
		strings.Contains(msg, "no rollout found for thread id")
}

// introSent is the role intro its session (same session id) in the channel was last given:
// the session's first turn always carries one, from firstPrompt, after its "## You" header,
// and a later turn carries one when the role's persona or skills changed since, from
// updatedRoleIntro. Empty when neither applies, so a prefix check against it never matches a
// non-empty roleIntro.
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
			sent, first = strings.TrimPrefix(t.Prompt, _youHeader), false
			continue
		}
		if after, ok := strings.CutPrefix(t.Prompt, _introChangedPrefix); ok {
			sent = after
		}
	}
	return sent, nil
}

// NewAgent returns the agent of backend, claude or codex.
func NewAgent(backend string) (agent.Agent, error) {
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
