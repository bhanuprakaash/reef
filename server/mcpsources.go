package server

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/store"
)

// foundMCP is an MCP server the user already has in Claude Code or Codex, read and never written.
// Conn carries its settings and no name.
type foundMCP struct {
	Source string
	Name   string
	Conn   store.Connection
}

// claudeMCP is one entry of a Claude Code mcpServers map.
type claudeMCP struct {
	Type    string            `json:"type"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	OAuth   *agent.OAuth      `json:"oauth"`
}

// codexMCPTimeout limits `codex mcp list`, so a hung codex does not hang the request.
const codexMCPTimeout = 10 * time.Second

// findMCPServers lists the servers claude loads for the user (~/.claude.json), for the folder
// projectPath (its entry in ~/.claude.json and its .mcp.json) and from the enabled plugins, and
// the servers codex loads. The Claude lists never include a type Reef cannot run (sse, ws). Codex
// is asked through `codex mcp list --json`, which knows CODEX_HOME and the project's own config
// and saves reading TOML here.
func findMCPServers(ctx context.Context, home, projectPath string) []foundMCP {
	var found []foundMCP
	add := func(source string, servers map[string]claudeMCP) {
		for _, name := range slices.Sorted(maps.Keys(servers)) {
			c, ok := servers[name].connection()
			seen := slices.ContainsFunc(found, func(f foundMCP) bool { return f.Source == source && f.Name == name })
			if ok && !seen {
				found = append(found, foundMCP{Source: source, Name: name, Conn: c})
			}
		}
	}
	var user struct {
		MCPServers json.RawMessage `json:"mcpServers"`
		Projects   map[string]struct {
			MCPServers json.RawMessage `json:"mcpServers"`
		} `json:"projects"`
	}
	readJSONFile(filepath.Join(home, ".claude.json"), &user)
	add("claude user", parseClaudeMCP(user.MCPServers))
	if projectPath != "" {
		add("claude project", parseClaudeMCP(user.Projects[projectPath].MCPServers))
		if body, err := os.ReadFile(filepath.Join(projectPath, ".mcp.json")); err == nil {
			add("claude project", parseClaudeMCP(body))
		}
	}
	for _, p := range enabledPlugins(home, projectPath) {
		add("claude plugin "+p.name, p.servers())
	}
	return append(found, codexMCP(ctx)...)
}

func (s claudeMCP) connection() (store.Connection, bool) {
	if s.Type != "" && s.Type != "stdio" && s.Type != "http" {
		return store.Connection{}, false
	}
	c := store.Connection{
		Command: s.Command, Args: s.Args, Env: s.Env, URL: s.URL, Headers: s.Headers, OAuth: s.OAuth,
	}
	return c, (c.Command == "") != (c.URL == "")
}

// parseClaudeMCP reads either {"mcpServers":{name:{...}}} or the bare {name:{...}}, both of
// which plugins ship.
func parseClaudeMCP(body []byte) map[string]claudeMCP {
	var top map[string]json.RawMessage
	if json.Unmarshal(body, &top) != nil {
		return nil
	}
	if inner, ok := top["mcpServers"]; ok {
		top = nil
		if json.Unmarshal(inner, &top) != nil {
			return nil
		}
	}
	servers := make(map[string]claudeMCP, len(top))
	for name, raw := range top {
		var s claudeMCP
		if json.Unmarshal(raw, &s) == nil {
			servers[name] = s
		}
	}
	return servers
}

type installedPlugin struct {
	name string
	dir  string
}

// enabledPlugins lists the plugins ~/.claude/settings.json enables, installed for the user or for
// projectPath, in the folders installed_plugins.json gives.
func enabledPlugins(home, projectPath string) []installedPlugin {
	var settings struct {
		EnabledPlugins map[string]bool `json:"enabledPlugins"`
	}
	var installed struct {
		Plugins map[string][]struct {
			Scope       string `json:"scope"`
			ProjectPath string `json:"projectPath"`
			InstallPath string `json:"installPath"`
		} `json:"plugins"`
	}
	readJSONFile(filepath.Join(home, ".claude", "settings.json"), &settings)
	readJSONFile(filepath.Join(home, ".claude", "plugins", "installed_plugins.json"), &installed)
	var out []installedPlugin
	for _, key := range slices.Sorted(maps.Keys(installed.Plugins)) {
		if !settings.EnabledPlugins[key] {
			continue
		}
		name, _, _ := strings.Cut(key, "@")
		for _, e := range installed.Plugins[key] {
			if e.Scope == "user" || (projectPath != "" && filepath.Clean(e.ProjectPath) == filepath.Clean(projectPath)) {
				out = append(out, installedPlugin{name: name, dir: e.InstallPath})
			}
		}
	}
	return out
}

// servers reads the plugin's .mcp.json, then the mcpServers of its plugin.json.
func (p installedPlugin) servers() map[string]claudeMCP {
	out := map[string]claudeMCP{}
	var manifest struct {
		MCPServers json.RawMessage `json:"mcpServers"`
	}
	readJSONFile(filepath.Join(p.dir, ".claude-plugin", "plugin.json"), &manifest)
	maps.Copy(out, parseClaudeMCP(manifest.MCPServers))
	if body, err := os.ReadFile(filepath.Join(p.dir, ".mcp.json")); err == nil {
		maps.Copy(out, parseClaudeMCP(body))
	}
	return out
}

// codexMCP lists the enabled servers of `codex mcp list --json`. Without codex, or when it fails,
// there are none.
func codexMCP(ctx context.Context) []foundMCP {
	ctx, cancel := context.WithTimeout(ctx, codexMCPTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "codex", "mcp", "list", "--json").Output()
	if err != nil {
		if !errors.Is(err, exec.ErrNotFound) {
			log.Printf("skip codex mcp servers: %v", err)
		}
		return nil
	}
	var listed []struct {
		Name      string `json:"name"`
		Enabled   bool   `json:"enabled"`
		Transport struct {
			Command     string            `json:"command"`
			Args        []string          `json:"args"`
			Env         map[string]string `json:"env"`
			URL         string            `json:"url"`
			HTTPHeaders map[string]string `json:"http_headers"`
		} `json:"transport"`
	}
	if err := json.Unmarshal(out, &listed); err != nil {
		log.Printf("skip codex mcp servers: %v", err)
		return nil
	}
	var found []foundMCP
	for _, l := range listed {
		t := l.Transport
		if l.Enabled {
			found = append(found, foundMCP{Source: "codex", Name: l.Name, Conn: store.Connection{
				Command: t.Command, Args: t.Args, Env: t.Env, URL: t.URL, Headers: t.HTTPHeaders,
			}})
		}
	}
	return found
}

// readJSONFile decodes the file into v; a file that is missing or bad leaves v as it was.
func readJSONFile(path string, v any) {
	body, err := os.ReadFile(path)
	if err != nil {
		return
	}
	if err := json.Unmarshal(body, v); err != nil {
		log.Printf("skip %s: %v", path, err)
	}
}
