package server

import (
	"cmp"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/bhanuprakaash/reef/memory"
	"github.com/bhanuprakaash/reef/space"
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
		Brief string `json:"brief" jsonschema:"the work item brief, empty if none yet"`
	}
	rememberIn struct {
		Kind          string   `json:"kind" jsonschema:"decision, convention, finding, fact or lesson"`
		Body          string   `json:"body" jsonschema:"what to remember"`
		About         []string `json:"about,omitempty" jsonschema:"what it is about, each as kind:id, such as work_item:3"`
		Replaces      []int64  `json:"replaces,omitempty" jsonschema:"ids of accepted items this one amends or replaces; they are retired"`
		Why           string   `json:"why,omitempty" jsonschema:"the reason for it"`
		Scope         string   `json:"scope,omitempty" jsonschema:"where it is kept: this work item, this Space (a free Space) or this conversation (a direct one), whichever you are in, by default; project, or bot (you, on every work item)"`
		NeedsApproval bool     `json:"needsApproval,omitempty" jsonschema:"ask the user to approve it before it counts"`
		ProjectID     int64    `json:"projectId,omitempty" jsonschema:"with scope project, which associated Project to keep it with; required when the Space has several"`
	}
	recallIn struct {
		Query string `json:"query" jsonschema:"text to find"`
		Limit int    `json:"limit,omitempty" jsonschema:"most hits to return, default 10"`
		Scope string `json:"scope,omitempty" jsonschema:"only this scope: work_item, space, conversation, project or bot; omit for all you can read"`
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
		BotID      *int64     `json:"botId"`
		Status     string     `json:"status"`
		RetiredAt  *time.Time `json:"retiredAt"`
		ReplacedBy *int64     `json:"replacedBy"`
		CreatedAt  time.Time  `json:"createdAt"`
		Links      []linkOut  `json:"links,omitempty"`
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
	searchIn struct {
		Query string `json:"query" jsonschema:"text to find"`
		Limit int    `json:"limit,omitempty" jsonschema:"most hits to return, default 20"`
	}
	searchOut struct {
		Hits []store.MessageHit `json:"hits"`
	}
	threadIn struct {
		SpaceID int64 `json:"spaceId" jsonschema:"Space id, from search_sessions"`
		After   int64 `json:"after,omitempty" jsonschema:"only messages with a larger id"`
		Root    int64 `json:"root,omitempty" jsonschema:"id of a top-level message: read it and its replies instead; after is ignored"`
	}
	threadOut struct {
		Messages []store.Message `json:"messages"`
	}
	postIn struct {
		Body   string `json:"body" jsonschema:"the message; start a line with @name to hand work over"`
		Parent *int64 `json:"parent,omitempty" jsonschema:"id of a top-level message to reply to in its thread; omit to post at the top level"`
	}
	directIn struct {
		To   string `json:"to" jsonschema:"exactly one bot role"`
		Body string `json:"body" jsonschema:"the private message for that bot"`
	}
	directListIn struct {
		After int64 `json:"after,omitempty" jsonschema:"only direct messages with a larger id"`
	}
	directListOut struct {
		Messages []store.DirectMessage `json:"messages"`
	}
	wrapIn struct {
		Brief  string               `json:"brief" jsonschema:"the full rewritten brief: goal, status, key decisions, open questions"`
		Items  []store.ProposedItem `json:"items,omitempty" jsonschema:"memory items made in this Space that are not saved with remember yet: kind (decision, finding, fact, lesson or convention), body, why, scope (work_item by default, project or bot), replaces (ids of live items it replaces)"`
		Retire []int64              `json:"retire,omitempty" jsonschema:"ids of live memory items that are stale or duplicate"`
	}
	briefIn struct {
		Brief string `json:"brief" jsonschema:"the full brief: goal, status, key decisions, open questions"`
	}
	wrapOut struct {
		ID      int64  `json:"id" jsonschema:"proposal id"`
		Message string `json:"message"`
	}
	createSubItemIn struct {
		Title       string `json:"title" jsonschema:"what the sub-item is, in one line"`
		Description string `json:"description,omitempty" jsonschema:"what to do and how to tell it is done"`
		Type        string `json:"type,omitempty" jsonschema:"a label: feature, bug, incident, investigation, task or improvement; default task"`
	}
	proposeWorkItemIn struct {
		Type         string   `json:"type" jsonschema:"a label: feature, bug, incident, investigation, task or improvement"`
		Title        string   `json:"title" jsonschema:"what the work item is, in one line"`
		Description  string   `json:"description" jsonschema:"what to do and why"`
		Acceptance   []string `json:"acceptance,omitempty" jsonschema:"lines that must hold for it to be done"`
		Owner        string   `json:"owner,omitempty" jsonschema:"name of the Bot you propose as owner, from find_bots; it need not be you"`
		Participants []string `json:"participants,omitempty" jsonschema:"names of the other Bots you propose as participants, from find_bots"`
		Targets      []int64  `json:"targets,omitempty" jsonschema:"ids of other Projects whose repository the work needs, from list_projects"`
	}
	proposeSpaceIn struct {
		Name          string  `json:"name" jsonschema:"what the Space is for, in a few words"`
		BotIDs        []int64 `json:"botIds" jsonschema:"ids of the Bots to put in the Space, from find_bots"`
		CoordinatorID int64   `json:"coordinatorBotId,omitempty" jsonschema:"the Bot that gets messages that mention nobody; default the first Bot"`
	}
	proposeMemberIn struct {
		BotID  int64  `json:"botId" jsonschema:"id of the Bot to add to this Space, from find_bots"`
		Reason string `json:"reason" jsonschema:"why the Space needs this Bot"`
	}
	proposeSkillIn struct {
		Name  string          `json:"name" jsonschema:"the skill's name: letters, digits, :, _ or -; it must match the name in SKILL.md"`
		Files []skillFileView `json:"files" jsonschema:"every file of the skill folder as {path, body}; SKILL.md with name and description in its frontmatter is required"`
	}
	proposeSkillOut struct {
		ID      int64  `json:"id" jsonschema:"draft id"`
		Message string `json:"message"`
	}
	subItemIn struct {
		ID int64 `json:"id" jsonschema:"sub-item id, from list_sub_items"`
	}
	setSubItemStatusIn struct {
		ID     int64  `json:"id" jsonschema:"sub-item id, from list_sub_items"`
		Status string `json:"status" jsonschema:"backlog, queued, in_progress, needs_you, review, done or dropped"`
	}
	subItemRow struct {
		ID     int64  `json:"id"`
		Type   string `json:"type"`
		Title  string `json:"title"`
		Status string `json:"status"`
	}
	listSubItemsOut struct {
		SubItems []subItemRow `json:"subItems"`
	}
	submitForReviewIn struct {
		Lines       []store.LineTick     `json:"lines,omitempty" jsonschema:"acceptance lines to tick, each a position (from 1), checked and a note on how you proved it or why it is not ticked; lines you leave out keep their tick and note"`
		Commands    []store.ProofCommand `json:"commands,omitempty" jsonschema:"the commands you ran as proof, each with its output"`
		Screenshots []string             `json:"screenshots,omitempty" jsonschema:"absolute paths of screenshot files under the Reef data folder"`
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
	findBotsIn struct {
		Query string  `json:"query,omitempty" jsonschema:"words describing the expertise you need; omit to list every other Bot"`
		IDs   []int64 `json:"ids,omitempty" jsonschema:"Bot ids you already hold, to check each still exists"`
	}
	botCard struct {
		ID           int64         `json:"id"`
		Name         string        `json:"name,omitempty"`
		Description  string        `json:"description,omitempty"`
		Missing      bool          `json:"missing,omitempty"`
		Capabilities *capabilities `json:"capabilities,omitempty"`
	}
	capabilities struct {
		Backend        string    `json:"backend"`
		Model          string    `json:"model,omitempty"`
		Write          bool      `json:"write"`
		PermissionMode string    `json:"permissionMode,omitempty"`
		Skills         []string  `json:"skills"`
		Tools          []string  `json:"tools"`
		MCP            []mcpView `json:"mcp"`
	}
	findBotsOut struct {
		Note string    `json:"note"`
		Bots []botCard `json:"bots"`
	}
	browserTextOut struct {
		Text string `json:"text"`
	}
)

// NewMCP returns the MCP server for one bot: tok names the Space it
// works in, the role it speaks as and, through the Space, the work item.
// dataDir is the Reef data folder, where the Reef app's browser socket is.
func NewMCP(st *store.Store, ms *memory.Store, tok store.Token, dataDir string) *mcp.Server {
	m := &mcpServer{st: st, ms: ms, tok: tok, dataDir: dataDir}
	srv := mcp.NewServer(&mcp.Implementation{Name: "reef", Version: "v0.1.0"}, nil)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_brief",
		Description: "Get this work item's brief. Call it first when you start work.",
	}, m.getBrief)
	mcp.AddTool(srv, &mcp.Tool{
		Name: "remember",
		Description: "Remember something for later, with why. A decision is a choice about how " +
			"the work goes, not progress, proof or a won't-fix list. The default scope keeps it " +
			"on this work item, free Space or direct conversation, whichever you are in (a " +
			"conversation item is found only with recall), project on every work item of the project, " +
			"bot on you across work items. Pass replaces when it amends or replaces " +
			"earlier items: they are retired. Shared conventions wait for the user's approval.",
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
		Name: "retire_memory",
		Description: "Retire one of your own bot-scope memory items that is stale or no longer true. " +
			"It is kept, marked retired, and needs no approval. Items of other scopes are refused.",
	}, m.retireMemory)
	mcp.AddTool(srv, &mcp.Tool{
		Name: "search_sessions",
		Description: "Search messages in all of this work item's Spaces, newest first. " +
			"Returns Space and message ids; read more with read_thread.",
	}, m.searchSessions)
	mcp.AddTool(srv, &mcp.Tool{
		Name: "read_thread",
		Description: "Read the messages of one of this work item's Spaces, oldest first. " +
			"Give root, a top-level message id, to read just that message and its replies.",
	}, m.readThread)
	mcp.AddTool(srv, &mcp.Tool{
		Name: "read_space",
		Description: "Read a Space the user referenced with #[name](space:id), including a closed " +
			"one, without waking it: it starts no turn, session or workspace there and adds nobody. " +
			"Returns messages and, with query, memory items, each with its id: cite the message or " +
			"memory id your answer rests on. At most the newest 50 messages come back; more is true " +
			"when older ones were left out, so narrow with query. snapshot is the Space as of snapshot.asOf; " +
			"say it is a snapshot when you report what is happening there. A deleted Space comes back unavailable: " +
			"say so, do not guess what it held.",
	}, m.readSpace)
	mcp.AddTool(srv, &mcp.Tool{
		Name: "find_bots",
		Description: "Find other Bots by expertise, or check that Bot ids you hold still exist. " +
			"Each result has the Bot's stable id, name, description and configured capabilities. " +
			"Finding a Bot adds it to no Space, gives it nothing to do and grants no access: " +
			"recommend it to the user, who decides.",
	}, m.findBots)
	mcp.AddTool(srv, &mcp.Tool{
		Name: "post_to_space",
		Description: "Post a message to this Space as yourself. Start a line with @name to wake " +
			"that bot when your turn ends; a post with no such line wakes no one.",
	}, m.postToSpace)
	mcp.AddTool(srv, &mcp.Tool{
		Name: "send_to_role",
		Description: "Send a private message to exactly one bot. The bot's ordinary " +
			"reply comes back privately once; use send_to_role again to continue.",
	}, m.sendToRole)
	mcp.AddTool(srv, &mcp.Tool{
		Name: "read_direct_messages",
		Description: "Read private messages in this Space that you sent or received; " +
			"the lead's wrap-up turn reads every pair's.",
	}, m.readDirectMessages)
	mcp.AddTool(srv, &mcp.Tool{
		Name: "propose_wrap",
		Description: "Propose how to wrap up this Space; the user approves it before " +
			"anything is saved. brief is the full rewritten brief (goal, status, key " +
			"decisions, open questions). items are memory items made in this Space that " +
			"are not saved with remember yet, each with a kind, body, why, a scope " +
			"(work_item by default; project or bot) and replaces for earlier items it replaces. " +
			"retire lists ids of live memory items that are stale or duplicate.",
	}, m.proposeWrap)
	mcp.AddTool(srv, &mcp.Tool{
		Name: "propose_brief",
		Description: "Propose a new brief for this work item when the user and the team agree " +
			"on its goal, scope or plan; the user approves it before it is saved. brief is " +
			"the full brief (goal, status, key decisions, open questions). The Space goes " +
			"on; use propose_wrap only to end it.",
	}, m.proposeBrief)
	mcp.AddTool(srv, &mcp.Tool{
		Name: "create_sub_item",
		Description: "Add a sub-item to this work item: a piece of its work, in the work item's " +
			"worktree. Sub-items run one turn at a time. The work item goes to review only when " +
			"every sub-item is done or dropped.",
	}, m.createSubItem)
	mcp.AddTool(srv, &mcp.Tool{
		Name: "propose_work_item",
		Description: "Propose a new work item for this project when you find work that is not " +
			"yours to do now. It lands in the Backlog as proposed by you, with the owner and " +
			"participants you name (find_bots finds them; the owner need not be you) and any other " +
			"Projects whose repository it needs. The user may change all of it, then approves it; " +
			"a proposal never starts work.",
	}, m.proposeWorkItem)
	mcp.AddTool(srv, &mcp.Tool{
		Name: "propose_space",
		Description: "Propose a new Space with participants and a coordinator. It lands in the " +
			"user's inbox; the user accepts or changes the selection before the Space is created. " +
			"A proposal creates nothing and grants no access.",
	}, m.proposeSpace)
	if c, err := st.Space(context.Background(), tok.SpaceID); err != nil || (c.Kind != store.KindConsultation && c.Kind != store.KindConversation) {
		mcp.AddTool(srv, &mcp.Tool{
			Name: "propose_member",
			Description: "Propose adding another Bot to this Space. The user approves it in the Space " +
				"before the Bot joins; a proposal adds nobody and grants no access. Handoffs to Bots " +
				"already in the Space need no proposal.",
		}, m.proposeMember)
	}
	mcp.AddTool(srv, &mcp.Tool{
		Name: "propose_skill",
		Description: "Propose a new skill when the user asks you to draft one. It shows in this " +
			"Space and in the skill library as a draft; the user reviews every file and saves or " +
			"rejects it. A draft is not in the library and is not given to any Bot.",
	}, m.proposeSkill)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_sub_items",
		Description: "List this work item's sub-items with their status, oldest first.",
	}, m.listSubItems)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "read_sub_item",
		Description: "Read one sub-item in full: description, status and acceptance lines.",
	}, m.readSubItem)
	mcp.AddTool(srv, &mcp.Tool{
		Name: "set_sub_item_status",
		Description: "Set the status of a sub-item: backlog, queued, in_progress, needs_you, " +
			"review, done or dropped. Only the lead may; the lead is the one that sets it done.",
	}, m.setSubItemStatus)
	mcp.AddTool(srv, &mcp.Tool{
		Name: "submit_for_review",
		Description: "Submit this work item for the user's review with your proof: tick its " +
			"acceptance lines with a note each, add the commands you ran with their output and " +
			"screenshot files. Only the lead may, and only while every sub-item is done or " +
			"dropped. The work item goes to review, its team is free for the next queued item, " +
			"and when your turn ends you are asked to wrap up this Space.",
	}, m.submitForReview)
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
	m.registerProjectTools(srv)
	if c, err := st.Space(context.Background(), tok.SpaceID); err == nil && c.WorkItemID == nil && c.Kind != store.KindConsultation {
		m.registerOpenWorkspace(srv)
	}
	if c, err := st.Space(context.Background(), tok.SpaceID); err == nil && c.Kind == store.KindConversation {
		m.registerConsult(srv)
		m.registerSendToSpace(srv)
	}
	if tok.Review {
		restrictToReview(srv)
	}
	return srv
}

