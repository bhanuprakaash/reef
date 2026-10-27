package server

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/bhanuprakaash/reef/channel"
	"github.com/bhanuprakaash/reef/memory"
	"github.com/bhanuprakaash/reef/store"
)

const _defaultSearchLimit = 20

type mcpServer struct {
	st      *store.Store
	ms      *memory.Store
	tok     store.Token
	dataDir string
}

type (
	briefOut struct {
		Brief string `json:"brief" jsonschema:"the feature brief, empty if none yet"`
	}
	rememberIn struct {
		Kind          string   `json:"kind" jsonschema:"decision, convention, finding, fact or lesson"`
		Body          string   `json:"body" jsonschema:"what to remember"`
		About         []string `json:"about,omitempty" jsonschema:"what it is about, each as kind:id, such as work_item:3"`
		Replaces      []int64  `json:"replaces,omitempty" jsonschema:"ids of accepted items this one amends or replaces; they are retired"`
		Why           string   `json:"why,omitempty" jsonschema:"the reason for it"`
		Scope         string   `json:"scope,omitempty" jsonschema:"where it is kept: work_item (this feature, the default), project or teammate (you, on every feature)"`
		NeedsApproval bool     `json:"needsApproval,omitempty" jsonschema:"ask the user to approve it before it counts"`
	}
	recallIn struct {
		Query string `json:"query" jsonschema:"text to find"`
		Limit int    `json:"limit,omitempty" jsonschema:"most hits to return, default 10"`
		Scope string `json:"scope,omitempty" jsonschema:"only this scope: work_item, project, team or teammate; omit for all you can read"`
	}
	readMemoryIn struct {
		ID int64 `json:"id" jsonschema:"memory id"`
	}
	linkOut struct {
		Rel string `json:"rel"`
		To  string `json:"to"`
	}
	itemOut struct {
		ID         int64      `json:"id"`
		Kind       string     `json:"kind"`
		Body       string     `json:"body"`
		Why        string     `json:"why"`
		Scope      string     `json:"scope"`
		Author     string     `json:"author"`
		Role       string     `json:"role"`
		Status     string     `json:"status"`
		RetiredAt  *time.Time `json:"retiredAt"`
		ReplacedBy *int64     `json:"replacedBy"`
		CreatedAt  time.Time  `json:"createdAt"`
		Links      []linkOut  `json:"links"`
	}
	similarOut struct {
		ID   int64  `json:"id"`
		Body string `json:"body"`
	}
	rememberOut struct {
		Item       itemOut      `json:"item"`
		Similar    []similarOut `json:"similar,omitempty"`
		EmbedError string       `json:"embedError,omitempty"`
	}
	hitOut struct {
		ID     int64  `json:"id"`
		Kind   string `json:"kind"`
		Body   string `json:"body"`
		Scope  string `json:"scope"`
		Status string `json:"status"`
		Via    string `json:"via"`
	}
	recallOut struct {
		Hits []hitOut `json:"hits"`
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
	threadIn struct {
		ChannelID int64 `json:"channelId" jsonschema:"channel id, from search_sessions"`
		After     int64 `json:"after,omitempty" jsonschema:"only messages with a larger id"`
		Root      int64 `json:"root,omitempty" jsonschema:"id of a top-level message: read it and its replies instead; after is ignored"`
	}
	threadOut struct {
		Messages []store.Message `json:"messages"`
	}
	postIn struct {
		Body   string `json:"body" jsonschema:"the message; start a line with @name to hand work over"`
		Parent *int64 `json:"parent,omitempty" jsonschema:"id of a top-level message to reply to in its thread; omit to post at the top level"`
	}
	stageIn struct {
		Stage string `json:"stage" jsonschema:"the stage the work has moved to: ideation, grill, plan, tickets, implement, testing, live-testing or post-release"`
	}
	directIn struct {
		To   string `json:"to" jsonschema:"exactly one teammate role"`
		Body string `json:"body" jsonschema:"the private message for that teammate"`
	}
	directListIn struct {
		After int64 `json:"after,omitempty" jsonschema:"only direct messages with a larger id"`
	}
	directListOut struct {
		Messages []store.DirectMessage `json:"messages"`
	}
	wrapIn struct {
		Brief  string               `json:"brief" jsonschema:"the full rewritten brief: goal, status, key decisions, open questions"`
		Items  []store.ProposedItem `json:"items,omitempty" jsonschema:"memory items made in this channel that are not saved with remember yet: kind (decision, finding, fact, lesson or convention), body, why, scope (work_item by default, project, teammate, or team for a team lead), replaces (ids of live items it replaces)"`
		Retire []int64              `json:"retire,omitempty" jsonschema:"ids of live memory items that are stale or duplicate"`
	}
	briefIn struct {
		Brief string `json:"brief" jsonschema:"the full brief: goal, status, key decisions, open questions"`
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
func NewMCP(st *store.Store, ms *memory.Store, tok store.Token, dataDir string) *mcp.Server {
	m := &mcpServer{st: st, ms: ms, tok: tok, dataDir: dataDir}
	srv := mcp.NewServer(&mcp.Implementation{Name: "reef", Version: "v0.1.0"}, nil)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_brief",
		Description: "Get this feature's brief. Call it first when you start work.",
	}, m.getBrief)
	mcp.AddTool(srv, &mcp.Tool{
		Name: "remember",
		Description: "Remember something for later, with why. A decision is a choice about how " +
			"the work goes, not progress, proof or a won't-fix list. Scope work_item (the " +
			"default) keeps it on this feature, project on every feature of the project, " +
			"teammate on you across features. Pass replaces when it amends or replaces " +
			"earlier items: they are retired. A convention from a role other than architect " +
			"or lead waits for the user's approval.",
	}, m.remember)
	mcp.AddTool(srv, &mcp.Tool{
		Name: "recall",
		Description: "Search what the team remembers that you can read, retired items " +
			"included and marked. Returns ids; read one in full with read_memory.",
	}, m.recall)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "read_memory",
		Description: "Read one remembered item in full, with its links.",
	}, m.readMemory)
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
		Name: "read_thread",
		Description: "Read the messages of one of this feature's channels, oldest first. " +
			"Give root, a top-level message id, to read just that message and its replies.",
	}, m.readThread)
	mcp.AddTool(srv, &mcp.Tool{
		Name: "post_to_channel",
		Description: "Post a message to this channel as yourself. Start a line with @name to wake " +
			"that teammate when your turn ends; a post with no such line wakes no one.",
	}, m.postToChannel)
	mcp.AddTool(srv, &mcp.Tool{
		Name: "set_stage",
		Description: "Move this feature to the next journey stage when the work moves there. " +
			"The channel sees a note; nothing needs approval.",
	}, m.setStage)
	mcp.AddTool(srv, &mcp.Tool{
		Name: "send_to_role",
		Description: "Send a private message to exactly one teammate. The teammate's ordinary " +
			"reply comes back privately once; use send_to_role again to continue.",
	}, m.sendToRole)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "read_direct_messages",
		Description: "Read private messages in this channel that you sent or received.",
	}, m.readDirectMessages)
	mcp.AddTool(srv, &mcp.Tool{
		Name: "propose_wrap",
		Description: "Propose how to wrap up this channel; the user approves it before " +
			"anything is saved. brief is the full rewritten brief (goal, status, key " +
			"decisions, open questions). items are memory items made in this channel that " +
			"are not saved with remember yet, each with a kind, body, why, a scope " +
			"(work_item by default; project, teammate, or team when you are the team lead: " +
			"team items wait for the user) and replaces for earlier items it replaces. " +
			"retire lists ids of live memory items that are stale or duplicate.",
	}, m.proposeWrap)
	mcp.AddTool(srv, &mcp.Tool{
		Name: "propose_brief",
		Description: "Propose a new brief for this feature when the user and the team agree " +
			"on its goal, scope or plan; the user approves it before it is saved. brief is " +
			"the full brief (goal, status, key decisions, open questions). The channel goes " +
			"on; use propose_wrap only to end it.",
	}, m.proposeBrief)
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
	return nil, briefOut{Brief: b.Body}, nil
}

