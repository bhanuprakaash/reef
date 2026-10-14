package main

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"time"
)

type mcpConfigFile struct {
	MCPServers map[string]struct {
		Command string            `json:"command"`
		Args    []string          `json:"args"`
		Env     map[string]string `json:"env"`
	} `json:"mcpServers"`
}

// callMCPTool runs server's command from mcpConfig and makes one tools/call of tool with
// input as its arguments, the handshake an MCP client does. It does nothing when mcpConfig
// or the server is missing: a tool call recorded against a server the current run has none
// of is not a side effect this run can reproduce.
func callMCPTool(mcpConfig, server, tool string, input json.RawMessage) {
	if mcpConfig == "" {
		return
	}
	b, err := os.ReadFile(mcpConfig)
	if err != nil {
		return
	}
	var cfg mcpConfigFile
	if json.Unmarshal(b, &cfg) != nil {
		return
	}
	entry, ok := cfg.MCPServers[server]
	if !ok {
		return
	}

	cmd := exec.Command(entry.Command, entry.Args...)
	cmd.Env = os.Environ()
	for k, v := range entry.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	if cmd.Start() != nil {
		return
	}
	defer func() {
		_ = stdin.Close()
		done := make(chan struct{})
		go func() { _ = cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
		}
	}()

	r := bufio.NewReader(stdout)
	send := func(id int, method string, params any) {
		msg := map[string]any{"jsonrpc": "2.0", "method": method}
		if id != 0 {
			msg["id"] = id
		}
		if params != nil {
			msg["params"] = params
		}
		stdin.Write(marshalLine(msg))
		stdin.Write([]byte("\n"))
	}
	reply := func(id int) bool {
		for {
			line, err := r.ReadBytes('\n')
			if len(line) > 0 {
				var m struct {
					ID int `json:"id"`
				}
				if json.Unmarshal(line, &m) == nil && m.ID == id {
					return true
				}
			}
			if err != nil {
				return false
			}
		}
	}

	send(1, "initialize", map[string]any{
		"protocolVersion": "2025-06-18", "capabilities": map[string]any{},
		"clientInfo": map[string]any{"name": "fake-claude", "version": "0"},
	})
	if !reply(1) {
		return
	}
	send(0, "notifications/initialized", nil)
	var args any
	_ = json.Unmarshal(input, &args)
	send(2, "tools/call", map[string]any{"name": tool, "arguments": args})
	reply(2)
}