func (m *mcpServer) currentName(ctx context.Context) (string, error) {
	if m.tok.BotID == nil {
		return "", fmt.Errorf("%w: token has no Bot ID", store.ErrInvalid)
	}
	team, err := m.st.SpaceTeam(ctx, m.tok.SpaceID)
	if err != nil {
		return "", err
	}
	for _, role := range team.Roles {
		if role.BotID == *m.tok.BotID {
			return role.Name, nil
		}
	}
	return "", fmt.Errorf("%w: token Bot is not a Space member", store.ErrInvalid)
}

func (m *mcpServer) workItem(ctx context.Context) (store.WorkItem, error) {
	c, err := m.st.Space(ctx, m.tok.SpaceID)
	if err != nil {
		return store.WorkItem{}, err
	}
	if c.WorkItemID == nil {
		return store.WorkItem{}, errors.New("this Space has no work item")
	}
	return m.st.WorkItem(ctx, *c.WorkItemID)
}

func (m *mcpServer) getBrief(
	ctx context.Context, _ *mcp.CallToolRequest, _ struct{},
) (*mcp.CallToolResult, briefOut, error) {
	f, err := m.workItem(ctx)
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
	name, err := m.currentName(ctx)
	if err != nil {
		return nil, rememberOut{}, err
	}
	c, err := m.st.Space(ctx, m.tok.SpaceID)
	if err != nil {
		return nil, rememberOut{}, err
	}
	scopes, err := m.withProjects(ctx, space.BotScopes(c, *m.tok.BotID))
	if err != nil {
		return nil, rememberOut{}, err
	}
	if m.tok.Review && in.Scope != memory.ScopeBot {
		return nil, rememberOut{}, fmt.Errorf("%w: a memory review remembers only with scope bot", store.ErrInvalid)
	}
	switch in.Scope {
	case "":
		in.Scope = scopes[0].Kind
	case memory.ScopeWorkItem, memory.ScopeSpace, memory.ScopeConversation, memory.ScopeProject,
		memory.ScopeBot:
	default:
		return nil, rememberOut{}, fmt.Errorf(
			"%w: scope %q is not work_item, space, conversation, project or bot",
			store.ErrInvalid, in.Scope)
	}
	if in.Scope == memory.ScopeProject && in.ProjectID == 0 {
		var ids []string
		for _, r := range scopes {
			if r.Kind == memory.ScopeProject {
				ids = append(ids, r.ID)
			}
		}
		if len(ids) > 1 {
			return nil, rememberOut{}, fmt.Errorf(
				"%w: this Space has several Projects (%s); say which with projectId",
				store.ErrInvalid, strings.Join(ids, ", "))
		}
	}
	i := slices.IndexFunc(scopes, func(r memory.Ref) bool {
		return r.Kind == in.Scope && (in.Scope != memory.ScopeProject || in.ProjectID == 0 ||
			r.ID == strconv.FormatInt(in.ProjectID, 10))
	})
	if i < 0 && in.Scope == memory.ScopeWorkItem {
		return nil, rememberOut{}, errors.New("this Space has no work item")
	}
	if i < 0 {
		return nil, rememberOut{}, fmt.Errorf("%w: you have no %q scope here", store.ErrInvalid, in.Scope)
	}
	about := make([]memory.Ref, 0, len(in.About))
	for _, a := range in.About {
		ref, err := memory.ParseRef(a)
		if err != nil {
			return nil, rememberOut{}, fmt.Errorf("about: %w", err)
		}
		about = append(about, ref)
	}
	for _, id := range in.Replaces {
		if _, err := m.readable(ctx, scopes, id); err != nil {
			return nil, rememberOut{}, err
		}
	}
	res, err := m.ms.Remember(ctx,
		memory.Writer{
			Author: name, Role: name, BotID: m.tok.BotID, Scope: scopes[i],
			Links: []memory.Link{{
				Rel: memory.RelSource, ToKind: "space", ToID: strconv.FormatInt(m.tok.SpaceID, 10),
			}},
		},
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
	_, err := m.currentName(ctx)
	if err != nil {
		return nil, recallOut{}, err
	}
	if in.Query == "" {
		return nil, recallOut{}, errors.New("query is required")
	}
	c, err := m.st.Space(ctx, m.tok.SpaceID)
	if err != nil {
		return nil, recallOut{}, err
	}
	scopes, err := space.ReadBotScopes(ctx, m.st, c, *m.tok.BotID)
	if err == nil {
		scopes, err = m.withProjects(ctx, scopes)
	}
	if err != nil {
		return nil, recallOut{}, err
	}
	if in.Scope != "" {
		scopes = slices.DeleteFunc(scopes, func(r memory.Ref) bool { return r.Kind != in.Scope })
		if len(scopes) == 0 {
			return nil, recallOut{}, fmt.Errorf("%w: you have no %q scope here", store.ErrInvalid, in.Scope)
		}
	}
	q := memory.Query{Scopes: scopes, Text: in.Query, Limit: in.Limit}
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
	_, err := m.currentName(ctx)
	if err != nil {
		return nil, itemOut{}, err
	}
	c, err := m.st.Space(ctx, m.tok.SpaceID)
	if err != nil {
		return nil, itemOut{}, err
	}
	scopes, err := space.ReadBotScopes(ctx, m.st, c, *m.tok.BotID)
	if err == nil {
		scopes, err = m.withProjects(ctx, scopes)
	}
	if err != nil {
		return nil, itemOut{}, err
	}
	it, err := m.readable(ctx, scopes, in.ID)
	if err != nil {
		return nil, itemOut{}, err
	}
	return nil, toItemOut(it), nil
}

func (m *mcpServer) retireMemory(
	ctx context.Context, _ *mcp.CallToolRequest, in readMemoryIn,
) (*mcp.CallToolResult, itemOut, error) {
	if _, err := m.currentName(ctx); err != nil {
		return nil, itemOut{}, err
	}
	own := memory.Ref{Kind: memory.ScopeBot, ID: strconv.FormatInt(*m.tok.BotID, 10)}
	if _, err := m.readable(ctx, []memory.Ref{own}, in.ID); err != nil {
		return nil, itemOut{}, err
	}
	if err := m.ms.Retire(ctx, in.ID); err != nil {
		return nil, itemOut{}, err
	}
	it, err := m.ms.Read(ctx, in.ID)
	if err != nil {
		return nil, itemOut{}, err
	}
	return nil, toItemOut(it), nil
}

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

func (m *mcpServer) searchSessions(
	ctx context.Context, _ *mcp.CallToolRequest, in searchIn,
) (*mcp.CallToolResult, searchOut, error) {
	if _, err := m.currentName(ctx); err != nil {
		return nil, searchOut{}, err
	}
	f, err := m.workItem(ctx)
	if err != nil {
		return nil, searchOut{}, err
	}
	if in.Query == "" {
		return nil, searchOut{}, errors.New("query is required")
	}
	if in.Limit <= 0 {
		in.Limit = _defaultSearchLimit
	}
	if m.tok.BotID == nil {
		return nil, searchOut{}, fmt.Errorf("token has no Bot ID: %w", store.ErrInvalid)
	}
	hits, err := m.st.SearchHistory(ctx, f.ID, in.Query, in.Limit, m.tok.BotID)
	return nil, searchOut{Hits: hits}, err
}

func (m *mcpServer) readThread(
	ctx context.Context, _ *mcp.CallToolRequest, in threadIn,
) (*mcp.CallToolResult, threadOut, error) {
	if _, err := m.currentName(ctx); err != nil {
		return nil, threadOut{}, err
	}
	c, err := m.st.Space(ctx, in.SpaceID)
	if err != nil {
		return nil, threadOut{}, err
	}
	if c.ID != m.tok.SpaceID {
		f, err := m.workItem(ctx)
		if err != nil {
			return nil, threadOut{}, err
		}
		if c.WorkItemID == nil || *c.WorkItemID != f.ID {
			return nil, threadOut{}, fmt.Errorf("space %d: %w", in.SpaceID, store.ErrNotFound)
		}
	}
	if in.Root != 0 {
		msgs, err := m.st.Thread(ctx, c.ID, in.Root)
		return nil, threadOut{Messages: msgs}, err
	}
	msgs, err := m.st.Messages(ctx, c.ID, in.After)
	return nil, threadOut{Messages: msgs}, err
}

func (m *mcpServer) postToSpace(
	ctx context.Context, _ *mcp.CallToolRequest, in postIn,
) (*mcp.CallToolResult, store.Message, error) {
	name, err := m.currentName(ctx)
	if err != nil {
		return nil, store.Message{}, err
	}
	ch, err := m.st.Space(ctx, m.tok.SpaceID)
	if err != nil {
		return nil, store.Message{}, err
	}
	if err := ch.Writable(); err != nil {
		return nil, store.Message{}, err
	}
	msg, err := m.st.CreateMessage(ctx, store.Message{
		SpaceID: m.tok.SpaceID, BotID: m.tok.BotID, ParentID: in.Parent, Author: name, Body: in.Body,
	})
	return nil, msg, err
}

func (m *mcpServer) sendToRole(
	ctx context.Context, _ *mcp.CallToolRequest, in directIn,
) (*mcp.CallToolResult, store.DirectMessage, error) {
	name, err := m.currentName(ctx)
	if err != nil {
		return nil, store.DirectMessage{}, err
	}
	ch, err := m.st.Space(ctx, m.tok.SpaceID)
	if err != nil {
		return nil, store.DirectMessage{}, err
	}
	if err := ch.Writable(); err != nil {
		return nil, store.DirectMessage{}, err
	}
	if ch.ActiveTurn == store.ActiveTurnStarting {
		return nil, store.DirectMessage{}, fmt.Errorf("space %d is starting: %w", ch.ID, store.ErrConflict)
	}
	if ch.ActiveTurn == store.ActiveTurnWrap {
		return nil, store.DirectMessage{}, fmt.Errorf("space %d is wrapping: %w", ch.ID, store.ErrConflict)
	}
	running, err := m.st.Turns(ctx, store.TurnFilter{SpaceID: &ch.ID, Status: "running"})
	if err != nil {
		return nil, store.DirectMessage{}, err
	}
	var source *int64
	sending := false
	for _, t := range running {
		if t.WakeText == "" {
			return nil, store.DirectMessage{}, fmt.Errorf("space %d is wrapping or promoting: %w", ch.ID, store.ErrConflict)
		}
		if t.BotID == nil || *t.BotID != *m.tok.BotID {
			continue
		}
		sending = true
		if t.DirectMessageID == nil {
			source = t.WakeMessageID
			continue
		}
		woken, err := m.st.DirectMessage(ctx, *t.DirectMessageID)
		if err != nil {
			return nil, store.DirectMessage{}, err
		}
		source = woken.SourceMessageID
	}
	if !sending {
		return nil, store.DirectMessage{}, fmt.Errorf("%w: %s has no running turn to send from", store.ErrInvalid, name)
	}
	team, err := m.st.SpaceTeam(ctx, ch.ID)
	if err != nil {
		return nil, store.DirectMessage{}, err
	}
	var recipient *int64
	for _, role := range team.Roles {
		if role.Name == in.To {
			id := role.BotID
			recipient = &id
			break
		}
	}
	if recipient == nil {
		return nil, store.DirectMessage{}, fmt.Errorf("%w: role %q is not in this Space's team", store.ErrInvalid, in.To)
	}
	if *recipient == *m.tok.BotID {
		return nil, store.DirectMessage{}, fmt.Errorf("%w: cannot send a direct message to yourself", store.ErrInvalid)
	}
	dm, err := m.st.CreateDirectMessage(ctx, store.DirectMessage{
		SpaceID: m.tok.SpaceID, SourceMessageID: source, From: name, To: in.To, FromBotID: m.tok.BotID, ToBotID: recipient, Body: in.Body,
	})
	return nil, dm, err
}

func (m *mcpServer) readDirectMessages(
	ctx context.Context, _ *mcp.CallToolRequest, in directListIn,
) (*mcp.CallToolResult, directListOut, error) {
	if _, err := m.currentName(ctx); err != nil {
		return nil, directListOut{}, err
	}
	ch, err := m.st.Space(ctx, m.tok.SpaceID)
	if err != nil {
		return nil, directListOut{}, err
	}
	var msgs []store.DirectMessage
	lead, err := m.isLead(ctx)
	if err != nil {
		return nil, directListOut{}, err
	}
	if ch.ActiveTurn == store.ActiveTurnWrap && lead {
		msgs, err = m.st.DirectMessages(ctx, ch.ID, in.After)
	} else {
		if m.tok.BotID == nil {
			return nil, directListOut{}, fmt.Errorf("token has no Bot ID: %w", store.ErrInvalid)
		}
		msgs, err = m.st.DirectMessagesForBot(ctx, ch.ID, *m.tok.BotID, in.After)
	}
	if msgs == nil {
		msgs = []store.DirectMessage{}
	}
	return nil, directListOut{Messages: msgs}, err
}

func (m *mcpServer) proposeWrap(
	ctx context.Context, _ *mcp.CallToolRequest, in wrapIn,
) (*mcp.CallToolResult, wrapOut, error) {
	name, err := m.currentName(ctx)
	if err != nil {
		return nil, wrapOut{}, err
	}
	if _, err := m.workItem(ctx); err != nil {
		return nil, wrapOut{}, err
	}
	c, err := m.st.Space(ctx, m.tok.SpaceID)
	if err != nil {
		return nil, wrapOut{}, err
	}
	scopes := space.BotScopes(c, *m.tok.BotID)
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
		if err := requireLive(ctx, m.ms, scopes[at:at+1], it.Replaces); err != nil {
			return nil, wrapOut{}, err
		}
	}
	if err := requireLive(ctx, m.ms, scopes, in.Retire); err != nil {
		return nil, wrapOut{}, err
	}
	p, err := m.st.CreateProposal(ctx, store.Proposal{
		SpaceID: m.tok.SpaceID, BotID: m.tok.BotID, Role: name, Brief: in.Brief, Items: in.Items, Retire: in.Retire,
	})
	if err != nil {
		return nil, wrapOut{}, err
	}
	return nil, wrapOut{ID: p.ID, Message: "proposal saved; the user will approve it"}, nil
}