func (m *mcpServer) remember(
	ctx context.Context, _ *mcp.CallToolRequest, in rememberIn,
) (*mcp.CallToolResult, rememberOut, error) {
	switch in.Scope {
	case "":
		in.Scope = memory.ScopeWorkItem
	case memory.ScopeWorkItem, memory.ScopeProject, memory.ScopeTeammate:
	default:
		return nil, rememberOut{}, fmt.Errorf("%w: scope %q is not work_item, project or teammate",
			store.ErrInvalid, in.Scope)
	}
	c, err := m.st.Channel(ctx, m.tok.ChannelID)
	if err != nil {
		return nil, rememberOut{}, err
	}
	scopes := channel.Scopes(c, m.tok.Role)
	i := slices.IndexFunc(scopes, func(r memory.Ref) bool { return r.Kind == in.Scope })
	if i < 0 {
		return nil, rememberOut{}, errors.New("this channel has no feature")
	}
	about := make([]memory.Ref, 0, len(in.About))
	for _, a := range in.About {
		kind, id, ok := strings.Cut(a, ":")
		if !ok || kind == "" || id == "" {
			return nil, rememberOut{}, fmt.Errorf("%w: about %q is not kind:id", store.ErrInvalid, a)
		}
		about = append(about, memory.Ref{Kind: kind, ID: id})
	}
	for _, id := range in.Replaces {
		if _, err := m.readable(ctx, scopes, id); err != nil {
			return nil, rememberOut{}, err
		}
	}
	res, err := m.ms.Remember(ctx,
		memory.Writer{Author: m.tok.Role, Role: m.tok.Role, Scope: scopes[i]},
		memory.Note{
			Kind: in.Kind, Body: in.Body, Why: in.Why, About: about, Replaces: in.Replaces,
			NeedsApproval: in.NeedsApproval,
		})
	if err != nil {
		return nil, rememberOut{}, err
	}
	return nil, rememberOut{
		Item: toItemOut(res.Item), Similar: toSimilarOut(res.Similar), EmbedError: embedMessage(res.EmbedErr),
	}, nil
}

