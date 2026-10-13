// Package codex implements agent.Agent over `codex app-server`, a JSON-RPC
// conversation on the child's stdin and stdout.
package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/bhanuprakaash/reef/agent"
)

var _ agent.Agent = (*Agent)(nil)

const (
	_killAfter  = time.Second
	_waitDelay  = 2 * time.Second
	_stderrKeep = 4 << 10

	_messageBreak = "\n\n"
)

type (
	// Agent is one codex app-server process holding one thread. Use New.
	Agent struct {
		cmd      *exec.Cmd
		stdin    io.Writer
		stdout   *bufio.Reader
		stderr   tail
		threadID string
		nextID   int
		events   chan agent.Event
		done     chan struct{} // closed by Interrupt
		exited   chan struct{} // closed when run has returned and Events is closed
		writeMu  sync.Mutex    // serializes writes to stdin
		mu       sync.Mutex    // guards used, cmd and Interrupt
		used     bool
		inTurn   bool   // between turn/started and the turn's end; resume replays usage outside it
		msgID    string // agent message item the last text delta belonged to; owned by run
		wrote    bool   // the turn in progress has sent text; owned by run
	}

	// tail keeps the last _stderrKeep bytes written to it.
	tail struct{ b []byte }

	mcpServer struct {
		agent.MCPServer
		ApprovalMode string `json:"default_tools_approval_mode"`
	}

	message struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
		Params struct {
			Delta      string `json:"delta"`
			ItemID     string `json:"itemId"`
			Item       item   `json:"item"`
			TokenUsage struct {
				Last struct {
					InputTokens           int64 `json:"inputTokens"`
					CachedInputTokens     int64 `json:"cachedInputTokens"`
					CacheWriteInputTokens int64 `json:"cacheWriteInputTokens"`
					OutputTokens          int64 `json:"outputTokens"`
				} `json:"last"`
			} `json:"tokenUsage"`
			Turn struct {
				Status string `json:"status"`
				Error  *struct {
					Message string `json:"message"`
				} `json:"error"`
			} `json:"turn"`
		} `json:"params"`
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	item struct {
		ID               string          `json:"id"`
		Type             string          `json:"type"`
		Status           string          `json:"status"`
		Command          string          `json:"command"`
		AggregatedOutput string          `json:"aggregatedOutput"`
		ExitCode         *int            `json:"exitCode"`
		Changes          json.RawMessage `json:"changes"`
		Server           string          `json:"server"`
		Tool             string          `json:"tool"`
		Arguments        json.RawMessage `json:"arguments"`
		Result           *struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
)

// New returns an Agent that has not started.
func New() *Agent {
	return &Agent{
		events: make(chan agent.Event),
		done:   make(chan struct{}),
		exited: make(chan struct{}),
	}
}

// Start runs codex in cfg.Cwd on a new thread. cfg.PermissionMode is codex's sandbox mode
// (read-only, workspace-write, danger-full-access); approvals are always "never" because
// nobody can answer them. MCP servers are set on the thread, their tools pre-approved.
func (a *Agent) Start(ctx context.Context, cfg agent.Config) error {
	return a.launch(ctx, cfg, "thread/start", threadParams(cfg))
}

// Resume runs codex in cfg.Cwd on the thread with the given id.
func (a *Agent) Resume(ctx context.Context, cfg agent.Config, sessionID string) error {
	params := threadParams(cfg)
	params["threadId"] = sessionID
	return a.launch(ctx, cfg, "thread/resume", params)
}

// Send starts a turn on the thread.
func (a *Agent) Send(_ context.Context, prompt string) error {
	if a.threadID == "" {
		return errors.New("codex not started")
	}
	return a.request("turn/start", map[string]any{
		"threadId": a.threadID,
		"input":    []map[string]string{{"type": "text", "text": prompt}},
	})
}

// Events streams events until codex exits, then closes the channel.
func (a *Agent) Events() <-chan agent.Event { return a.events }

// Interrupt stops codex, waits for it, and closes Events. Codex gets SIGTERM so it can stop
// the commands it started, which run in their own process groups; SIGKILL follows after
// _killAfter.
func (a *Agent) Interrupt() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.interrupted() {
		return nil
	}
	a.used = true
	close(a.done)
	if a.cmd == nil {
		close(a.events)
		return nil
	}
	select {
	case <-a.exited:
		return nil
	default:
	}
	err := a.signal(syscall.SIGTERM)
	select {
	case <-a.exited:
	case <-time.After(_killAfter):
		err = errors.Join(err, a.signal(syscall.SIGKILL))
		<-a.exited
	}
	return err
}

