package server

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/bhanuprakaash/reef/memory"
	"github.com/bhanuprakaash/reef/store"
)

type (
	projectCard struct {
		ID         int64  `json:"id"`
		Name       string `json:"name"`
		Purpose    string `json:"purpose"`
		Path       string `json:"path"`
		BaseBranch string `json:"baseBranch"`
		Associated bool   `json:"associated"`
	}
	listProjectsOut struct {
		Projects []projectCard `json:"projects"`
	}
	projectIn struct {
		ID int64 `json:"id" jsonschema:"Project id, from list_projects"`
	}
	projectOut struct {
		ID            int64     `json:"id"`
		Name          string    `json:"name"`
		Purpose       string    `json:"purpose"`
		Path          string    `json:"path"`
		BaseBranch    string    `json:"baseBranch"`
		Associated    bool      `json:"associated"`
		Relationships []itemOut `json:"relationships"`
	}
	projectEditIn struct {
		ProjectID int64 `json:"projectId" jsonschema:"Project id, from list_projects"`
	}
	relateProjectsIn struct {
		From          int64  `json:"from" jsonschema:"id of the Project the fact is kept with"`
		To            int64  `json:"to" jsonschema:"id of the other Project"`
		Fact          string `json:"fact" jsonschema:"the relationship in plain words, such as: SM2 Infra deploys SM2"`
		SourceMessage int64  `json:"sourceMessage,omitempty" jsonschema:"id of the message that said it; default the user's latest message in this Space"`
	}
)

func (m *mcpServer) registerProjectTools(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "list_projects",
		Description: "List every Project with its name, purpose, repository path and base branch, " +
			"and whether this conversation is associated with it. Read one in full with read_project.",
	}, m.listProjects)
	mcp.AddTool(srv, &mcp.Tool{
		Name: "read_project",
		Description: "Read one Project: its metadata and the relationships saved about it, such as " +
			"which Project deploys it. Use recall with scope project for its other memory.",
	}, m.readProject)
	mcp.AddTool(srv, &mcp.Tool{
		Name: "associate_project",
		Description: "Associate this conversation with a Project so it is found under it. Do it only " +
			"when the Project is really relevant to the work or the user asks: a passing mention " +
			"is not a reason. The change shows in the history under your name; a Space of a work " +
			"item cannot change Projects.",
	}, m.associateProject)
	mcp.AddTool(srv, &mcp.Tool{
		Name: "dissociate_project",
		Description: "Remove this conversation's association with a Project. The conversation and " +
			"its history stay. The change shows in the history under your name.",
	}, m.dissociateProject)
	mcp.AddTool(srv, &mcp.Tool{
		Name: "relate_projects",
		Description: "Save a relationship the user stated between two Projects, such as \"SM2 Infra " +
			"deploys SM2\", as a Project memory fact that refers to both and to the user's message. " +
			"Correct it with remember and replaces, or retire it, like any memory item.",
	}, m.relateProjects)
}

func (m *mcpServer) associated(ctx context.Context) ([]store.Project, error) {
	if _, err := m.currentName(ctx); err != nil {
		return nil, err
	}
	return m.st.SpaceProjects(ctx, m.tok.SpaceID)
}

func (m *mcpServer) listProjects(
	ctx context.Context, _ *mcp.CallToolRequest, _ struct{},
) (*mcp.CallToolResult, listProjectsOut, error) {
	mine, err := m.associated(ctx)
	if err != nil {
		return nil, listProjectsOut{}, err
	}
	all, err := m.st.Projects(ctx)
	if err != nil {
		return nil, listProjectsOut{}, err
	}
	out := listProjectsOut{Projects: make([]projectCard, 0, len(all))}
	for _, p := range all {
		out.Projects = append(out.Projects, card(p, mine))
	}
	return nil, out, nil
}

func (m *mcpServer) readProject(
	ctx context.Context, _ *mcp.CallToolRequest, in projectIn,
) (*mcp.CallToolResult, projectOut, error) {
	mine, err := m.associated(ctx)
	if err != nil {
		return nil, projectOut{}, err
	}
	p, err := m.st.Project(ctx, in.ID)
	if err != nil {
		return nil, projectOut{}, err
	}
	items, err := m.ms.About(ctx, memory.Ref{Kind: memory.ScopeProject, ID: strconv.FormatInt(p.ID, 10)})
	if err != nil {
		return nil, projectOut{}, err
	}
	c := card(p, mine)
	out := projectOut{
		ID: c.ID, Name: c.Name, Purpose: c.Purpose, Path: c.Path, BaseBranch: c.BaseBranch,
		Associated: c.Associated, Relationships: make([]itemOut, 0, len(items)),
	}
	for _, it := range items {
		out.Relationships = append(out.Relationships, toItemOut(it))
	}
	return nil, out, nil
}

