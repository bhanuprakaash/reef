package server

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/bhanuprakaash/reef/store"
)

const _defaultSearchLimit = 20

type mcpServer struct {
	st      *store.Store
	tok     store.Token
	dataDir string
}

type (
	briefOut struct {
		Brief string `json:"brief" jsonschema:"the feature brief, empty if none yet"`
		Notes string `json:"notes" jsonschema:"your role's notes on this feature, empty if none"`
	}
	decideIn struct {
		Body      string `json:"body" jsonschema:"the decision"`
		Why       string `json:"why,omitempty" jsonschema:"the reason for it"`
		MessageID *int64 `json:"messageId,omitempty" jsonschema:"id of the message it came from"`
	}
	taskIn struct {
		Title string `json:"title" jsonschema:"short task title"`
		Body  string `json:"body,omitempty" jsonschema:"details"`
	}
	listTasksIn struct {
		Status string `json:"status,omitempty" jsonschema:"only tasks with this status: todo, doing or done; omit for all"`
	}
	listTasksOut struct {
		Tasks []store.Task `json:"tasks"`
	}
	readTaskIn struct {
		ID int64 `json:"id" jsonschema:"task id"`
	}
	updateTaskIn struct {
		ID     int64   `json:"id" jsonschema:"task id"`
		Title  *string `json:"title,omitempty" jsonschema:"new title; omit to keep it"`
		Body   *string `json:"body,omitempty" jsonschema:"new body; omit to keep it"`
		Status *string `json:"status,omitempty" jsonschema:"new status: todo, doing or done; omit to keep it"`
	}
	searchIn struct {
		Query string `json:"query" jsonschema:"text to find"`
		Limit int    `json:"limit,omitempty" jsonschema:"most hits to return, default 20"`
	}
	searchOut struct {
		Hits []store.MessageHit `json:"hits"`
	}
	decisionIn struct {
		ID int64 `json:"id" jsonschema:"decision id"`
	}
	threadIn struct {
		ChannelID int64 `json:"channelId" jsonschema:"channel id, from search_sessions"`
		After     int64 `json:"after,omitempty" jsonschema:"only messages with a larger id"`
		Root      int64 `json:"root,omitempty" jsonschema:"id of a top-level message: read it and its replies instead; after is ignored"`
	}
	threadOut struct {
		Messages []store.Message `json:"messages"`
	}
	postIn struct {
		Body   string `json:"body" jsonschema:"the message; @mention a teammate to hand work over"`
		Parent *int64 `json:"parent,omitempty" jsonschema:"id of a top-level message to reply to in its thread; omit to post at the top level"`
	}
	stageIn struct {
		Stage string `json:"stage" jsonschema:"the stage the work has moved to: ideation, grill, plan, tickets, implement, testing, live-testing or post-release"`
	}
	wrapIn struct {
		Brief     string                   `json:"brief" jsonschema:"the full rewritten brief: goal, status, key decisions, open questions"`
		Notes     string                   `json:"notes" jsonschema:"your own notes on this feature"`
		Decisions []store.ProposedDecision `json:"decisions" jsonschema:"decisions made in this channel that are not recorded yet"`
	}
	wrapOut struct {
		ID      int64  `json:"id" jsonschema:"proposal id"`
		Message string `json:"message"`
	}
	browserNavigateIn struct {
		URL string `json:"url" jsonschema:"the URL to open in the browser pane"`
	}
	browserClickIn struct {
		Ref string `json:"ref" jsonschema:"a ref from browser_snapshot, such as e12"`
	}
	browserTypeIn struct {
		Ref  string `json:"ref" jsonschema:"a ref from browser_snapshot, such as e12"`
		Text string `json:"text" jsonschema:"the text to type"`
	}
	browserTextOut struct {
		Text string `json:"text"`
	}
)