func (a *Agent) launch(
	ctx context.Context, cfg agent.Config, method string, params map[string]any,
) error {
	ready, err := a.spawn(cfg, method, params)
	if err != nil {
		return err
	}
	stop := context.AfterFunc(ctx, func() { _ = a.Interrupt() })
	err = <-ready
	if !stop() || err != nil {
		_ = a.Interrupt()
		if ctx.Err() != nil {
			return fmt.Errorf("%s: %w", method, ctx.Err())
		}
		return err
	}
	return nil
}

// spawn starts the process and the goroutine that runs the handshake. It holds mu so that
// Interrupt sees either no process or a started one.
func (a *Agent) spawn(
	cfg agent.Config, method string, params map[string]any,
) (<-chan error, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.used {
		return nil, errors.New("codex agent already used")
	}
	a.used = true
	cmd := exec.Command("codex", "app-server")
	cmd.Dir = cfg.Cwd
	cmd.Env = append(os.Environ(), cfg.Env...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stderr = &a.stderr
	cmd.WaitDelay = _waitDelay
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start codex: %w", err)
	}
	a.cmd, a.stdin, a.stdout = cmd, stdin, bufio.NewReader(stdout)
	ready := make(chan error, 1)
	go a.run(method, params, ready)
	return ready, nil
}

// run owns stdout: it does the handshake, streams events, then reaps the process.
func (a *Agent) run(method string, params map[string]any, ready chan<- error) {
	defer close(a.exited)
	defer close(a.events)
	err := a.handshake(method, params)
	ready <- err
	if err != nil {
		_ = a.cmd.Wait()
		return
	}
	a.read()
	// Wait closes stdout, so it must come after the last read.
	err = a.cmd.Wait()
	if err == nil || a.interrupted() {
		return
	}
	a.emit(agent.Event{
		Kind: agent.KindError,
		Text: fmt.Sprintf("codex exited: %v: %s", err, strings.TrimSpace(string(a.stderr.b))),
	})
}

func (a *Agent) handshake(method string, params map[string]any) error {
	clientInfo := map[string]any{"name": "reef", "title": nil, "version": "0"}
	if _, err := a.call("initialize", map[string]any{"clientInfo": clientInfo}); err != nil {
		return fmt.Errorf("initialize: %w", err)
	}
	if err := a.write(map[string]any{"method": "initialized"}); err != nil {
		return err
	}
	res, err := a.call(method, params)
	if err != nil {
		return fmt.Errorf("%s: %w", method, err)
	}
	var thread struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if err := json.Unmarshal(res, &thread); err != nil {
		return fmt.Errorf("%s result: %w", method, err)
	}
	a.threadID = thread.Thread.ID
	return nil
}

func (a *Agent) call(method string, params any) (json.RawMessage, error) {
	if err := a.request(method, params); err != nil {
		return nil, err
	}
	want := strconv.Itoa(a.nextID)
	for {
		line, err := a.stdout.ReadBytes('\n')
		if err != nil {
			return nil, err
		}
		var m message
		if json.Unmarshal(line, &m) != nil || string(m.ID) != want || m.Method != "" {
			continue
		}
		if m.Error != nil {
			return nil, errors.New(m.Error.Message)
		}
		return m.Result, nil
	}
}

func (a *Agent) request(method string, params any) error {
	a.nextID++
	return a.write(map[string]any{"id": a.nextID, "method": method, "params": params})
}

func (a *Agent) write(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	a.writeMu.Lock()
	defer a.writeMu.Unlock()
	_, err = a.stdin.Write(append(b, '\n'))
	return err
}

