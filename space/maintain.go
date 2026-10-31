package space

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/bhanuprakaash/reef/agent"
	"github.com/bhanuprakaash/reef/skill"
	"github.com/bhanuprakaash/reef/store"
)

// Maintain runs a turn in which Bot botID reviews its own memory, recorded in Space spaceID. It
// shares nothing with the Space's Session: the Bot gets a new backend context with its current
// backend and model, cannot write files, has no connections or skills, only the reef MCP server
// with a review token, and a folder of its own to work in. On claude it also runs no commands; on
// codex a read-only sandbox still runs them. The turn keeps no wake and
// posts no message.
func Maintain(
	ctx context.Context, st *store.Store, dataDir, exe string, spaceID, botID int64,
	mk func(backend string) (agent.Agent, error), onEvent func(agent.Event),
) error {
	wctx := context.WithoutCancel(ctx)
	team, err := st.SpaceTeam(wctx, spaceID)
	if err != nil {
		return err
	}
	var r store.Role
	for _, member := range team.Roles {
		if member.BotID == botID {
			r = member
			break
		}
	}
	if r.BotID == 0 {
		return fmt.Errorf("%w: Bot %d is not a Space member", store.ErrInvalid, botID)
	}
	r.Write, r.PermissionMode, r.Tools, r.Skills, r.Connections = false, "", nil, nil, nil
	cwd := filepath.Join(dataDir, "reviews", strconv.FormatInt(botID, 10))
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		return fmt.Errorf("make folder of memory review: %w", err)
	}
	ag, err := mk(r.Backend)
	if err != nil {
		return err
	}
	m := &member{ag: ag}
	defer func() {
		_ = m.close(st)
	}()
	tok, err := st.CreateReviewToken(wctx, spaceID, botID)
	if err != nil {
		return err
	}
	m.token = tok.Token
	cfg := roleConfig(r, cwd, nil)
	cfg.DisallowedTools = append(cfg.DisallowedTools, "Bash")
	if r.Backend == "claude" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("find home folder to list skills: %w", err)
		}
		for _, s := range skill.Claude(home, cwd) {
			cfg.DisallowedTools = append(cfg.DisallowedTools, "Skill("+s.Name+")")
		}
	}
	cfg.MCPServers = map[string]agent.MCPServer{"reef": {
		Command: exe,
		Args:    []string{"mcp", "--data", dataDir},
		Env:     map[string]string{TokenEnv: tok.Token},
	}}
	if cfg.Bin, err = st.RoleBin(wctx, r); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ag.Start(ctx, cfg); err != nil {
		return err
	}
	run := NewRunner(st, ag, spaceID, r.Name, r.Backend, "")
	run.botID = &botID
	run.NoWake, run.NoReply = true, true
	return run.Run(ctx, _maintainPrompt, nil, onEvent)
}