// NewMCP returns the MCP server for one teammate: tok names the channel it
// works in, the role it speaks as and, through the channel, the feature.
// dataDir is the Reef data folder, where the Reef app's browser socket is.
func NewMCP(st *store.Store, tok store.Token, dataDir string) *mcp.Server {
	m := &mcpServer{st: st, tok: tok, dataDir: dataDir}
	srv := mcp.NewServer(&mcp.Implementation{Name: "reef", Version: "v0.1.0"}, nil)
	mcp.AddTool(srv, &mcp.Tool{
		Name: "get_brief",
		Description: "Get this feature's brief and your role's notes on it. " +
			"Call it first when you start work.",
	}, m.getBrief)
	mcp.AddTool(srv, &mcp.Tool{
		Name: "record_decision",
		Description: "Record a decision made on this feature, with why " +
			"and the message it came from.",
	}, m.recordDecision)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "add_task",
		Description: "Add a task to this feature's project.",
	}, m.addTask)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_tasks",
		Description: "List this feature's tasks by id, optionally only those with one status.",
	}, m.listTasks)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "read_task",
		Description: "Read one task of this feature in full.",
	}, m.readTask)
	mcp.AddTool(srv, &mcp.Tool{
		Name: "update_task",
		Description: "Change the title, body or status of one of this feature's tasks; " +
			"fields you omit stay as they are. Mark a task done with status done.",
	}, m.updateTask)
	mcp.AddTool(srv, &mcp.Tool{
		Name: "search_sessions",
		Description: "Search messages in all of this feature's channels, newest first. " +
			"Returns channel and message ids; read more with read_thread.",
	}, m.searchSessions)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "read_decision",
		Description: "Read one decision of this feature in full.",
	}, m.readDecision)
	mcp.AddTool(srv, &mcp.Tool{
		Name: "read_thread",
		Description: "Read the messages of one of this feature's channels, oldest first. " +
			"Give root, a top-level message id, to read just that message and its replies.",
	}, m.readThread)
	mcp.AddTool(srv, &mcp.Tool{
		Name: "post_to_channel",
		Description: "Post a message to this channel as yourself. @mention a teammate to wake " +
			"them when your turn ends; a post that mentions nobody wakes no one.",
	}, m.postToChannel)
	mcp.AddTool(srv, &mcp.Tool{
		Name: "set_stage",
		Description: "Move this feature to the next journey stage when the work moves there. " +
			"The channel sees a note; nothing needs approval.",
	}, m.setStage)
	mcp.AddTool(srv, &mcp.Tool{
		Name: "propose_wrap",
		Description: "Propose how to wrap up this channel; the user approves it before " +
			"anything is saved. brief is the full rewritten brief (goal, status, key " +
			"decisions, open questions), notes are your own notes for this feature, " +
			"decisions are ones made in this channel that are not recorded yet.",
	}, m.proposeWrap)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "browser_navigate",
		Description: "Open a URL in the shared browser pane. The user watches it live.",
	}, m.browserNavigate)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "browser_screenshot",
		Description: "Take a screenshot of the browser pane's current page.",
	}, m.browserScreenshot)
	mcp.AddTool(srv, &mcp.Tool{
		Name: "browser_snapshot",
		Description: "Read the browser pane's current page as text, one line per element with " +
			"a ref such as e12. Use the ref with browser_click and browser_type. Refs are only " +
			"valid until the next snapshot or a navigation.",
	}, m.browserSnapshot)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "browser_click",
		Description: "Click an element in the browser pane, by its ref from browser_snapshot.",
	}, m.browserClick)
	mcp.AddTool(srv, &mcp.Tool{
		Name: "browser_type",
		Description: "Type text into an element in the browser pane, by its ref from " +
			"browser_snapshot.",
	}, m.browserType)
	mcp.AddTool(srv, &mcp.Tool{
		Name: "browser_console",
		Description: "Read the browser pane's console messages since the last navigation, " +
			"one per line with its level.",
	}, m.browserConsole)
	mcp.AddTool(srv, &mcp.Tool{
		Name: "browser_network",
		Description: "List the browser pane's network requests since the last navigation, " +
			"one per line with method, URL and status.",
	}, m.browserNetwork)
	return srv
}

func (m *mcpServer) feature(ctx context.Context) (store.Feature, error) {
	c, err := m.st.Channel(ctx, m.tok.ChannelID)
	if err != nil {
		return store.Feature{}, err
	}
	if c.FeatureID == nil {
		return store.Feature{}, errors.New("this channel has no feature")
	}
	return m.st.Feature(ctx, *c.FeatureID)
}

func (m *mcpServer) getBrief(
	ctx context.Context, _ *mcp.CallToolRequest, _ struct{},
) (*mcp.CallToolResult, briefOut, error) {
	f, err := m.feature(ctx)
	if err != nil {
		return nil, briefOut{}, err
	}
	b, err := m.st.Brief(ctx, f.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, briefOut{}, err
	}
	n, err := m.st.RoleNotes(ctx, f.ID, m.tok.Role)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, briefOut{}, err
	}
	return nil, briefOut{Brief: b.Body, Notes: n.Body}, nil
}

func (m *mcpServer) recordDecision(
	ctx context.Context, _ *mcp.CallToolRequest, in decideIn,
) (*mcp.CallToolResult, store.Decision, error) {
	f, err := m.feature(ctx)
	if err != nil {
		return nil, store.Decision{}, err
	}
	d, err := m.st.CreateDecision(ctx, store.Decision{
		FeatureID: f.ID, MessageID: in.MessageID, Body: in.Body, Why: in.Why,
		Author: m.tok.Role,
	})
	return nil, d, err
}

