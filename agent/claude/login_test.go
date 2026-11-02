package claude

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bhanuprakaash/reef/agent"
)

func TestLogin_SignsInTheSessionsServer(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "claude")
	seen := filepath.Join(dir, "seen")
	script := "#!/bin/sh\n" +
		`printf '%s\n' "$*" > ` + seen + "\n" +
		`cat .mcp.json >> ` + seen + "\n" +
		`[ -t 0 ] && echo "Authentication successful" || { echo "stdin isn't a terminal"; exit 1; }` + "\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	srv := agent.MCPServer{URL: "https://mcp.slack.com/mcp", OAuth: &agent.OAuth{ClientID: "id1", CallbackPort: 3118}}

	out, err := Login(context.Background(), bin, "slack", srv)
	if err != nil {
		t.Fatalf("Login: %v (%s)", err, out)
	}
	if out != "Authentication successful" {
		t.Errorf("output = %q, want the terminal's text", out)
	}
	got, err := os.ReadFile(seen)
	if err != nil {
		t.Fatal(err)
	}
	session, err := mcpJSON(map[string]agent.MCPServer{"slack": srv})
	if err != nil {
		t.Fatal(err)
	}
	want := `--settings {"enableAllProjectMcpServers":true} mcp login slack` + "\n" + string(session)
	if strings.TrimSpace(string(got)) != want {
		t.Errorf("claude got\n%s\nwant\n%s", got, want)
	}
}
