package space

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

// Session is a Space with one agent session per bot in use. Each is opened for its
// first turn. The lead's stays open until Close; a specialist's is closed after its turn.
type Session struct {
	st       *store.Store
	ms       *memory.Store
	dataDir  string
	exe      string
	ch       store.Space
	leadID   int64
	mk       func(backend string) (agent.Agent, error)
	lock     *os.File
	onPrompt func(prompt string)
	onNote   func()
	members  map[int64]*member
	trees    *Trees
	// renewTokens is the context size that has the Bot's next turn start a new backend context.
	renewTokens int64
}

type member struct {
	run   *Runner
	ag    agent.Agent
	token string
	// write and dir say which tree the role's turns hold, one at a time.
	write bool
	dir   string
}

type runInput struct {
	botID  *int64
	text   string
	parent *int64
	// messageID is the public message that woke the turn.
	messageID *int64
	direct    *store.DirectMessage
	// scopes, when set, say the wake was routed: text is what it was routed with, which a turn
	// that never starts keeps, and the turn's own text is built from the unread messages of
	// scopes when it starts.
	scopes store.Scopes
	// fork starts the agent in a copy of its session, which the Session does not keep.
	fork    bool
	onEvent func(agent.Event)
}

// TokenEnv carries the token to the Space's reef mcp. WrapCommand is the text that asks
// for a wrap-up.
const (
	TokenEnv    = "REEF_TOKEN"
	WrapCommand = "/wrap"

	_wrapPrompt = "Wrap up this Space. Call the get_brief tool first. Then call the " +
		"propose_wrap tool once with the full brief rewritten as the latest brief plus what " +
		"this Space achieved (goal, status, key decisions, open questions): keep what still " +
		"holds, make status where the work item is now, not a history, drop what is done or no " +
		"longer true, and keep it short. Add as items what this Space made that you have not " +
		"saved with remember yet: decisions, findings, facts, lessons and conventions, each " +
		"with its scope (work_item by default; project for what holds across the project; " +
		"bot for what only you should recall). Skip what you already saved with " +
		"remember. Use recall to find stale or duplicate " +
		"items: pass retire with their ids, and replaces on a new item that amends older " +
		"ones. Then reply with one line saying it is proposed."
	_maintainPrompt = "Review your own memory. Use recall with scope bot to list the items you keep " +
		"for yourself, and read_memory for the full ones. Where several say the same, call " +
		"remember with scope bot and replaces listing their ids, once for the group, with one item " +
		"that says it all. Call retire_memory on a lesson that is stale or no longer true. Change " +
		"only your own bot items: leave project, work item and every other shared scope as they " +
		"are, and post nothing to any Space. Then reply with one line saying what you changed."
	_maxDirectLines = 100
	_maxDirectRunes = 1000

	_directIntro = "Direct exchanges in this Space, private between two bots:\n"
	_directAsk   = "Propose as items what these exchanges decided or found that is not saved yet, " +
		"and a brief that reflects the direct work that is complete.\n\n"
)

// ErrNoProposal is returned by Wrap when the agent made no proposal.
var ErrNoProposal = errors.New("no proposal made")