func (m *mcpServer) addTask(
	ctx context.Context, _ *mcp.CallToolRequest, in taskIn,
) (*mcp.CallToolResult, store.Task, error) {
	f, err := m.feature(ctx)
	if err != nil {
		return nil, store.Task{}, err
	}
	t, err := m.st.CreateTask(ctx, store.Task{
		ProjectID: f.ProjectID, FeatureID: &f.ID, Title: in.Title, Body: in.Body,
	})
	return nil, t, err
}

func (m *mcpServer) listTasks(
	ctx context.Context, _ *mcp.CallToolRequest, in listTasksIn,
) (*mcp.CallToolResult, listTasksOut, error) {
	f, err := m.feature(ctx)
	if err != nil {
		return nil, listTasksOut{}, err
	}
	ts, err := m.st.Tasks(ctx, store.TaskFilter{FeatureID: &f.ID, Status: in.Status})
	if err != nil {
		return nil, listTasksOut{}, err
	}
	if ts == nil {
		ts = []store.Task{}
	}
	return nil, listTasksOut{Tasks: ts}, nil
}

func (m *mcpServer) readTask(
	ctx context.Context, _ *mcp.CallToolRequest, in readTaskIn,
) (*mcp.CallToolResult, store.Task, error) {
	t, err := m.featureTask(ctx, in.ID)
	return nil, t, err
}

func (m *mcpServer) updateTask(
	ctx context.Context, _ *mcp.CallToolRequest, in updateTaskIn,
) (*mcp.CallToolResult, store.Task, error) {
	t, err := m.featureTask(ctx, in.ID)
	if err != nil {
		return nil, store.Task{}, err
	}
	if in.Title != nil {
		t.Title = *in.Title
	}
	if in.Body != nil {
		t.Body = *in.Body
	}
	if in.Status != nil {
		t.Status = *in.Status
	}
	t, err = m.st.UpdateTask(ctx, t)
	return nil, t, err
}

func (m *mcpServer) featureTask(ctx context.Context, id int64) (store.Task, error) {
	f, err := m.feature(ctx)
	if err != nil {
		return store.Task{}, err
	}
	t, err := m.st.Task(ctx, id)
	if err != nil {
		return store.Task{}, err
	}
	if t.FeatureID == nil || *t.FeatureID != f.ID {
		return store.Task{}, fmt.Errorf("task %d: %w", id, store.ErrNotFound)
	}
	return t, nil
}

func (m *mcpServer) searchSessions(
	ctx context.Context, _ *mcp.CallToolRequest, in searchIn,
) (*mcp.CallToolResult, searchOut, error) {
	f, err := m.feature(ctx)
	if err != nil {
		return nil, searchOut{}, err
	}
	if in.Query == "" {
		return nil, searchOut{}, errors.New("query is required")
	}
	if in.Limit <= 0 {
		in.Limit = _defaultSearchLimit
	}
	hits, err := m.st.SearchMessages(ctx, f.ID, in.Query, in.Limit)
	return nil, searchOut{Hits: hits}, err
}

func (m *mcpServer) readDecision(
	ctx context.Context, _ *mcp.CallToolRequest, in decisionIn,
) (*mcp.CallToolResult, store.Decision, error) {
	f, err := m.feature(ctx)
	if err != nil {
		return nil, store.Decision{}, err
	}
	d, err := m.st.Decision(ctx, in.ID)
	if err != nil {
		return nil, store.Decision{}, err
	}
	if d.FeatureID != f.ID {
		return nil, store.Decision{}, fmt.Errorf("decision %d: %w", in.ID, store.ErrNotFound)
	}
	return nil, d, nil
}

func (m *mcpServer) readThread(
	ctx context.Context, _ *mcp.CallToolRequest, in threadIn,
) (*mcp.CallToolResult, threadOut, error) {
	c, err := m.st.Channel(ctx, in.ChannelID)
	if err != nil {
		return nil, threadOut{}, err
	}
	if c.ID != m.tok.ChannelID {
		f, err := m.feature(ctx)
		if err != nil {
			return nil, threadOut{}, err
		}
		if c.FeatureID == nil || *c.FeatureID != f.ID {
			return nil, threadOut{}, fmt.Errorf("channel %d: %w", in.ChannelID, store.ErrNotFound)
		}
	}
	if in.Root != 0 {
		msgs, err := m.st.Thread(ctx, c.ID, in.Root)
		return nil, threadOut{Messages: msgs}, err
	}
	msgs, err := m.st.Messages(ctx, c.ID, in.After)
	return nil, threadOut{Messages: msgs}, err
}

