package claude

import (
	"context"
	"os"
	"path/filepath"

	"github.com/bhanuprakaash/reef/agent"
)

// _approveProject lets `claude mcp login` use the folder's .mcp.json server without the
// interactive approval a project server otherwise waits for.
const _approveProject = `{"enableAllProjectMcpServers":true}`

// Login signs the user in to the HTTP server s, named name, and returns what claude printed. claude
// keeps the token under the server's name, url and headers, so a session given the same server
// finds it. `claude mcp login` knows only configured servers and needs a terminal: it runs under
// one, from a temporary folder whose .mcp.json holds just this server, opens the browser and
// returns once the user finishes. bin "" is the claude on PATH.
func Login(ctx context.Context, bin, name string, s agent.MCPServer) (string, error) {
	if bin == "" {
		bin = "claude"
	}
	dir, err := os.MkdirTemp("", "reef-login-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	b, err := mcpJSON(map[string]agent.MCPServer{name: s})
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, ".mcp.json"), b, 0o600); err != nil {
		return "", err
	}
	cmd := agent.InTerminal(ctx, bin, "--settings", _approveProject, "mcp", "login", name)
	cmd.Dir = dir
	return agent.RunTerminal(cmd)
}