func (m *mcpServer) associateProject(
	ctx context.Context, _ *mcp.CallToolRequest, in projectEditIn,
) (*mcp.CallToolResult, listProjectsOut, error) {
	return m.editProject(ctx, in.ProjectID, m.st.AddSpaceProject)
}

func (m *mcpServer) dissociateProject(
	ctx context.Context, _ *mcp.CallToolRequest, in projectEditIn,
) (*mcp.CallToolResult, listProjectsOut, error) {
	return m.editProject(ctx, in.ProjectID, m.st.RemoveSpaceProject)
}

func (m *mcpServer) editProject(
	ctx context.Context, projectID int64,
	edit func(ctx context.Context, id, projectID int64, by *int64) error,
) (*mcp.CallToolResult, listProjectsOut, error) {
	if _, err := m.associated(ctx); err != nil {
		return nil, listProjectsOut{}, err
	}
	if err := edit(ctx, m.tok.SpaceID, projectID, m.tok.BotID); err != nil {
		return nil, listProjectsOut{}, err
	}
	return m.listProjects(ctx, nil, struct{}{})
}

func (m *mcpServer) relateProjects(
	ctx context.Context, _ *mcp.CallToolRequest, in relateProjectsIn,
) (*mcp.CallToolResult, rememberOut, error) {
	name, err := m.currentName(ctx)
	if err != nil {
		return nil, rememberOut{}, err
	}
	if in.From == in.To {
		return nil, rememberOut{}, fmt.Errorf("%w: a relationship needs two different Projects", store.ErrInvalid)
	}
	mine, err := m.st.SpaceProjects(ctx, m.tok.SpaceID)
	if err != nil {
		return nil, rememberOut{}, err
	}
	if !slices.ContainsFunc(mine, func(p store.Project) bool { return p.ID == in.From }) {
		return nil, rememberOut{}, fmt.Errorf(
			"%w: this conversation is not associated with Project %d; call associate_project first",
			store.ErrInvalid, in.From)
	}
	for _, id := range []int64{in.From, in.To} {
		if _, err := m.st.Project(ctx, id); err != nil {
			return nil, rememberOut{}, err
		}
	}
	msgs, err := m.st.Messages(ctx, m.tok.SpaceID, 0)
	if err != nil {
		return nil, rememberOut{}, err
	}
	if in.SourceMessage == 0 {
		for j := len(msgs) - 1; j >= 0 && in.SourceMessage == 0; j-- {
			if msgs[j].Author == "user" {
				in.SourceMessage = msgs[j].ID
			}
		}
		if in.SourceMessage == 0 {
			return nil, rememberOut{}, errors.New("no user message in this Space to cite as the source")
		}
	} else if !slices.ContainsFunc(msgs, func(msg store.Message) bool { return msg.ID == in.SourceMessage }) {
		return nil, rememberOut{}, fmt.Errorf("%w: message %d is not in this Space", store.ErrInvalid, in.SourceMessage)
	}
	id := func(n int64) string { return strconv.FormatInt(n, 10) }
	res, err := m.ms.Remember(ctx,
		memory.Writer{
			Author: name, Role: name, BotID: m.tok.BotID,
			Scope: memory.Ref{Kind: memory.ScopeProject, ID: id(in.From)},
			Links: []memory.Link{{Rel: memory.RelSource, ToKind: "space", ToID: id(m.tok.SpaceID)}},
		},
		memory.Note{
			Kind: memory.KindFact, Body: in.Fact,
			About: []memory.Ref{
				{Kind: memory.ScopeProject, ID: id(in.From)},
				{Kind: memory.ScopeProject, ID: id(in.To)},
				{Kind: "message", ID: id(in.SourceMessage)},
			},
		})
	if err != nil {
		return nil, rememberOut{}, err
	}
	return nil, rememberOut{
		Item: toItemOut(res.Item), Similar: toSimilarOut(res.Similar), EmbedError: embedMessage(res.EmbedErr),
	}, nil
}

func card(p store.Project, mine []store.Project) projectCard {
	return projectCard{
		ID: p.ID, Name: p.Name, Purpose: p.Purpose, Path: p.Path, BaseBranch: p.BaseBranch,
		Associated: slices.ContainsFunc(mine, func(q store.Project) bool { return q.ID == p.ID }),
	}
}

// withProjects adds the shelf of every Project the token's Space is associated with to scopes.
func (m *mcpServer) withProjects(ctx context.Context, scopes []memory.Ref) ([]memory.Ref, error) {
	ps, err := m.st.SpaceProjects(ctx, m.tok.SpaceID)
	if err != nil {
		return nil, err
	}
	for _, p := range ps {
		r := memory.Ref{Kind: memory.ScopeProject, ID: strconv.FormatInt(p.ID, 10)}
		if !slices.Contains(scopes, r) {
			scopes = append(scopes, r)
		}
	}
	return scopes, nil
}