func (m *mcpServer) postToChannel(
	ctx context.Context, _ *mcp.CallToolRequest, in postIn,
) (*mcp.CallToolResult, store.Message, error) {
	msg, err := m.st.CreateMessage(ctx, store.Message{
		ChannelID: m.tok.ChannelID, ParentID: in.Parent, Author: m.tok.Role, Body: in.Body,
	})
	return nil, msg, err
}

func (m *mcpServer) setStage(
	ctx context.Context, _ *mcp.CallToolRequest, in stageIn,
) (*mcp.CallToolResult, store.Feature, error) {
	f, err := m.feature(ctx)
	if err != nil {
		return nil, store.Feature{}, err
	}
	f, err = m.st.SetFeatureStage(ctx, f.ID, in.Stage)
	if err != nil {
		return nil, store.Feature{}, err
	}
	_, err = m.st.CreateMessage(ctx, store.Message{
		ChannelID: m.tok.ChannelID, Author: store.ReefAuthor,
		Body: fmt.Sprintf("%s moved the stage to %s", m.tok.Role, f.Stage),
	})
	return nil, f, err
}

func (m *mcpServer) proposeWrap(
	ctx context.Context, _ *mcp.CallToolRequest, in wrapIn,
) (*mcp.CallToolResult, wrapOut, error) {
	if _, err := m.feature(ctx); err != nil {
		return nil, wrapOut{}, err
	}
	p, err := m.st.CreateProposal(ctx, store.Proposal{
		ChannelID: m.tok.ChannelID, Role: m.tok.Role, Brief: in.Brief, Notes: in.Notes,
		Decisions: in.Decisions,
	})
	if err != nil {
		return nil, wrapOut{}, err
	}
	return nil, wrapOut{ID: p.ID, Message: "proposal saved; the user will approve it"}, nil
}

// browserText runs a browser tool that replies with plain text; browser_screenshot
// is the only one that does not, since its reply is an image.
func (m *mcpServer) browserText(
	ctx context.Context, tool string, args any,
) (*mcp.CallToolResult, browserTextOut, error) {
	reply, err := callBrowser(ctx, m.dataDir, tool, args)
	if err != nil {
		return nil, browserTextOut{}, err
	}
	return nil, browserTextOut{Text: reply.Text}, nil
}

func (m *mcpServer) browserNavigate(
	ctx context.Context, _ *mcp.CallToolRequest, in browserNavigateIn,
) (*mcp.CallToolResult, browserTextOut, error) {
	return m.browserText(ctx, "navigate", map[string]string{"url": in.URL})
}

func (m *mcpServer) browserScreenshot(
	ctx context.Context, _ *mcp.CallToolRequest, _ struct{},
) (*mcp.CallToolResult, any, error) {
	reply, err := callBrowser(ctx, m.dataDir, "screenshot", nil)
	if err != nil {
		return nil, nil, err
	}
	data, err := base64.StdEncoding.DecodeString(reply.Image)
	if err != nil {
		return nil, nil, fmt.Errorf("decode screenshot: %w", err)
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.ImageContent{Data: data, MIMEType: "image/png"}},
	}, nil, nil
}

func (m *mcpServer) browserSnapshot(
	ctx context.Context, _ *mcp.CallToolRequest, _ struct{},
) (*mcp.CallToolResult, browserTextOut, error) {
	return m.browserText(ctx, "snapshot", nil)
}

func (m *mcpServer) browserClick(
	ctx context.Context, _ *mcp.CallToolRequest, in browserClickIn,
) (*mcp.CallToolResult, browserTextOut, error) {
	return m.browserText(ctx, "click", map[string]string{"ref": in.Ref})
}

func (m *mcpServer) browserType(
	ctx context.Context, _ *mcp.CallToolRequest, in browserTypeIn,
) (*mcp.CallToolResult, browserTextOut, error) {
	return m.browserText(ctx, "type", map[string]string{"ref": in.Ref, "text": in.Text})
}

func (m *mcpServer) browserConsole(
	ctx context.Context, _ *mcp.CallToolRequest, _ struct{},
) (*mcp.CallToolResult, browserTextOut, error) {
	return m.browserText(ctx, "console", nil)
}

func (m *mcpServer) browserNetwork(
	ctx context.Context, _ *mcp.CallToolRequest, _ struct{},
) (*mcp.CallToolResult, browserTextOut, error) {
	return m.browserText(ctx, "network", nil)
}