func (m *mcpServer) isLead(ctx context.Context) (bool, error) {
	t, err := m.st.SpaceTeam(ctx, m.tok.SpaceID)
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(t.Roles, func(r store.Role) bool {
		return r.Lead && m.tok.BotID != nil && r.BotID == *m.tok.BotID
	}), nil
}

func (m *mcpServer) createSubItem(
	ctx context.Context, _ *mcp.CallToolRequest, in createSubItemIn,
) (*mcp.CallToolResult, store.WorkItem, error) {
	name, err := m.currentName(ctx)
	if err != nil {
		return nil, store.WorkItem{}, err
	}
	f, err := m.workItem(ctx)
	if err != nil {
		return nil, store.WorkItem{}, err
	}
	w, err := m.st.CreateWorkItem(ctx, store.WorkItem{
		ProjectID: f.ProjectID, ParentID: &f.ID, Type: cmp.Or(in.Type, "task"),
		Title: in.Title, Description: in.Description,
	})
	if err != nil {
		return nil, store.WorkItem{}, err
	}
	text := fmt.Sprintf("%s added sub-item %d to work item %d: %s", name, w.ID, f.ID, w.Title)
	if err := m.st.AddFYI(ctx, store.KindSubItem, w.ID, text); err != nil {
		return nil, store.WorkItem{}, err
	}
	return nil, w, nil
}

