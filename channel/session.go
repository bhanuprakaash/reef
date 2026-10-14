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
	"syscall"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/agent/claude"
	"github.com/bhanuprakaash/reef/agent/codex"
	"github.com/bhanuprakaash/reef/store"
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
}

type member struct {
	run   *Runner
	ag    agent.Agent
	token string
}

// TokenEnv carries the token to the channel's reef mcp. WrapCommand is the text that asks
// for a wrap-up; PromoteCommand is the stored message of a promote turn.
const (
	TokenEnv       = "REEF_TOKEN"
	WrapCommand    = "/wrap"
	PromoteCommand = "/promote"

	_wrapPrompt = "Wrap up this channel. Call the propose_wrap tool once with the full " +
		"rewritten brief (goal, status, key decisions, open questions), your own notes on " +
		"this feature, and the decisions made in this channel that are not recorded yet. " +
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
// the Session.
func Open(
	ctx context.Context, st *store.Store, dataDir, exe string, channelID int64,
) (*Session, error) {
	return open(ctx, st, dataDir, exe, channelID, newAgent)
}

func open(
	ctx context.Context, st *store.Store, dataDir, exe string, channelID int64,
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
		members: map[string]*member{},
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
	_, err := s.member(ctx, s.ch.Role)
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

// Run runs one turn of the lead with text. A cancelled ctx wraps ctx.Err(); a turn over
// the tool call cap pauses the channel and wraps ErrCapped. The reply is a reply in the
// thread of parent when it is set.
func (s *Session) Run(
	ctx context.Context, text string, parent *int64, onEvent func(agent.Event),
) error {
	return s.RunAs(ctx, s.ch.Role, text, parent, onEvent)
}

// RunAs is Run for any role of the channel's team. A specialist's agent is closed when
// its turn ends; its next turn resumes the session. A failure to close it is logged, and
// does not fail the turn.
func (s *Session) RunAs(
	ctx context.Context, role, text string, parent *int64, onEvent func(agent.Event),
) error {
	m, err := s.member(ctx, role)
	if err != nil {
		return err
	}
	err = m.run.Run(ctx, text, parent, onEvent)
	if role == s.ch.Role {
		return err
	}
	delete(s.members, role)
	if cerr := m.close(s.st); cerr != nil {
		log.Printf("close %s of channel %d: %v", role, s.ch.ID, cerr)
	}
	return err
}

// Wrap runs a wrap-up turn and returns the proposal the agent made, or ErrNoProposal. Like
// Run, a cancelled ctx still stores the turn.
func (s *Session) Wrap(ctx context.Context, onEvent func(agent.Event)) (store.Proposal, error) {
	return s.propose(ctx, _wrapPrompt, onEvent)
}

// Promote runs a promote turn and returns the proposal the agent made, or ErrNoProposal.
// The channel must already have its feature. Like Run, a cancelled ctx still stores the turn.
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
	return s.propose(ctx, fmt.Sprintf(prompt, f.Name, f.Slug), onEvent)
}

// Close ends the Session.
func (s *Session) Close() error {
	var errs []error
	for _, m := range s.members {
		errs = append(errs, m.close(s.st))
	}
	return errors.Join(append(errs, s.lock.Close())...)
}

// member returns the agent session of role, starting or resuming it from the role's own
// newest turn with a session id on the role's backend. A new session reads the team from the
// store, so a team applied since the channel opened is the one it gets. An agent that fails
// to start leaves a failed turn for the role, and so does a ctx cancelled before it starts:
// the turn is interrupted. The store is read even then, as Run stores its turn.
func (s *Session) member(ctx context.Context, role string) (_ *member, err error) {
	if m, ok := s.members[role]; ok {
		return m, nil
	}
	wctx := context.WithoutCancel(ctx)
	team, err := s.st.Team(wctx, s.ch.Team)
	if err != nil {
		return nil, err
	}
	r, err := s.roleSetup(team, role)
	if err != nil {
		return nil, err
	}
	ag, err := s.mk(r.Backend)
	if err != nil {
		return nil, err
	}
	m := &member{ag: ag}
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
	cfg := agent.Config{
		Cwd:            s.project.Path,
		PermissionMode: r.PermissionMode,
		Tools:          append(slices.Clone(r.Tools), "mcp__reef__*"),
		MCPServers:     servers,
	}
	var preamble string
	switch {
	case ctx.Err() != nil:
		err = ctx.Err()
	case last.SessionID != "":
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
		t, terr := s.st.CreateTurn(wctx, store.Turn{
			ChannelID: s.ch.ID, Role: role, Backend: r.Backend,
		})
		if terr == nil {
			t.Status, t.Error = "failed", reason
			_, terr = s.st.EndTurn(wctx, t)
		}
		return nil, errors.Join(err, terr)
	}
	m.run = NewRunner(s.st, ag, s.ch.ID, role, r.Backend, preamble)
	m.run.OnPrompt = s.onPrompt
	s.members[role] = m
	return m, nil
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

func (s *Session) propose(
	ctx context.Context, prompt string, onEvent func(agent.Event),
) (store.Proposal, error) {
	before, err := s.st.PendingProposal(context.WithoutCancel(ctx), s.ch.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return store.Proposal{}, err
	}
	if err := s.Run(ctx, prompt, nil, onEvent); err != nil {
		return store.Proposal{}, err
	}
	p, err := s.st.PendingProposal(ctx, s.ch.ID)
	if errors.Is(err, store.ErrNotFound) || (err == nil && p.ID == before.ID) {
		return store.Proposal{}, ErrNoProposal
	}
	return p, err
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
		return roleIntro(r) + projectContext(p, r.Name) + teamIntro(team, r), nil
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
