package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/bhanuprakaash/reef/server"
	"github.com/bhanuprakaash/reef/space"
	"github.com/bhanuprakaash/reef/store"
)

const (
	_mcpUsage   = "usage: reef mcp [--data DIR]"
	_tokenUsage = "usage: reef token <feature> --role R [--data DIR]"
)

// mcpCmd must write nothing but MCP to stdout.
func mcpCmd(ctx context.Context, args []string) error {
	value := os.Getenv(space.TokenEnv)
	if value == "" {
		return errors.New(space.TokenEnv + " is not set")
	}
	c := newCommand("reef mcp", _mcpUsage, 0, 0)
	st, _, err := c.open(args)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	dir, err := dataDir(*c.data)
	if err != nil {
		return err
	}
	tok, err := st.Token(ctx, value)
	if errors.Is(err, store.ErrNotFound) {
		return errors.New("unknown " + space.TokenEnv)
	}
	if err != nil {
		return err
	}
	ms, err := openMemory(ctx, st)
	if err != nil {
		return err
	}
	err = server.NewMCP(st, ms, tok, dir).Run(ctx, &mcp.StdioTransport{})
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

// tokenCmd makes a Space on the feature for role and a token for it, and
// prints the command that registers the MCP server with Claude Code.
func tokenCmd(ctx context.Context, args []string, stdout io.Writer) error {
	c := newCommand("reef token", _tokenUsage, 1, 1)
	role := c.fs.String("role", "", "")
	st, pos, err := c.open(args)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()
	if *role == "" {
		return errors.New(_tokenUsage)
	}

	f, err := st.FeatureByRef(ctx, pos[0])
	if err != nil {
		return err
	}
	ch, err := st.CreateSpace(ctx, store.Space{
		ProjectID: f.ProjectID, FeatureID: &f.ID, Name: *role + " (claude code)",
		Role: "lead", Backend: "claude",
	})
	if err != nil {
		return err
	}
	tok, err := st.CreateToken(ctx, ch.ID, *role)
	if err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find reef path: %w", err)
	}
	dir, err := dataDir(*c.data)
	if err != nil {
		return err
	}
	if dir, err = filepath.Abs(dir); err != nil {
		return fmt.Errorf("resolve data dir: %w", err)
	}
	_, err = fmt.Fprintf(stdout, "claude mcp add reef --env %s=%s -- %s mcp --data %s\n",
		space.TokenEnv, tok.Token, shellQuote(exe), shellQuote(dir))
	return err
}

func shellQuote(s string) string {
	if !strings.ContainsAny(s, " \t\n\"'\\$`!*?[]{}()<>|&;#~") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