func (m *mcpServer) proposeWorkItem(
	ctx context.Context, _ *mcp.CallToolRequest, in proposeWorkItemIn,
) (*mcp.CallToolResult, store.WorkItem, error) {
	name, err := m.currentName(ctx)
	if err != nil {
		return nil, store.WorkItem{}, err
	}
	c, err := m.st.Space(ctx, m.tok.SpaceID)
	if err != nil {
		return nil, store.WorkItem{}, err
	}
	if c.ProjectID == 0 {
		return nil, store.WorkItem{}, errors.New("this Space has no project, so it cannot propose a work item")
	}
	w := store.WorkItem{
		ProjectID: c.ProjectID, Type: in.Type, Title: in.Title, Description: in.Description,
		ProposedBy: name,
	}
	for _, text := range in.Acceptance {
		w.AcceptanceLines = append(w.AcceptanceLines, store.AcceptanceLine{Text: text})
	}
	for _, id := range in.Targets {
		w.Targets = append(w.Targets, store.Target{ProjectID: id})
	}
	proposal := ""
	if in.Owner != "" || len(in.Participants) > 0 {
		bots, err := m.st.Bots(ctx)
		if err != nil {
			return nil, store.WorkItem{}, err
		}
		if in.Owner != "" {
			b, err := findBot(bots, in.Owner)
			if err != nil {
				return nil, store.WorkItem{}, err
			}
			w.OwnerBotID = &b.ID
			proposal = " (proposed owner " + b.Name + ")"
		}
		for _, ref := range in.Participants {
			b, err := findBot(bots, ref)
			if err != nil {
				return nil, store.WorkItem{}, err
			}
			w.ParticipantBotIDs = append(w.ParticipantBotIDs, b.ID)
		}
		if len(w.ParticipantBotIDs) > 0 && w.OwnerBotID == nil {
			return nil, store.WorkItem{}, errors.New("participants need an owner")
		}
	}
	w, err = m.st.CreateWorkItem(ctx, w)
	if err != nil {
		return nil, store.WorkItem{}, err
	}
	text := fmt.Sprintf("%s proposed work item %d: %s%s. It is in the Backlog until you approve it.",
		name, w.ID, w.Title, proposal)
	if err := m.st.AddFYI(ctx, store.KindProposed, w.ID, text); err != nil {
		return nil, store.WorkItem{}, err
	}
	return nil, w, nil
}