func (m *mcpServer) recall(
	ctx context.Context, _ *mcp.CallToolRequest, in recallIn,
) (*mcp.CallToolResult, recallOut, error) {
	if in.Query == "" {
		return nil, recallOut{}, errors.New("query is required")
	}
	c, err := m.st.Channel(ctx, m.tok.ChannelID)
	if err != nil {
		return nil, recallOut{}, err
	}
	scopes := channel.Scopes(c, m.tok.Role)
	q := memory.Query{Scopes: scopes, Text: in.Query, Limit: in.Limit}
	if in.Scope != "" {
		i := slices.IndexFunc(scopes, func(r memory.Ref) bool { return r.Kind == in.Scope })
		if i < 0 {
			return nil, recallOut{}, fmt.Errorf("%w: you have no %q scope here", store.ErrInvalid, in.Scope)
		}
		q.Scope = &scopes[i]
	}
	hits, err := m.ms.Recall(ctx, q)
	if err != nil {
		return nil, recallOut{}, err
	}
	out := recallOut{Hits: make([]hitOut, 0, len(hits))}
	for _, h := range hits {
		out.Hits = append(out.Hits, hitOut{
			ID: h.ID, Kind: h.Kind, Body: h.Body, Scope: refString(h.Scope), Status: h.Status, Via: h.Via,
		})
	}
	return nil, out, nil
}

func (m *mcpServer) readMemory(
	ctx context.Context, _ *mcp.CallToolRequest, in readMemoryIn,
) (*mcp.CallToolResult, itemOut, error) {
	c, err := m.st.Channel(ctx, m.tok.ChannelID)
	if err != nil {
		return nil, itemOut{}, err
	}
	scopes := channel.Scopes(c, m.tok.Role)
	it, err := m.readable(ctx, scopes, in.ID)
	if err != nil {
		return nil, itemOut{}, err
	}
	return nil, toItemOut(it), nil
}

// readable reads an item; it is not found when its scope is not one of scopes.
func (m *mcpServer) readable(ctx context.Context, scopes []memory.Ref, id int64) (memory.Item, error) {
	it, err := m.ms.Read(ctx, id)
	if err != nil {
		return memory.Item{}, err
	}
	if !slices.Contains(scopes, it.Scope) {
		return memory.Item{}, fmt.Errorf("memory %d: %w", id, memory.ErrNotFound)
	}
	return it, nil
}

