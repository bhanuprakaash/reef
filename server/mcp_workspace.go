package server

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/bhanuprakaash/reef/space"
	"github.com/bhanuprakaash/reef/store"
)

type (
	openWorkspaceIn struct {
		Project int64  `json:"project" jsonschema:"id of the Project whose repository to change, from list_projects"`
		Fix     string `json:"fix,omitempty" jsonschema:"the fix to work in: a name this conversation already uses to continue it, or a new name for independent work; omit it only when this conversation has no open fix in the Project"`
	}
	openWorkspaceOut struct {
		ID         int64  `json:"id"`
		Fix        string `json:"fix"`
		Dir        string `json:"dir" jsonschema:"the folder to change, inside your working folder"`
		Branch     string `json:"branch"`
		Base       string `json:"base"`
		BaseSource string `json:"baseSource" jsonschema:"origin when the base branch was refreshed from origin, local when it was not"`
		State      string `json:"state" jsonschema:"ready, or paused with the reason in note"`
		Note       string `json:"note,omitempty"`
	}
)

// registerOpenWorkspace adds open_workspace to the tools of a Space that has no work item.
func (m *mcpServer) registerOpenWorkspace(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "open_workspace",
		Description: "Open an isolated working tree of a Project's repository to implement and verify a " +
			"fix from this conversation, with no work item. It starts on a new branch from the " +
			"Project's base branch. Pass the fix name of earlier work here to continue it, or a new " +
			"name for independent work. With no name while a fix is open in that Project it is " +
			"refused: ask the user whether to continue it or start a new fix. Change files only " +
			"under the dir returned.",
	}, m.openWorkspace)
}

func (m *mcpServer) openWorkspace(
	ctx context.Context, _ *mcp.CallToolRequest, in openWorkspaceIn,
) (*mcp.CallToolResult, openWorkspaceOut, error) {
	if _, err := m.currentName(ctx); err != nil {
		return nil, openWorkspaceOut{}, err
	}
	sp, err := m.st.Space(ctx, m.tok.SpaceID)
	if err != nil {
		return nil, openWorkspaceOut{}, err
	}
	if sp.WorkItemID != nil {
		return nil, openWorkspaceOut{}, fmt.Errorf("%w: this Space belongs to a work item and already has a workspace per target", store.ErrConflict)
	}
	p, err := m.st.Project(ctx, in.Project)
	if err != nil {
		return nil, openWorkspaceOut{}, err
	}
	if !p.Active() {
		return nil, openWorkspaceOut{}, fmt.Errorf("%w: project %s is deleted", store.ErrInvalid, p.Name)
	}
	rows, err := m.st.SpaceWorkspaces(ctx, sp.ID)
	if err != nil {
		return nil, openWorkspaceOut{}, err
	}
	var open []string
	var all int
	for _, w := range rows {
		if w.ProjectID != p.ID {
			continue
		}
		all++
		if w.State != store.WorkspaceRemoved {
			open = append(open, w.Name)
		}
	}
	fix := space.FixName(in.Fix)
	switch {
	case in.Fix != "" && fix == "":
		return nil, openWorkspaceOut{}, fmt.Errorf("%w: fix %q has no letters or digits", store.ErrInvalid, in.Fix)
	case fix == "" && len(open) > 0:
		return nil, openWorkspaceOut{}, fmt.Errorf("%w: this conversation has open fixes in %s: %s. "+
			"Ask the user: continue one of them, or start a new fix?", store.ErrInvalid, p.Name, strings.Join(open, ", "))
	case fix == "":
		fix = fmt.Sprintf("fix-%d", all+1)
	}
	for _, w := range rows {
		if w.ProjectID != p.ID || w.Name != fix {
			continue
		}
		if w.State == store.WorkspaceRemoved {
			return nil, openWorkspaceOut{}, fmt.Errorf("%w: fix %s was abandoned; choose a new name", store.ErrConflict, fix)
		}
	}
	w, err := space.EnsureFixWorkspace(ctx, m.st, space.NewTrees(m.dataDir), m.dataDir, sp.ID, fix, p)
	if err != nil {
		return nil, openWorkspaceOut{}, err
	}
	if w.State == store.WorkspaceReady && m.tok.BotID != nil {
		turns, err := m.st.Turns(ctx, store.TurnFilter{SpaceID: &sp.ID, BotID: m.tok.BotID, Status: "running"})
		if err != nil {
			return nil, openWorkspaceOut{}, err
		}
		if len(turns) > 0 {
			turn := turns[len(turns)-1]
			checkpointed := false
			for _, checkpoint := range turn.Checkpoints {
				if checkpoint.WorkspaceID == w.ID {
					checkpointed = true
					break
				}
			}
			if !checkpointed {
				if err := space.CheckpointWorkspace(ctx, m.st, m.dataDir, turn.ID, w); err != nil {
					return nil, openWorkspaceOut{}, err
				}
			}
		}
	}
	return nil, openWorkspaceOut{
		ID: w.ID, Fix: w.Name, Dir: w.Dir, Branch: w.Branch, Base: w.Base, BaseSource: w.BaseSource,
		State: w.State, Note: w.Note,
	}, nil
}