func (m *mcpServer) proposeSpace(
	ctx context.Context, _ *mcp.CallToolRequest, in proposeSpaceIn,
) (*mcp.CallToolResult, store.SpaceProposal, error) {
	name, err := m.currentName(ctx)
	if err != nil {
		return nil, store.SpaceProposal{}, err
	}
	p, err := m.st.CreateSpaceProposal(ctx, store.SpaceProposal{
		ProposedBy: name, Name: in.Name, BotIDs: in.BotIDs, CoordinatorBotID: in.CoordinatorID,
	})
	return nil, p, err
}

func (m *mcpServer) proposeMember(
	ctx context.Context, _ *mcp.CallToolRequest, in proposeMemberIn,
) (*mcp.CallToolResult, store.MemberProposal, error) {
	name, err := m.currentName(ctx)
	if err != nil {
		return nil, store.MemberProposal{}, err
	}
	p, err := m.st.CreateMemberProposal(ctx, m.tok.SpaceID, name, in.BotID, in.Reason)
	return nil, p, err
}

func (m *mcpServer) proposeSkill(
	ctx context.Context, _ *mcp.CallToolRequest, in proposeSkillIn,
) (*mcp.CallToolResult, proposeSkillOut, error) {
	if _, err := m.currentName(ctx); err != nil {
		return nil, proposeSkillOut{}, err
	}
	p, err := buildSkillPackage(in.Name, in.Files)
	if err != nil {
		return nil, proposeSkillOut{}, err
	}
	d, err := m.st.CreateSkillDraft(ctx, m.tok.SpaceID, *m.tok.BotID, p)
	if err != nil {
		return nil, proposeSkillOut{}, err
	}
	return nil, proposeSkillOut{ID: d.ID, Message: "draft saved; the user will review it"}, nil
}

