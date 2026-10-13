// Package claude is the Claude Code backend: one long-lived `claude -p` process speaking
// stream-json on stdin and stdout.
package claude

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/bhanuprakaash/reef/agent"
)

var _ agent.Agent = (*Agent)(nil)

const (
	// SIGTERM lets claude stop the tool processes it started; they are not in its group.
	_termGrace = time.Second
	// WaitDelay bounds Wait when a process that outlived claude holds its stderr pipe.
	_waitDelay = 2 * time.Second
	_stderrMax = 4 << 10
)

// Agent drives one claude process. Use one Agent per session.
type Agent struct {
	cmd     *exec.Cmd
	stdin   io.Writer
	stderr  bytes.Buffer
	events  chan agent.Event
	quit    chan struct{}
	done    chan struct{}
	mcpPath string
	mu      sync.Mutex // guards started, stopped and the fields start sets
	started bool
	stopped bool
}

type (
	mcpFile struct {
		MCPServers map[string]mcpEntry `json:"mcpServers"`
	}

	mcpEntry struct {
		agent.MCPServer
		Type string `json:"type,omitempty"`
	}

	inputMessage struct {
		Type    string       `json:"type"`
		Message inputContent `json:"message"`
	}

	inputContent struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}

	outputLine struct {
		Type      string `json:"type"`
		Subtype   string `json:"subtype"`
		SessionID string `json:"session_id"`
		Event     struct {
			Type  string `json:"type"`
			Delta struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"delta"`
		} `json:"event"`
		Message struct {
			Content []block `json:"content"`
		} `json:"message"`
		IsError bool     `json:"is_error"`
		Result  string   `json:"result"`
		Errors  []string `json:"errors"`
		Usage   struct {
			InputTokens      int64 `json:"input_tokens"`
			OutputTokens     int64 `json:"output_tokens"`
			CacheReadTokens  int64 `json:"cache_read_input_tokens"`
			CacheWriteTokens int64 `json:"cache_creation_input_tokens"`
		} `json:"usage"`
	}

	block struct {
		Type      string          `json:"type"`
		ID        string          `json:"id"`
		Name      string          `json:"name"`
		Input     json.RawMessage `json:"input"`
		ToolUseID string          `json:"tool_use_id"`
		Content   json.RawMessage `json:"content"`
		IsError   bool            `json:"is_error"`
	}
)

// New returns an Agent that has not started. Its Events channel is valid at once.
func New() *Agent {
	return &Agent{
		events: make(chan agent.Event),
		quit:   make(chan struct{}),
		done:   make(chan struct{}),
	}
}

// Start launches claude in cfg.Cwd on a new session.
func (a *Agent) Start(_ context.Context, cfg agent.Config) error {
	return a.start(cfg)
}

// Resume launches claude in cfg.Cwd continuing the session with the given id.
func (a *Agent) Resume(_ context.Context, cfg agent.Config, sessionID string) error {
	return a.start(cfg, "--resume", sessionID)
}

// Send writes the prompt to claude as one user message.
func (a *Agent) Send(_ context.Context, prompt string) error {
	if a.stdin == nil {
		return errors.New("claude not started")
	}
	msg, err := json.Marshal(inputMessage{
		Type:    "user",
		Message: inputContent{Role: "user", Content: prompt},
	})
	if err != nil {
		return err
	}
	if _, err := a.stdin.Write(append(msg, '\n')); err != nil {
		return fmt.Errorf("write prompt: %w", err)
	}
	return nil
}

// Events streams events until claude exits, then closes the channel.
func (a *Agent) Events() <-chan agent.Event { return a.events }

// Interrupt kills claude and anything it spawned, and waits until Events is closed.
func (a *Agent) Interrupt() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.stopped {
		return nil
	}
	a.stopped = true
	close(a.quit)
	if a.cmd == nil {
		close(a.events)
		return nil
	}
	select {
	case <-a.done:
		return nil
	default:
	}
	_ = syscall.Kill(-a.cmd.Process.Pid, syscall.SIGTERM)
	select {
	case <-a.done:
	case <-time.After(_termGrace):
		_ = syscall.Kill(-a.cmd.Process.Pid, syscall.SIGKILL)
		<-a.done
	}
	return nil
}

func (a *Agent) start(cfg agent.Config, extra ...string) (err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.started || a.stopped {
		return errors.New("claude already started or interrupted")
	}
	a.started = true

	args := []string{
		"-p",
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"--verbose",
		"--include-partial-messages",
	}
	if cfg.PermissionMode != "" {
		args = append(args, "--permission-mode", cfg.PermissionMode)
	}
	if len(cfg.MCPServers) > 0 {
		path, werr := writeMCPConfig(cfg.MCPServers)
		if werr != nil {
			return werr
		}
		defer func() {
			if err != nil {
				_ = os.Remove(path)
			}
		}()
		args = append(args, "--mcp-config", path)
		a.mcpPath = path
	}
	args = append(args, extra...)

	cmd := exec.Command("claude", args...)
	cmd.Dir = cfg.Cwd
	cmd.Env = append(os.Environ(), cfg.Env...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = _waitDelay
	cmd.Stderr = &a.stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start claude: %w", err)
	}

	a.cmd, a.stdin = cmd, stdin
	go a.read(stdout)
	return nil
}

func (a *Agent) read(stdout io.Reader) {
	defer close(a.done)
	defer close(a.events)
	defer func() {
		if a.mcpPath != "" {
			_ = os.Remove(a.mcpPath)
		}
	}()

	r := bufio.NewReader(stdout)
	var last agent.Kind
	for {
		line, err := r.ReadBytes('\n')
		for _, ev := range parse(line) {
			if !a.emit(ev) {
				_ = a.cmd.Wait()
				return
			}
			last = ev.Kind
		}
		if err != nil {
			break
		}
	}
	err := a.cmd.Wait()
	if err == nil || last == agent.KindError {
		return
	}
	stderr := a.stderr.Bytes()
	if len(stderr) > _stderrMax {
		stderr = stderr[len(stderr)-_stderrMax:]
	}
	a.emit(agent.Event{
		Kind: agent.KindError,
		Text: fmt.Sprintf("claude exited: %v: %s", err, bytes.TrimSpace(stderr)),
	})
}

// emit reports whether ev was delivered. Once Interrupt has run it delivers nothing.
func (a *Agent) emit(ev agent.Event) bool {
	select {
	case <-a.quit:
		return false
	default:
	}
	select {
	case a.events <- ev:
		return true
	case <-a.quit:
		return false
	}
}

func writeMCPConfig(servers map[string]agent.MCPServer) (string, error) {
	entries := make(map[string]mcpEntry, len(servers))
	for name, s := range servers {
		e := mcpEntry{MCPServer: s}
		if s.URL != "" {
			e.Type = "http"
		}
		entries[name] = e
	}
	b, err := json.Marshal(mcpFile{MCPServers: entries})
	if err != nil {
		return "", err
	}
	f, err := os.CreateTemp("", "reef-mcp-*.json")
	if err != nil {
		return "", err
	}
	_, err = f.Write(b)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// parse maps one stdout line to Events. Text comes from the partial-message deltas, so
// the complete assistant text blocks that follow are skipped. Unknown lines give none.
func parse(line []byte) []agent.Event {
	var l outputLine
	if err := json.Unmarshal(line, &l); err != nil {
		return nil
	}
	switch l.Type {
	case "system":
		if l.Subtype == "init" {
			return []agent.Event{{Kind: agent.KindSession, SessionID: l.SessionID}}
		}
	case "stream_event":
		if l.Event.Type == "content_block_delta" && l.Event.Delta.Type == "text_delta" {
			return []agent.Event{{Kind: agent.KindText, Text: l.Event.Delta.Text}}
		}
	case "assistant":
		var evs []agent.Event
		for _, b := range l.Message.Content {
			if b.Type == "tool_use" {
				evs = append(evs, agent.Event{
					Kind: agent.KindToolCall, ToolID: b.ID, ToolName: b.Name, Input: b.Input,
				})
			}
		}
		return evs
	case "user":
		var evs []agent.Event
		for _, b := range l.Message.Content {
			if b.Type == "tool_result" {
				evs = append(evs, agent.Event{
					Kind: agent.KindToolResult, ToolID: b.ToolUseID,
					Output: resultText(b.Content), IsError: b.IsError,
				})
			}
		}
		return evs
	case "result":
		usage := &agent.Usage{
			InputTokens:      l.Usage.InputTokens,
			OutputTokens:     l.Usage.OutputTokens,
			CacheReadTokens:  l.Usage.CacheReadTokens,
			CacheWriteTokens: l.Usage.CacheWriteTokens,
		}
		end := agent.Event{Kind: agent.KindTurnEnd}
		if l.IsError {
			text := strings.Join(append([]string{l.Result}, l.Errors...), "\n")
			end = agent.Event{Kind: agent.KindError, Text: strings.TrimSpace(text)}
		}
		return []agent.Event{{Kind: agent.KindUsage, Usage: usage}, end}
	}
	return nil
}

func resultText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		parts = append(parts, b.Text)
	}
	return strings.Join(parts, "\n")
}
