package channel

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/agent/claude"
	"github.com/bhanuprakaash/reef/agent/codex"
	"github.com/bhanuprakaash/reef/store"
)

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
)

// ErrNoProposal is returned by Wrap when the agent made no proposal.
var ErrNoProposal = errors.New("no proposal made")

// Session is a channel with a running agent.
type Session struct {
	run   *Runner
	st    *store.Store
	ag    agent.Agent
	lock  *os.File
	token string
}

// Open starts or resumes the channel's agent. The caller must Close the Session.
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
	ag, err := mk(ch.Backend)
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
	s := &Session{st: st, ag: ag, lock: lock}
	defer func() {
		if err != nil {
			_ = s.Close()
		}
	}()

	if err := failStaleTurns(ctx, st, channelID); err != nil {
		return nil, err
	}
	last, err := newestSessionTurn(ctx, st, channelID)
	if err != nil {
		return nil, err
	}
	tok, err := st.CreateToken(ctx, channelID, ch.Role)
	if err != nil {
		return nil, err
	}
	s.token = tok.Token
	cfg := agent.Config{
		Cwd:            p.Path,
		PermissionMode: ch.PermissionMode,
		MCPServers: map[string]agent.MCPServer{"reef": {
			Command: exe,
			Args:    []string{"mcp", "--data", dataDir},
			Env:     map[string]string{TokenEnv: tok.Token},
		}},
	}
	var preamble string
	if last.SessionID != "" {
		err = ag.Resume(ctx, cfg, last.SessionID)
	} else {
		if preamble, err = startPrompt(ctx, st, ch, p); err != nil {
			return nil, err
		}
		err = ag.Start(ctx, cfg)
	}
	if err != nil {
		return nil, err
	}
	s.run = NewRunner(st, ag, channelID, ch.Role, ch.Backend, preamble)
	return s, nil
}

// SetOnPrompt sets a hook that sees every prompt before it is sent to the agent.
func (s *Session) SetOnPrompt(f func(prompt string)) { s.run.OnPrompt = f }

// Say stores body as the user message.
func (s *Session) Say(ctx context.Context, body string) error { return s.run.Say(ctx, body) }

// Run runs one turn with text. A cancelled ctx wraps ctx.Err(); a turn over the tool
// call cap pauses the channel and wraps ErrCapped.
func (s *Session) Run(ctx context.Context, text string, onEvent func(agent.Event)) error {
	return s.run.Run(ctx, text, onEvent)
}

// Wrap runs a wrap-up turn and returns the proposal the agent made, or ErrNoProposal.
func (s *Session) Wrap(ctx context.Context, onEvent func(agent.Event)) (store.Proposal, error) {
	return s.propose(ctx, _wrapPrompt, onEvent)
}

// Promote runs a promote turn and returns the proposal the agent made, or ErrNoProposal.
// The channel must already have its feature.
func (s *Session) Promote(ctx context.Context, onEvent func(agent.Event)) (store.Proposal, error) {
	ch, err := s.st.Channel(ctx, s.run.channelID)
	if err != nil {
		return store.Proposal{}, err
	}
	if ch.FeatureID == nil {
		return store.Proposal{}, fmt.Errorf("%w: channel %d has no feature", store.ErrInvalid, ch.ID)
	}
	f, err := s.st.Feature(ctx, *ch.FeatureID)
	if err != nil {
		return store.Proposal{}, err
	}
	return s.propose(ctx, fmt.Sprintf(_promotePrompt, f.Name, f.Slug), onEvent)
}

func (s *Session) propose(
	ctx context.Context, prompt string, onEvent func(agent.Event),
) (store.Proposal, error) {
	before, err := s.st.PendingProposal(ctx, s.run.channelID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return store.Proposal{}, err
	}
	if err := s.run.Run(ctx, prompt, onEvent); err != nil {
		return store.Proposal{}, err
	}
	p, err := s.st.PendingProposal(ctx, s.run.channelID)
	if errors.Is(err, store.ErrNotFound) || (err == nil && p.ID == before.ID) {
		return store.Proposal{}, ErrNoProposal
	}
	return p, err
}

// Close ends the Session.
func (s *Session) Close() error {
	errs := []error{s.ag.Interrupt()}
	if s.token != "" {
		errs = append(errs, s.st.DeleteToken(context.Background(), s.token))
	}
	return errors.Join(append(errs, s.lock.Close())...)
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
	ctx context.Context, st *store.Store, ch store.Channel, p store.Project,
) (string, error) {
	if ch.FeatureID == nil {
		return projectContext(p, ch.Role), nil
	}
	return Context(ctx, st, *ch.FeatureID, ch.Role)
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

func newestSessionTurn(ctx context.Context, st *store.Store, channelID int64) (store.Turn, error) {
	turns, err := st.Turns(ctx, store.TurnFilter{ChannelID: &channelID})
	if err != nil {
		return store.Turn{}, err
	}
	var last store.Turn
	for _, t := range turns {
		if t.SessionID != "" {
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