func (m *mcpServer) findBots(
	ctx context.Context, _ *mcp.CallToolRequest, in findBotsIn,
) (*mcp.CallToolResult, findBotsOut, error) {
	bots, err := m.st.Bots(ctx)
	if err != nil {
		return nil, findBotsOut{}, err
	}
	connections, err := m.st.Connections(ctx)
	if err != nil {
		return nil, findBotsOut{}, err
	}
	out := findBotsOut{
		Note: "Capabilities are what each Bot is configured with, not permission for you or anyone to use them.",
		Bots: []botCard{},
	}
	card := func(b store.Bot) botCard {
		return botCard{ID: b.ID, Name: b.Name, Description: b.Description, Capabilities: &capabilities{
			Backend: b.Backend, Model: b.Model, Write: b.Write, PermissionMode: b.PermissionMode,
			Skills: nonNilStrings(b.Skills), Tools: nonNilStrings(b.Tools), MCP: viewMCP(selected(connections, b.Connections)),
		}}
	}
	if len(in.IDs) > 0 {
		for _, id := range in.IDs {
			i := slices.IndexFunc(bots, func(b store.Bot) bool { return b.ID == id })
			if i < 0 {
				out.Bots = append(out.Bots, botCard{ID: id, Missing: true})
				continue
			}
			out.Bots = append(out.Bots, card(bots[i]))
		}
		return nil, out, nil
	}
	words := strings.Fields(strings.ToLower(in.Query))
	type hit struct {
		bot   store.Bot
		score int
	}
	var hits []hit
	for _, b := range bots {
		if m.tok.BotID != nil && b.ID == *m.tok.BotID {
			continue
		}
		text := strings.ToLower(b.Name + " " + b.Description + " " + strings.Join(b.Skills, " "))
		score := 0
		for _, w := range words {
			if strings.Contains(text, w) {
				score++
			}
		}
		if len(words) == 0 || score > 0 {
			hits = append(hits, hit{b, score})
		}
	}
	slices.SortStableFunc(hits, func(a, b hit) int { return b.score - a.score })
	for _, h := range hits {
		out.Bots = append(out.Bots, card(h.bot))
	}
	return nil, out, nil
}

