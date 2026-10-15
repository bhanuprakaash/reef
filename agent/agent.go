// Package agent defines the interface every agent backend implements and the
// normalized Event it streams.
package agent

import (
	"context"
	"encoding/json"
)

// Agent runs one backend session (Claude Code, Codex) and streams normalized Events.
// Call Start or Resume once per Agent; to continue after Interrupt, Resume on a new Agent.
// ctx bounds only the call it is passed to; Interrupt is the only way to stop a session.
// Send is not safe for concurrent use.
type Agent interface {
	// Start launches a new session in cfg.Cwd.
	Start(ctx context.Context, cfg Config) error
	// Resume launches the backend continuing an earlier session. It needs the session id
	// reported by a KindSession event of that earlier session.
	Resume(ctx context.Context, cfg Config, sessionID string) error
	// Send starts one turn with the prompt. The turn ends with a KindTurnEnd or KindError event.
	Send(ctx context.Context, prompt string) error
	// Events streams events until the backend process exits; then the channel is closed.
	// It is valid before Start.
	Events() <-chan Event
	// Interrupt stops the current turn and the backend process. When it returns the
	// process is gone and Events is closed, also when Start failed or never ran. Tool
	// processes the backend started in their own process group may outlive a forced kill.
	// Events may still hold the usage of the stopped turn: read it until it closes.
	Interrupt() error
}

// Config is how a session starts.
type Config struct {
	Cwd             string
	PermissionMode  string               // backend-specific value, passed through
	Tools           []string             // tools allowed without asking; codex ignores it
	DisallowedTools []string             // tools denied whatever settings allow; codex ignores it
	MCPServers      map[string]MCPServer // extra MCP servers, by name
	Env             []string             // extra "KEY=value" on top of os.Environ
}

// MCPServer is a stdio server (Command) or an HTTP server (URL).
type MCPServer struct {
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	URL     string            `json:"url,omitempty"`
}

// Kind says which fields of an Event are set.
type Kind string

// Event kinds.
const (
	KindText       Kind = "text"
	KindToolCall   Kind = "tool_call"
	KindToolResult Kind = "tool_result"
	KindUsage      Kind = "usage"
	KindTurnEnd    Kind = "turn_end"
	KindError      Kind = "error"
	KindSession    Kind = "session"
)

// Event is one normalized step of a session. Its JSON is the wire format
// reef-agent prints, one line per event.
type Event struct {
	Kind         Kind            `json:"kind"`
	Text         string          `json:"text,omitempty"`      // text delta, error message
	SessionID    string          `json:"sessionId,omitempty"` // KindSession
	Model        string          `json:"model,omitempty"`     // KindSession
	ToolID       string          `json:"toolId,omitempty"`    // tool call and its result share it
	ToolName     string          `json:"toolName,omitempty"`
	ParentToolID string          `json:"parentToolId,omitempty"` // ToolID of the Agent call whose subagent made this
	Input        json.RawMessage `json:"input,omitempty"`        // tool call arguments
	Output       string          `json:"output,omitempty"`       // tool result
	IsError      bool            `json:"isError,omitempty"`      // tool result failed
	Usage        *Usage          `json:"usage,omitempty"`
	CostUSD      *float64        `json:"costUsd,omitempty"` // KindTurnEnd or KindError, claude only; the turn's own cost, not the process total
}

// Usage is the token count of a turn.
type Usage struct {
	InputTokens      int64 `json:"inputTokens"`
	OutputTokens     int64 `json:"outputTokens"`
	CacheReadTokens  int64 `json:"cacheReadTokens"`
	CacheWriteTokens int64 `json:"cacheWriteTokens"`
}

// Add adds v's counts to u.
func (u *Usage) Add(v Usage) {
	u.InputTokens += v.InputTokens
	u.OutputTokens += v.OutputTokens
	u.CacheReadTokens += v.CacheReadTokens
	u.CacheWriteTokens += v.CacheWriteTokens
}