func toItemOut(it memory.Item) itemOut {
	links := make([]linkOut, 0, len(it.Links))
	for _, l := range it.Links {
		links = append(links, linkOut{Rel: l.Rel, To: l.ToKind + ":" + l.ToID})
	}
	return itemOut{
		ID: it.ID, Kind: it.Kind, Body: it.Body, Why: it.Why, Scope: refString(it.Scope),
		Author: it.Author, Role: it.Role, Status: it.Status, RetiredAt: it.RetiredAt,
		ReplacedBy: it.ReplacedBy, CreatedAt: it.CreatedAt, Links: links,
	}
}

func toSimilarOut(similar []memory.Similar) []similarOut {
	var out []similarOut
	for _, s := range similar {
		out = append(out, similarOut{ID: s.ID, Body: s.Body})
	}
	return out
}

func embedMessage(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func refString(r memory.Ref) string { return r.Kind + ":" + r.ID }

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
	hits, err := m.st.SearchHistory(ctx, f.ID, in.Query, in.Limit, m.tok.Role)
	return nil, searchOut{Hits: hits}, err
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
	was := f.Stage
	f, err = m.st.SetFeatureStage(ctx, f.ID, in.Stage)
	if err != nil || f.Stage == was {
		return nil, f, err
	}
	_, err = m.st.CreateMessage(ctx, store.Message{
		ChannelID: m.tok.ChannelID, Author: store.ReefAuthor,
		Body: fmt.Sprintf("%s moved the stage to %s", m.tok.Role, f.Stage),
	})
	return nil, f, err
}

func (m *mcpServer) sendToRole(
	ctx context.Context, _ *mcp.CallToolRequest, in directIn,
) (*mcp.CallToolResult, store.DirectMessage, error) {
	if in.To == m.tok.Role {
		return nil, store.DirectMessage{}, fmt.Errorf("%w: cannot send a direct message to yourself", store.ErrInvalid)
	}
	ch, err := m.st.Channel(ctx, m.tok.ChannelID)
	if err != nil {
		return nil, store.DirectMessage{}, err
	}
	if ch.ClosedAt != nil {
		return nil, store.DirectMessage{}, fmt.Errorf("channel %d is closed: %w", ch.ID, store.ErrConflict)
	}
	if ch.ActiveTurn == store.ActiveTurnStarting {
		return nil, store.DirectMessage{}, fmt.Errorf("channel %d is starting: %w", ch.ID, store.ErrConflict)
	}
	if ch.ActiveTurn == store.ActiveTurnWrap || ch.ActiveTurn == store.ActiveTurnPromote {
		return nil, store.DirectMessage{}, fmt.Errorf("channel %d is wrapping or promoting: %w", ch.ID, store.ErrConflict)
	}
	running, err := m.st.Turns(ctx, store.TurnFilter{ChannelID: &ch.ID, Status: "running"})
	if err != nil {
		return nil, store.DirectMessage{}, err
	}
	var source *int64
	for _, t := range running {
		if t.WakeText == "" {
			return nil, store.DirectMessage{}, fmt.Errorf("channel %d is wrapping or promoting: %w", ch.ID, store.ErrConflict)
		}
		if t.Role != m.tok.Role {
			continue
		}
		if t.DirectMessageID == nil {
			source = wakeSource(t.WakeText)
			continue
		}
		woken, err := m.st.DirectMessage(ctx, *t.DirectMessageID)
		if err != nil {
			return nil, store.DirectMessage{}, err
		}
		source = woken.SourceMessageID
	}
	team, err := m.st.Team(ctx, ch.Team)
	if err != nil {
		return nil, store.DirectMessage{}, err
	}
	found := false
	for _, role := range team.Roles {
		name := role.Name
		if role.Lead {
			name = ch.Role
		}
		if name == in.To {
			found = true
			break
		}
	}
	if !found {
		return nil, store.DirectMessage{}, fmt.Errorf("%w: role %q is not in this channel's team", store.ErrInvalid, in.To)
	}
	dm, err := m.st.CreateDirectMessage(ctx, store.DirectMessage{
		ChannelID: m.tok.ChannelID, SourceMessageID: source, From: m.tok.Role, To: in.To, Body: in.Body,
	})
	return nil, dm, err
}