// findBot is the Bot whose name is ref, ignoring case, or whose id is ref.
func findBot(bots []store.Bot, ref string) (store.Bot, error) {
	if i := slices.IndexFunc(bots, func(b store.Bot) bool { return strings.EqualFold(b.Name, ref) }); i >= 0 {
		return bots[i], nil
	}
	if i := slices.IndexFunc(bots, func(b store.Bot) bool { return strconv.FormatInt(b.ID, 10) == ref }); i >= 0 {
		return bots[i], nil
	}
	return store.Bot{}, fmt.Errorf("%w: no Bot %q; use a name or id from find_bots", store.ErrInvalid, ref)
}

func selected(all []store.Connection, ids []int64) []store.Connection {
	var out []store.Connection
	for _, c := range all {
		if slices.Contains(ids, c.ID) {
			out = append(out, c)
		}
	}
	return out
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func (m *mcpServer) listSubItems(
	ctx context.Context, _ *mcp.CallToolRequest, _ struct{},
) (*mcp.CallToolResult, listSubItemsOut, error) {
	f, err := m.workItem(ctx)
	if err != nil {
		return nil, listSubItemsOut{}, err
	}
	subs, err := m.st.WorkItems(ctx, store.WorkItemFilter{ParentID: &f.ID})
	if err != nil {
		return nil, listSubItemsOut{}, err
	}
	out := listSubItemsOut{SubItems: make([]subItemRow, 0, len(subs))}
	for _, w := range subs {
		out.SubItems = append(out.SubItems, subItemRow{ID: w.ID, Type: w.Type, Title: w.Title, Status: w.Status})
	}
	return nil, out, nil
}

func (m *mcpServer) readSubItem(
	ctx context.Context, _ *mcp.CallToolRequest, in subItemIn,
) (*mcp.CallToolResult, store.WorkItem, error) {
	w, err := m.subItem(ctx, in.ID)
	return nil, w, err
}

func (m *mcpServer) setSubItemStatus(
	ctx context.Context, _ *mcp.CallToolRequest, in setSubItemStatusIn,
) (*mcp.CallToolResult, store.WorkItem, error) {
	lead, err := m.isLead(ctx)
	if err != nil {
		return nil, store.WorkItem{}, err
	}
	if !lead {
		return nil, store.WorkItem{}, fmt.Errorf("%w: only the lead sets a sub-item's status", store.ErrInvalid)
	}
	if _, err := m.subItem(ctx, in.ID); err != nil {
		return nil, store.WorkItem{}, err
	}
	w, err := m.st.MoveWorkItem(ctx, in.ID, in.Status, store.ByLead)
	return nil, w, err
}

func (m *mcpServer) submitForReview(
	ctx context.Context, _ *mcp.CallToolRequest, in submitForReviewIn,
) (*mcp.CallToolResult, store.WorkItem, error) {
	lead, err := m.isLead(ctx)
	if err != nil {
		return nil, store.WorkItem{}, err
	}
	if !lead {
		return nil, store.WorkItem{}, fmt.Errorf("%w: only the lead submits a work item for review", store.ErrInvalid)
	}
	f, err := m.workItem(ctx)
	if err != nil {
		return nil, store.WorkItem{}, err
	}
	for _, p := range in.Screenshots {
		if err := m.checkScreenshot(p); err != nil {
			return nil, store.WorkItem{}, err
		}
	}
	w, err := m.st.SubmitForReview(ctx, f.ID, in.Lines, store.Proof{
		SpaceID: m.tok.SpaceID, Commands: in.Commands, Screenshots: in.Screenshots,
	})
	return nil, w, err
}

// checkScreenshot is ErrInvalid unless path is an existing file under the Reef data folder.
func (m *mcpServer) checkScreenshot(path string) error {
	rel, err := filepath.Rel(m.dataDir, path)
	if err != nil || !filepath.IsAbs(path) || !filepath.IsLocal(rel) {
		return fmt.Errorf("%w: screenshot %q is not under the data folder %s", store.ErrInvalid, path, m.dataDir)
	}
	if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("%w: screenshot %q is not a file", store.ErrInvalid, path)
	}
	return nil
}

