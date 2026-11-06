package codex

import (
	"context"
	"encoding/json"

	"github.com/bhanuprakaash/reef/agent"
)

// Login signs the user in to the HTTP server s, named name, and returns what codex printed. codex
// keeps the token under the server's name and url, so a session given the same server finds it.
// The server is passed with -c, so the user's config.toml is not changed. It runs under a
// terminal, opens the browser and returns once the user finishes. bin "" is the codex on PATH.
func Login(ctx context.Context, bin, name string, s agent.MCPServer) (string, error) {
	if bin == "" {
		bin = "codex"
	}
	key := "mcp_servers." + name + "."
	args := []string{"mcp", "login", name, "-c", key + "url=" + tomlString(s.URL)}
	if s.OAuth != nil && s.OAuth.ClientID != "" {
		args = append(args, "-c", key+"oauth.client_id="+tomlString(s.OAuth.ClientID))
	}
	return agent.RunTerminal(agent.InTerminal(ctx, bin, args...))
}

// tomlString quotes v as a TOML basic string; a JSON string is one.
func tomlString(v string) string {
	b, _ := json.Marshal(v)
	return string(b)
}