func (m *mcpServer) readDirectMessages(
	ctx context.Context, _ *mcp.CallToolRequest, in directListIn,
) (*mcp.CallToolResult, directListOut, error) {
	msgs, err := m.st.DirectMessagesForRole(ctx, m.tok.ChannelID, m.tok.Role, in.After)
	if msgs == nil {
		msgs = []store.DirectMessage{}
	}
	return nil, directListOut{Messages: msgs}, err
}

func (m *mcpServer) proposeWrap(
	ctx context.Context, _ *mcp.CallToolRequest, in wrapIn,
) (*mcp.CallToolResult, wrapOut, error) {
	if _, err := m.feature(ctx); err != nil {
		return nil, wrapOut{}, err
	}
	c, err := m.st.Channel(ctx, m.tok.ChannelID)
	if err != nil {
		return nil, wrapOut{}, err
	}
	scopes := channel.Scopes(c, m.tok.Role)
	for i := range in.Items {
		it := &in.Items[i]
		if it.Scope == "" {
			it.Scope = memory.ScopeWorkItem
		}
		if !memory.ValidKind(it.Kind) {
			return nil, wrapOut{}, fmt.Errorf("%w: unknown item kind %q", store.ErrInvalid, it.Kind)
		}
		at := slices.IndexFunc(scopes, func(r memory.Ref) bool { return r.Kind == it.Scope })
		if at < 0 {
			kinds := make([]string, len(scopes))
			for j, r := range scopes {
				kinds[j] = r.Kind
			}
			return nil, wrapOut{}, fmt.Errorf("%w: scope %q is not one of %s",
				store.ErrInvalid, it.Scope, strings.Join(kinds, ", "))
		}
		if it.Scope == memory.ScopeTeam {
			if err := m.requireLead(ctx, c.Team); err != nil {
				return nil, wrapOut{}, err
			}
		}
		if err := requireLive(ctx, m.ms, scopes[at:at+1], it.Replaces); err != nil {
			return nil, wrapOut{}, err
		}
	}
	if err := requireLive(ctx, m.ms, scopes, in.Retire); err != nil {
		return nil, wrapOut{}, err
	}
	p, err := m.st.CreateProposal(ctx, store.Proposal{
		ChannelID: m.tok.ChannelID, Role: m.tok.Role, Brief: in.Brief, Items: in.Items, Retire: in.Retire,
	})
	if err != nil {
		return nil, wrapOut{}, err
	}
	return nil, wrapOut{ID: p.ID, Message: "proposal saved; the user will approve it"}, nil
}

// requireLead fails unless the token's role leads team.
func (m *mcpServer) requireLead(ctx context.Context, team string) error {
	t, err := m.st.Team(ctx, team)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(t.Roles, func(r store.Role) bool { return r.Lead && r.Name == m.tok.Role }) {
		return fmt.Errorf("%w: only the lead of team %q can propose team items", store.ErrInvalid, team)
	}
	return nil
}

// requireLive fails with ErrInvalid unless every id is an accepted item in one of scopes.
func requireLive(ctx context.Context, ms *memory.Store, scopes []memory.Ref, ids []int64) error {
	for _, id := range ids {
		it, err := ms.Read(ctx, id)
		if err != nil && !errors.Is(err, memory.ErrNotFound) {
			return err
		}
		if err != nil || it.Status != memory.StatusAccepted || !slices.Contains(scopes, it.Scope) {
			names := make([]string, len(scopes))
			for i, r := range scopes {
				names[i] = refString(r)
			}
			return fmt.Errorf("%w: memory %d is not a live item in %s",
				store.ErrInvalid, id, strings.Join(names, ", "))
		}
	}
	return nil
}

func (m *mcpServer) proposeBrief(
	ctx context.Context, _ *mcp.CallToolRequest, in briefIn,
) (*mcp.CallToolResult, wrapOut, error) {
	if _, err := m.feature(ctx); err != nil {
		return nil, wrapOut{}, err
	}
	p, err := m.st.CreateProposal(ctx, store.Proposal{
		ChannelID: m.tok.ChannelID, Role: m.tok.Role, Kind: store.ProposalBrief, Brief: in.Brief,
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