// subItem is the sub-item id of this Space's work item; any other work item is not found.
func (m *mcpServer) subItem(ctx context.Context, id int64) (store.WorkItem, error) {
	f, err := m.workItem(ctx)
	if err != nil {
		return store.WorkItem{}, err
	}
	w, err := m.st.WorkItem(ctx, id)
	if err != nil {
		return store.WorkItem{}, err
	}
	if w.ParentID == nil || *w.ParentID != f.ID {
		return store.WorkItem{}, fmt.Errorf("sub-item %d: %w", id, store.ErrNotFound)
	}
	return w, nil
}

func (m *mcpServer) proposeBrief(
	ctx context.Context, _ *mcp.CallToolRequest, in briefIn,
) (*mcp.CallToolResult, wrapOut, error) {
	name, err := m.currentName(ctx)
	if err != nil {
		return nil, wrapOut{}, err
	}
	if _, err := m.workItem(ctx); err != nil {
		return nil, wrapOut{}, err
	}
	p, err := m.st.CreateProposal(ctx, store.Proposal{
		SpaceID: m.tok.SpaceID, BotID: m.tok.BotID, Role: name, Kind: store.ProposalBrief, Brief: in.Brief,
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

func toItemOut(it memory.Item) itemOut {
	links := make([]linkOut, 0, len(it.Links))
	for _, l := range it.Links {
		links = append(links, linkOut{Rel: l.Rel, To: l.ToKind + ":" + l.ToID})
	}
	return itemOut{
		ID: it.ID, Kind: it.Kind, Body: it.Body, Why: it.Why, Scope: refString(it.Scope),
		Author: it.Author, Role: it.Role, BotID: it.BotID, Status: it.Status, RetiredAt: it.RetiredAt,
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
