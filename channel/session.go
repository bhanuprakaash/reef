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
	// TokenEnv is the variable that carries the token to the channel's reef mcp.
	TokenEnv = "REEF_TOKEN"
	// WrapCommand is the message that asks for a wrap-up.
	WrapCommand = "/wrap"

	_wrapPrompt = "Wrap up this channel. Call the propose_wrap tool once with the full " +
		"rewritten brief (goal, status, key decisions, open questions), your own notes on " +
		"this feature, and the decisions made in this channel that are not recorded yet. " +
		"Then reply with one line saying it is proposed."
)

// ErrNoProposal is returned by Wrap when the agent made no proposal.
var ErrNoProposal = errors.New("no proposal made")

// Session is a channel with a running agent. It holds the channel's chat lock and
// its token until Close.
type Session struct {
	*Runner

	st    *store.Store
	ag    agent.Agent
	lock  *os.File
	token string
}

// Open starts the agent of a channel in its project's folder with the reef MCP
// server, which is exe mcp on dataDir. It resumes the session of the channel's
// newest turn that has one, and otherwise starts a new session whose first prompt
// is the channel's context. It first fails any turn the channel left running.
// The caller must Close the Session.
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

	last, err := resumeTurn(ctx, st, channelID)
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
	s.Runner = NewRunner(st, ag, channelID, ch.Role, ch.Backend, preamble)
	return s, nil
}

// Wrap asks the agent for a wrap-up proposal, storing the user message /wrap, and
// returns the proposal it made in that turn. It returns ErrNoProposal when the agent
// made none.
func (s *Session) Wrap(ctx context.Context, onEvent func(agent.Event)) (store.Proposal, error) {
	before, err := s.st.PendingProposal(ctx, s.channelID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return store.Proposal{}, err
	}
	if err := s.SendAs(ctx, WrapCommand, _wrapPrompt, onEvent); err != nil {
		return store.Proposal{}, err
	}
	p, err := s.st.PendingProposal(ctx, s.channelID)
	if errors.Is(err, store.ErrNotFound) || (err == nil && p.ID == before.ID) {
		return store.Proposal{}, ErrNoProposal
	}
	return p, err
}

// Close stops the agent, deletes the token and releases the chat lock.
func (s *Session) Close() error {
	errs := []error{s.ag.Interrupt()}
	if s.token != "" {
		errs = append(errs, s.st.DeleteToken(context.Background(), s.token))
	}
	return errors.Join(append(errs, s.lock.Close())...)
}

// InUse reports whether a Session holds the chat lock of the channel in dataDir.
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

// resumeTurn is the channel's newest turn that has a session id, or the zero Turn.
// It first fails any turn left running by a chat that crashed; the caller holds the
// chat lock.
func resumeTurn(ctx context.Context, st *store.Store, channelID int64) (store.Turn, error) {
	turns, err := st.Turns(ctx, store.TurnFilter{ChannelID: &channelID})
	if err != nil {
		return store.Turn{}, err
	}
	var last store.Turn
	for _, t := range turns {
		if t.Status == "running" {
			t.Status, t.Error = "failed", "chat exited"
			if _, err := st.EndTurn(ctx, t); err != nil {
				return store.Turn{}, err
			}
		}
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
		return nil, fmt.Errorf("unknown backend %q", backend)
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
			return nil, fmt.Errorf("channel %d is open in another reef chat", channelID)
		}
		return nil, fmt.Errorf("lock %s: %w", path, err)
	}
	return lock, nil
}