func (a *Agent) read() {
	if !a.emit(agent.Event{Kind: agent.KindSession, SessionID: a.threadID}) {
		return
	}
	for {
		line, err := a.stdout.ReadBytes('\n')
		var m message
		if json.Unmarshal(line, &m) == nil {
			if m.Method != "" && len(m.ID) > 0 {
				// Codex asks the client something; nobody is there to answer.
				_ = a.write(map[string]any{
					"id":    m.ID,
					"error": map[string]any{"code": -32601, "message": "unsupported"},
				})
			} else if !a.handle(m) {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

// handle emits the event for m and reports whether to keep reading. Codex reports each
// model call; each report is emitted as it arrives, so an interrupted turn keeps its usage.
// A break goes before the text of a second agent message in a turn.
func (a *Agent) handle(m message) bool {
	switch m.Method {
	case "turn/started":
		a.inTurn, a.msgID, a.wrote = true, "", false
	case "thread/tokenUsage/updated":
		if ev, _ := m.event(); a.inTurn {
			return a.emit(ev)
		}
		return true
	case "item/agentMessage/delta":
		if a.wrote && m.Params.ItemID != a.msgID &&
			!a.emit(agent.Event{Kind: agent.KindText, Text: _messageBreak}) {
			return false
		}
		a.msgID, a.wrote = m.Params.ItemID, true
	}
	ev, ok := m.event()
	if !ok {
		return true
	}
	if ev.Kind == agent.KindTurnEnd || ev.Kind == agent.KindError {
		a.inTurn = false
	}
	return a.emit(ev)
}

func (a *Agent) emit(ev agent.Event) bool {
	select {
	case a.events <- ev:
		return true
	case <-a.done:
		return false
	}
}

func (a *Agent) interrupted() bool {
	select {
	case <-a.done:
		return true
	default:
		return false
	}
}

func (a *Agent) signal(sig syscall.Signal) error {
	err := syscall.Kill(-a.cmd.Process.Pid, sig)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}

func (t *tail) Write(p []byte) (int, error) {
	t.b = append(t.b, p...)
	if len(t.b) > _stderrKeep {
		t.b = t.b[len(t.b)-_stderrKeep:]
	}
	return len(p), nil
}

func (m message) event() (ev agent.Event, ok bool) {
	p := m.Params
	switch {
	case m.Method == "" && m.Error != nil:
		return agent.Event{Kind: agent.KindError, Text: m.Error.Message}, true
	case m.Method == "item/agentMessage/delta":
		return agent.Event{Kind: agent.KindText, Text: p.Delta}, true
	case m.Method == "item/started":
		return p.Item.toolCall()
	case m.Method == "item/completed":
		return p.Item.toolResult()
	case m.Method == "thread/tokenUsage/updated":
		u := p.TokenUsage.Last
		return agent.Event{Kind: agent.KindUsage, Usage: &agent.Usage{
			InputTokens:      u.InputTokens - u.CachedInputTokens - u.CacheWriteInputTokens,
			OutputTokens:     u.OutputTokens,
			CacheReadTokens:  u.CachedInputTokens,
			CacheWriteTokens: u.CacheWriteInputTokens,
		}}, true
	case m.Method == "turn/completed" && p.Turn.Status == "failed":
		msg := "turn failed"
		if p.Turn.Error != nil {
			msg = p.Turn.Error.Message
		}
		return agent.Event{Kind: agent.KindError, Text: msg}, true
	case m.Method == "turn/completed":
		return agent.Event{Kind: agent.KindTurnEnd}, true
	}
	return agent.Event{}, false
}

func (it item) toolCall() (agent.Event, bool) {
	ev := agent.Event{Kind: agent.KindToolCall, ToolID: it.ID, ToolName: it.Type}
	switch it.Type {
	case "commandExecution":
		ev.Input, _ = json.Marshal(map[string]string{"command": it.Command})
	case "fileChange":
		ev.Input = it.Changes
	case "mcpToolCall":
		ev.ToolName = "mcp__" + it.Server + "__" + it.Tool
		ev.Input = it.Arguments
	default:
		return agent.Event{}, false
	}
	return ev, true
}

func (it item) toolResult() (agent.Event, bool) {
	ev := agent.Event{Kind: agent.KindToolResult, ToolID: it.ID, IsError: it.Status != "completed"}
	switch it.Type {
	case "commandExecution":
		ev.Output = it.AggregatedOutput
		ev.IsError = ev.IsError || (it.ExitCode != nil && *it.ExitCode != 0)
	case "fileChange":
		ev.Output = it.Status
	case "mcpToolCall":
		switch {
		case it.Error != nil:
			ev.Output = it.Error.Message
		case it.Result != nil:
			texts := make([]string, 0, len(it.Result.Content))
			for _, c := range it.Result.Content {
				texts = append(texts, c.Text)
			}
			ev.Output = strings.Join(texts, "\n")
		}
	default:
		return agent.Event{}, false
	}
	return ev, true
}

func threadParams(cfg agent.Config) map[string]any {
	params := map[string]any{"cwd": cfg.Cwd, "approvalPolicy": "never"}
	if cfg.PermissionMode != "" {
		params["sandbox"] = cfg.PermissionMode
	}
	if len(cfg.MCPServers) > 0 {
		servers := make(map[string]mcpServer, len(cfg.MCPServers))
		for name, s := range cfg.MCPServers {
			servers[name] = mcpServer{MCPServer: s, ApprovalMode: "approve"}
		}
		params["config"] = map[string]any{"mcp_servers": servers}
	}
	return params
}