// Open opens the Space; no agent starts until a turn needs it. mk makes the agent of each
// backend, NewAgent in production. The caller must Close the Session. Sessions that share
// trees run one write turn at a time on a working tree.
func Open(
	ctx context.Context, st *store.Store, ms *memory.Store, dataDir, exe string, spaceID int64,
	trees *Trees, mk func(backend string) (agent.Agent, error),
) (_ *Session, err error) {
	ch, err := st.Space(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	lock, err := lockChat(dataDir, spaceID)
	if err != nil {
		return nil, err
	}
	team, err := st.SpaceTeam(ctx, spaceID)
	if err != nil {
		_ = lock.Close()
		return nil, err
	}
	if len(team.Roles) == 0 {
		_ = lock.Close()
		return nil, fmt.Errorf("%w: Space %d has no members", store.ErrConflict, spaceID)
	}
	s := &Session{
		st: st, ms: ms, dataDir: dataDir, exe: exe, ch: ch, leadID: team.Roles[0].BotID, mk: mk, lock: lock,
		members: map[int64]*member{}, trees: trees, renewTokens: _renewTokens,
	}
	defer func() {
		if err != nil {
			_ = s.Close()
		}
	}()

	if err := failStaleTurns(ctx, st, spaceID); err != nil {
		return nil, err
	}
	return s, nil
}

// Start starts or resumes the lead now instead of at its first turn.
func (s *Session) Start(ctx context.Context) error {
	if err := s.ch.Writable(); err != nil {
		return err
	}
	_, err := s.member(ctx, "", runInput{botID: &s.leadID})
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
	ch, err := s.st.Space(ctx, s.ch.ID)
	if err != nil {
		return store.Message{}, err
	}
	if err := ch.Writable(); err != nil {
		return store.Message{}, err
	}
	msg, err := s.st.CreateMessage(ctx, store.Message{
		SpaceID: s.ch.ID, ParentID: parent, Author: _userAuthor, Body: body,
	})
	if err != nil {
		return store.Message{}, err
	}
	return msg, s.st.SaveMessageRefs(ctx, msg.ID, store.ParseSpaceRefs(body))
}

// Run runs one turn of the lead with text. A cancelled ctx wraps ctx.Err(). The reply is a
// reply in the thread of parent when it is set.
func (s *Session) Run(
	ctx context.Context, text string, parent *int64, onEvent func(agent.Event),
) error {
	return s.RunBot(ctx, s.leadID, text, parent, nil, onEvent)
}

// RunAs is Run for any role of the Space's team. A specialist's agent is closed when
// its turn ends; its next turn resumes the session. A failure to close it is logged, and
// does not fail the turn. A role that writes waits for the turns of other writers on its
// tree, and its turn starts when they end. The turn keeps messageID, the public message that
// woke it, when it is set.
func (s *Session) RunAs(
	ctx context.Context, role, text string, parent, messageID *int64, onEvent func(agent.Event),
) error {
	return s.runAs(ctx, role,
		runInput{text: text, parent: parent, messageID: messageID, onEvent: onEvent})
}

// WakeAs is RunAs for a wake routed with routed, in scopes. The turn's text is not routed: it
// is built when the turn starts, from the messages of scopes that the role has not been sent
// in its session. A wake with none runs no turn. A wake that never starts keeps routed.
func (s *Session) WakeAs(
	ctx context.Context, role, routed string, scopes store.Scopes, parent, messageID *int64,
	onEvent func(agent.Event),
) error {
	if s.allRead(ctx, role, scopes) {
		return nil
	}
	return s.runAs(ctx, role,
		runInput{text: routed, parent: parent, messageID: messageID, scopes: scopes, onEvent: onEvent})
}

func (s *Session) RunBot(ctx context.Context, id int64, text string, parent, messageID *int64, onEvent func(agent.Event)) error {
	return s.runAs(ctx, "", runInput{botID: &id, text: text, parent: parent, messageID: messageID, onEvent: onEvent})
}

func (s *Session) WakeBot(ctx context.Context, id int64, routed string, scopes store.Scopes, parent, messageID *int64, onEvent func(agent.Event)) error {
	if s.allReadBot(ctx, id, scopes) {
		return nil
	}
	return s.runAs(ctx, "", runInput{botID: &id, text: routed, parent: parent, messageID: messageID, scopes: scopes, onEvent: onEvent})
}

func (s *Session) RunDirectBot(ctx context.Context, id int64, text string, dm store.DirectMessage, onEvent func(agent.Event)) error {
	if dm.ToBotID == nil || *dm.ToBotID != id {
		return fmt.Errorf("%w: direct recipient does not match Bot", store.ErrInvalid)
	}
	return s.runDirect(ctx, "", runInput{botID: &id, text: text, direct: &dm, onEvent: onEvent})
}

// allRead reports whether role's session has been sent every message of scopes, so no agent
// need start. It is false when it cannot tell: the turn then starts and finds out.
func (s *Session) allRead(ctx context.Context, role string, scopes store.Scopes) bool {
	ctx = context.WithoutCancel(ctx)
	team, err := s.st.SpaceTeam(ctx, s.ch.ID)
	if err != nil {
		return false
	}
	r, err := s.roleSetup(team, role)
	if err != nil {
		return false
	}
	return s.readScopes(ctx, r, scopes)
}

func (s *Session) allReadBot(ctx context.Context, id int64, scopes store.Scopes) bool {
	ctx = context.WithoutCancel(ctx)
	team, err := s.st.SpaceTeam(ctx, s.ch.ID)
	if err != nil {
		return false
	}
	for _, r := range team.Roles {
		if r.BotID == id {
			return s.readScopes(ctx, r, scopes)
		}
	}
	return false
}

func (s *Session) readScopes(ctx context.Context, r store.Role, scopes store.Scopes) bool {
	sessionID := ""
	if m, ok := s.members[r.BotID]; ok {
		if m.run.sessionGone {
			return false
		}
		sessionID = m.run.sessionID
	} else {
		bs, err := s.botSession(ctx, r.BotID)
		if err != nil {
			return false
		}
		last, err := newestSessionTurn(ctx, s.st, s.ch.ID, r.BotID, bs.ID)
		if err != nil {
			return false
		}
		sessionID = last.SessionID
	}
	text, _, err := wakeText(ctx, s.st, s.ch.ID, r.BotID, sessionID, scopes)
	return err == nil && text == ""
}

func (s *Session) runAs(ctx context.Context, role string, in runInput) error {
	m, err := s.member(ctx, role, in)
	if err != nil {
		return err
	}
	err = s.run(ctx, m, in)
	if *m.run.botID == s.leadID {
		return err
	}
	delete(s.members, *m.run.botID)
	if cerr := m.close(s.st); cerr != nil {
		log.Printf("close %s of Space %d: %v", role, s.ch.ID, cerr)
	}
	return err
}

// RunDirectAs runs a role for a direct message, keeping the request/response boundary in
// Runner while applying the same role setup, worktree access and session lifetime as RunAs.
func (s *Session) RunDirectAs(ctx context.Context, role, text string, dm store.DirectMessage, onEvent func(agent.Event)) error {
	id, err := s.st.SpaceMemberBotID(context.WithoutCancel(ctx), s.ch.ID, role)
	if err != nil {
		return err
	}
	return s.RunDirectBot(ctx, id, text, dm, onEvent)
}

func (s *Session) runDirect(ctx context.Context, role string, in runInput) error {
	dm := in.direct
	m, err := s.member(ctx, role, in)
	if err != nil {
		if _, rerr := s.st.CreateFailedDirectResponse(ctx, dm.ID, dm.ToBotID, role, ""); rerr != nil && !errors.Is(rerr, store.ErrConflict) {
			log.Printf("direct response for %d: %v", dm.ID, rerr)
		}
		return err
	}
	if dm.ToBotID == nil || *dm.ToBotID != *m.run.botID {
		return fmt.Errorf("%w: direct recipient does not match Bot", store.ErrInvalid)
	}
	err = s.run(ctx, m, in)
	if *m.run.botID != s.leadID {
		delete(s.members, *m.run.botID)
		if closeErr := m.close(s.st); closeErr != nil {
			log.Printf("close %s of Space %d: %v", m.run.role, s.ch.ID, closeErr)
		}
	}
	return err
}

// run runs a turn of m, holding m's tree when it writes and counting as a reader of it when
// it does not. A ctx cancelled while it waits leaves an interrupted turn.
func (s *Session) run(ctx context.Context, m *member, in runInput) error {
	if *m.run.botID == s.leadID {
		defer m.run.Watch()
	}
	if !m.write {
		defer s.trees.Read(m.dir)()
		return s.turn(ctx, m, in)
	}
	unlock, err := s.trees.Lock(ctx, m.dir)
	if err != nil {
		in.text = m.run.wake(in.text)
		err = s.failTurn(ctx, m.run.role, m.run.backend, m.run.botID, in, err)
		if in.direct != nil {
			if _, rerr := s.st.CreateFailedDirectResponse(
				context.WithoutCancel(ctx), in.direct.ID, m.run.botID, m.run.role, "",
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
			context.WithoutCancel(ctx), s.st, s.ch.ID, *m.run.botID, m.run.sessionID, in.scopes)
		if err != nil {
			return s.failTurn(ctx, m.run.role, m.run.backend, m.run.botID, in, err)
		}
		if text == "" {
			return nil
		}
		parent = nil
		if thread := sent[0].Thread; thread != 0 {
			parent = &thread
		}
	}
	guidance, err := s.st.UnseenGuidance(context.WithoutCancel(ctx), s.ch.ID, *m.run.botID)
	if err != nil {
		return s.failTurn(ctx, m.run.role, m.run.backend, m.run.botID, in, err)
	}
	var notice strings.Builder
	for _, g := range guidance {
		fmt.Fprintf(&notice, "User guidance (to %s, message %d): %s\n\n", g.To, *g.OriginMessageID, g.Body)
	}
	text = notice.String() + text
	m.run.Scopes, m.run.WakeMessage = sent, in.messageID
	if in.direct != nil {
		return m.run.RunDirect(ctx, text, *in.direct, in.onEvent)
	}
	return m.run.Run(ctx, text, parent, in.onEvent)
}

// botSession is the active BotSession of the Bot in the Space: the settings it adopted. A Bot with
// none adopts its current settings now.
func (s *Session) botSession(ctx context.Context, botID int64) (store.BotSession, error) {
	bs, err := s.st.ActiveBotSession(ctx, s.ch.ID, botID)
	if errors.Is(err, store.ErrNotFound) {
		return AdoptBotSession(ctx, s.st, s.dataDir, s.ch.ID, botID)
	}
	return bs, err
}

// failTurn stores a failed turn of role for a turn that never started and returns cause
// joined with the error of storing it. The turn keeps the wake it was for, so retry can run
// it again; a ctx cancelled by then makes the turn interrupted.
func (s *Session) failTurn(
	ctx context.Context, role, backend string, botID *int64, in runInput, cause error,
) error {
	reason := cause.Error()
	if ctx.Err() != nil {
		reason = "interrupted"
	}
	ctx = context.WithoutCancel(ctx)
	var botSessionID *int64
	if botID != nil {
		if bs, err := s.st.ActiveBotSession(ctx, s.ch.ID, *botID); err == nil {
			botSessionID = &bs.ID
		}
	}
	t, err := s.st.CreateTurn(ctx, store.Turn{
		SpaceID: s.ch.ID, BotID: botID, BotSessionID: botSessionID, Role: role, Backend: backend, WakeText: in.text, WakeParent: in.parent,
		DirectMessageID: in.direct.IDRef(), WakeMessageID: in.messageID, Scopes: in.scopes,
	})
	if err != nil {
		log.Printf("store failed start of %s in Space %d: %v", role, s.ch.ID, err)
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

// Close ends the Session.
func (s *Session) Close() error {
	return errors.Join(s.closeMembers(), s.lock.Close())
}

// closeMembers ends every open agent, so the next turn of each role starts one again.
func (s *Session) closeMembers() error {
	var errs []error
	for id, m := range s.members {
		errs = append(errs, m.close(s.st))
		delete(s.members, id)
	}
	return errors.Join(errs...)
}

// member returns the agent session of role, starting or resuming it from the newest turn with a
// session id of the role's active BotSession. The role runs with the settings that BotSession
// adopted, not the Bot's current ones. A new session reads the Space's members from the store,
// and its first prompt reads them when it is built. An agent that fails to start, for any
// reason, leaves a failed turn for the role that keeps the wake it was starting for, and so
// does a ctx cancelled before it starts: the turn is interrupted. The store is read even
// then, as Run stores its turn. A role the team no longer has leaves one too, on the
// Space's backend.
func (s *Session) member(ctx context.Context, role string, in runInput) (_ *member, err error) {
	wctx := context.WithoutCancel(ctx)
	team, err := s.st.SpaceTeam(wctx, s.ch.ID)
	if err != nil {
		return nil, err
	}
	backend := "claude"
	if len(team.Roles) > 0 {
		backend = team.Roles[0].Backend
	}
	var botID *int64
	defer func() {
		if err != nil {
			err = s.failTurn(ctx, role, backend, botID, in, err)
		}
	}()
	var r store.Role
	if in.botID != nil {
		for _, member := range team.Roles {
			if member.BotID == *in.botID {
				r = member
				break
			}
		}
		if r.BotID == 0 {
			return nil, fmt.Errorf("%w: Bot %d is not a Space member", store.ErrInvalid, *in.botID)
		}
		role = r.Name
	} else {
		r, err = s.roleSetup(team, role)
		if err != nil {
			return nil, err
		}
	}
	botID = &r.BotID
	bs, err := s.botSession(wctx, r.BotID)
	if err != nil {
		return nil, err
	}
	r = bs.Apply(r)
	backend = r.Backend
	last, err := newestSessionTurn(wctx, s.st, s.ch.ID, r.BotID, bs.ID)
	if err != nil {
		return nil, err
	}
	renew := !in.fork && last.ContextTokens >= s.renewTokens
	if m, ok := s.members[r.BotID]; ok && !in.fork {
		m.run.endWatch()
		_, err := os.Stat(m.dir)
		if !errors.Is(err, os.ErrNotExist) && !m.run.sessionGone && !renew && *m.run.botSessID == bs.ID {
			m.run.role = role
			return m, nil
		}
		delete(s.members, *m.run.botID)
		if err := m.close(s.st); err != nil {
			log.Printf("close %s of Space %d: %v", role, s.ch.ID, err)
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

	if in.fork && r.Backend != "claude" {
		last = store.Turn{} // only claude can resume a copy of a session
	}
	if renew {
		last = store.Turn{}
	}
	var history, refreshed, joined bool
	if last.SessionID == "" && !in.fork {
		if history, refreshed, err = replacement(wctx, s.st, s.ch.ID, r.BotID, bs.ID); err != nil {
			return nil, err
		}
		if !history {
			if joined, err = newcomer(wctx, s.st, s.ch.ID, r); err != nil {
				return nil, err
			}
		}
	}
	tok, err := s.st.CreateToken(wctx, s.ch.ID, r.BotID)
	if err != nil {
		return nil, err
	}
	m.token = tok.Token
	servers := map[string]agent.MCPServer{"reef": {
		Command: s.exe,
		Args:    []string{"mcp", "--data", s.dataDir},
		Env:     map[string]string{TokenEnv: tok.Token},
	}}
	for _, c := range r.Connections {
		servers[c.Name] = c.Server()
	}
	cwd, key, err := WorkDir(wctx, s.st, s.dataDir, s.ch.ID)
	if err != nil {
		return nil, err
	}
	m.dir = cwd
	cfg := roleConfig(r, cwd, bs.Skills)
	cfg.MCPServers = servers
	if skillsPrompt(bs.Skills) != "" {
		cfg.AddDirs = []string{BotSkillsDir(s.dataDir, bs.ID)}
	}
	cfg.Bin, err = s.st.RoleBin(wctx, r)
	if err == nil && ctx.Err() == nil && key != "" {
		var c store.Space
		var p store.Project
		if c, err = s.st.Space(wctx, s.ch.ID); err == nil {
			p, err = s.st.Project(wctx, c.ProjectID)
		}
		if err == nil {
			err = EnsureWorktree(ctx, s.trees, p, cwd, key)
		}
	}
	switch {
	case err != nil:
	case ctx.Err() != nil:
		err = ctx.Err()
	case last.SessionID != "":
		cfg.Fork = in.fork
		err = ag.Resume(ctx, cfg, last.SessionID)
	default:
		err = ag.Start(ctx, cfg)
	}
	if err != nil {
		return nil, err
	}
	m.run = NewRunner(s.st, ag, s.ch.ID, role, r.Backend, "")
	m.run.botID, m.run.botSessID = &r.BotID, &bs.ID
	m.run.sessionID = last.SessionID
	if last.SessionID != "" {
		compacted, err := s.st.SessionCompacted(wctx, s.ch.ID, r.BotID, last.SessionID)
		if err != nil {
			return nil, err
		}
		m.run.firstPromptSent = !compacted
	}
	m.run.FirstPrompt = func(ctx context.Context) (string, error) {
		ch, err := s.st.Space(ctx, s.ch.ID)
		if err != nil {
			return "", err
		}
		team, err := s.st.SpaceTeam(ctx, s.ch.ID)
		if err != nil {
			return "", err
		}
		var r store.Role
		for _, member := range team.Roles {
			if member.BotID == *m.run.botID {
				r = member
				break
			}
		}
		if r.BotID == 0 {
			return "", fmt.Errorf("%w: Bot %d is not in Space", store.ErrNotFound, *m.run.botID)
		}
		return firstPrompt(ctx, s.st, s.ms, ch.ProjectID, ch.WorkItemID, ch.ID, team, bs.Apply(r))
	}
	if history || joined {
		m.run.Handover = func(ctx context.Context) (string, error) {
			ch, err := s.st.Space(ctx, s.ch.ID)
			if err != nil {
				return "", err
			}
			return handover(ctx, s.st, s.ms, ch, r.BotID, cwd, joined)
		}
	}
	if refreshed {
		m.run.refreshNote = refreshNote(r)
	}
	m.run.OnPrompt = s.onPrompt
	m.run.Memory = s.ms
	if m.write && (key != "" || worktree.IsRepo(wctx, cwd)) {
		m.run.CheckpointDir = cwd
		m.run.DataDir = s.dataDir
	}
	m.run.OnNote = s.onNote
	if !in.fork {
		s.members[r.BotID] = m
	}
	return m, nil
}

// Refresh ends the Bot's BotSession in the Space and adopts the Bot's current settings. Its agent
// is closed, so its next turn starts a new backend context, with a handover, and never resumes the
// earlier one. Call it only between turns.
func (s *Session) Refresh(ctx context.Context, botID int64) error {
	if _, err := AdoptBotSession(ctx, s.st, s.dataDir, s.ch.ID, botID); err != nil {
		return err
	}
	return s.Drop(botID)
}

// Drop closes the Bot's agent, if it has one open, so its next turn starts one again.
func (s *Session) Drop(botID int64) error {
	if m, ok := s.members[botID]; ok {
		delete(s.members, botID)
		return m.close(s.st)
	}
	return nil
}

func refreshNote(r store.Role) string {
	settings := r.Backend
	if r.Model != "" {
		settings += ", " + r.Model
	}
	return r.Name + " refreshed: " + settings
}

func roleConfig(r store.Role, cwd string, skills []store.AdoptedSkill) agent.Config {
	mode, tools, denied := access(r, claudeSkills(r, cwd))
	return agent.Config{
		Cwd:             cwd,
		PermissionMode:  mode,
		Model:           r.Model,
		Instructions:    Instructions(r, skills),
		Tools:           append(tools, "mcp__reef__*"),
		DisallowedTools: denied,
	}
}

// ChatDir is the neutral folder of Space spaceID, which has no work item.
func ChatDir(dataDir string, spaceID int64) string {
	return filepath.Join(dataDir, "chats", strconv.FormatInt(spaceID, 10))
}

// WorkDir is where an agent of Space spaceID runs: the worktree of its work item, or else a
// folder of its own under the data folder, made when it is missing. That folder is no
// repository, and a project the Space is associated with is not its folder. The work item is
// read now, as an attach may have given it one since the Space opened. key is the work item's
// tree key, empty for a Space with no work item.
func WorkDir(
	ctx context.Context, st *store.Store, dataDir string, spaceID int64,
) (dir, key string, err error) {
	ch, err := st.Space(ctx, spaceID)
	if err != nil {
		return "", "", err
	}
	if ch.WorkItemID == nil {
		dir = ChatDir(dataDir, spaceID)
		return dir, "", os.MkdirAll(dir, 0o700)
	}
	p, err := st.Project(ctx, ch.ProjectID)
	if err != nil {
		return "", "", err
	}
	f, err := st.WorkItem(ctx, *ch.WorkItemID)
	if err != nil {
		return "", "", err
	}
	return worktree.Path(dataDir, p.ID, f.TreeKey()), f.TreeKey(), nil
}

func (s *Session) roleSetup(team store.Team, name string) (store.Role, error) {
	if len(team.Roles) == 0 {
		return store.Role{}, fmt.Errorf("%w: Space %d has no members", store.ErrConflict, s.ch.ID)
	}
	for _, r := range team.Roles {
		if r.Name != name {
			continue
		}
		return r, nil
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
// mode the role sets stays when the role writes, so it may narrow it. A claude role that picked
// skills never loads a skill from disk through claude's Skill tool, picked or not: onDisk names
// the skills on disk and each is denied, so the role uses the copy its instructions list.
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
		denied = slices.Clone(denied)
		for _, name := range onDisk {
			denied = append(denied, "Skill("+name+")")
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
	// The prompt holds every pair's direct messages, so the turn runs in a copy of the lead's
	// session on claude, and in a fresh thread on codex: the lead's own session never sees them.
	m, err := s.member(ctx, "", runInput{botID: &s.leadID, fork: true})
	if err != nil {
		return store.Proposal{}, err
	}
	m.run.NoWake = true
	defer func() {
		if err := m.close(s.st); err != nil {
			log.Printf("close wrap-up copy of %s of Space %d: %v", s.ch.Role, s.ch.ID, err)
		}
	}()
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

// directExchanges is the Space's newest direct messages as plain text for a wrap-up prompt,
// or empty when there are none.
func (s *Session) directExchanges(ctx context.Context) (string, error) {
	msgs, err := s.st.DirectMessages(ctx, s.ch.ID, 0)
	if err != nil || len(msgs) == 0 {
		return "", err
	}
	omitted := max(len(msgs)-_maxDirectLines, 0)
	msgs = msgs[omitted:]
	var b strings.Builder
	b.WriteString(_directIntro)
	if omitted > 0 {
		fmt.Fprintf(&b, "%d older exchanges left out (read_direct_messages)\n", omitted)
	}
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
		if err := st.DeleteToken(context.Background(), m.token); !errors.Is(err, store.ErrNotFound) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// InUse reports whether a Session holds the Space's chat lock.
func InUse(dataDir string, spaceID int64) bool {
	lock, err := os.Open(chatLockPath(dataDir, spaceID))
	if err != nil {
		return false
	}
	defer func() { _ = lock.Close() }()
	return errors.Is(syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB), syscall.EWOULDBLOCK)
}

func failStaleTurns(ctx context.Context, st *store.Store, spaceID int64) error {
	turns, err := st.Turns(ctx, store.TurnFilter{SpaceID: &spaceID, Status: "running"})
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

// newestSessionTurn is the newest turn of the BotSession botSessionID that holds the session to
// resume, or none when the session is gone.
func newestSessionTurn(
	ctx context.Context, st *store.Store, spaceID, botID, botSessionID int64,
) (store.Turn, error) {
	turns, err := st.Turns(ctx, store.TurnFilter{SpaceID: &spaceID, BotID: &botID})
	if err != nil {
		return store.Turn{}, err
	}
	var last store.Turn
	for _, t := range turns {
		switch {
		case t.BotSessionID == nil || *t.BotSessionID != botSessionID:
		case sessionMissing(t.Error):
			last = store.Turn{}
		case t.WakeText == "":
			// A wrap-up turn keeps no wake and runs in a copy of the session: it is not the one to resume.
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

func chatLockPath(dataDir string, spaceID int64) string {
	return filepath.Join(dataDir, fmt.Sprintf("chat-%d.lock", spaceID))
}

func lockChat(dataDir string, spaceID int64) (*os.File, error) {
	path := chatLockPath(dataDir, spaceID)
	lock, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = lock.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf("space %d is open elsewhere: %w", spaceID, store.ErrConflict)
		}
		return nil, fmt.Errorf("lock %s: %w", path, err)
	}
	return lock, nil
}
