# Reef: product

## Vision

Reef is a personal Mac app where a team of AI agents works on your projects in one window, and every Work item keeps its own memory.

**The problem.** Big pieces of work get split across sessions: plan in one, implement in another. Today the context is carried by hand, through a handoff file the agent writes so the next session can resume. Sessions pile up per project, and the history of why things were done is lost.

**The idea.** Use persistent conversations for discussion and Work items for tracked work. A Work item holds its acceptance lines, sub-items, decisions, Spaces and proof. Bots collaborate in a Space and retrieve relevant context as needed.

**Why a team, not one agent.** Two reasons beyond memory. Visibility: a single Claude Code session is a black box; separate roles in a Space show who is doing what. One place: Spaces, the board, PRs and the browser live in one window, so there is no switching between tools.

**Relation to Jellyfish.** Jellyfish is the Go agent harness (a learning project on agents and MCP). Reef is the home where agents live and work together. Reef orchestrates Claude Code and Codex; Jellyfish can plug in as another backend.

**Inspiration.** T3 Code (pingdotgg/t3code): a server that runs coding agents, a web UI, and an Electron desktop shell.

## Core entities

The target model below is settled. The runtime already has Bots, one global profile each, and Crews (the `team` table; a Team and a Crew are the same thing); Bot ids sit in Team and Space rosters. The numbered decisions override historical descriptions below where they conflict. See [server](server.md), [memory](memory.md) and [agents](agents.md) for current behavior.

| Entity | Responsibility |
| --- | --- |
| Project | One registered repository, with basic discovery metadata and an intended base. |
| Bot | A global, stable identity and one authoritative configuration, with personal memory. |
| Crew | An optional reusable Bot roster and coordinator suggestion; no queue, memory or Bot overrides. |
| Space | A persistent conversation with participants and a coordinator; it may be free or associated with tracked work. |
| Personal chat | A persistent conversation with one Bot; one Bot can have many chats. |
| Work item | One coherent tracked assignment, with owner, participants, brief, acceptance lines and proof; it may span repositories. |
| Agent session | A Bot's replaceable execution context within a conversation. |
| Memory item | Scoped knowledge, with retained source and retirement history. |
| Channel | An incoming event stream, not a Space; automation policy remains future work. |

Conversations have zero or more Project associations. A Project filter organizes them; it does not own or copy them. A Work item may span Projects, while each Project represents one repository.

> The sections through the decision table describe the existing runtime and earlier design. The target decisions 143–155 supersede conflicting Team, queue, and project-ownership rules; Teammate is now Bot.

## Work items

**What one holds.** A description and acceptance lines (the things that must be true for it to be done) · a brief, top-level items only · sub-items · its Spaces · memory items · a worktree, branch and PR · the proof submitted for Review.

**Type** is a label for the icon and the board filter: `feature`, `bug`, `incident`, `investigation`, `task` or `improvement`. It changes no behaviour.

**Status.** Backlog, Queued, In progress, Needs you, Review, Done and Dropped. Who moves an item:

- You: Backlog and Queued both ways, Review to Done (top-level only) or back to Queued with a note, and any status to Dropped. Dropped is final.
- The Team's queue: Queued to In progress.
- The Space: In progress to Needs you when it pauses, and back when it continues.
- The lead: sub-items only, to any status. It is the one that sets a sub-item Done. `submit_for_review` moves the top-level item to Review.

Only you set a top-level item Done. A parent goes to Review only when every sub-item is Done or Dropped.

**Starting work.** You create an item on the board, from the "+" menu or with capture, or a Bot proposes one (`propose_work_item`): it lands in Backlog marked "proposed by X" and an FYI tells you. Nothing runs until you approve it with an owner.

**Queue.** An item starts as soon as you approve it with an owner: nothing limits how many items run at once. The server opens the item's Space led by its owner, with the other participants as members, and sends it the item. An item with no owner cannot be approved. An item you send back from Review starts again in its Space.

**Sub-items.** Any Bot can create a sub-item without asking. It shares its parent's owner, participants and repositories, shares the parent's Space and worktree, and shows as an FYI. Sub-items run one turn at a time; subagents inside one turn are fine. True parallel work is a later slice.

**Review.** The owner submits an item with the proof: the PR (found from the branch), each acceptance line ticked with a note, test commands with their output, and screenshots. Submitting also starts the wrap-up, so the brief and memory items wait for you next to the proof. Setting Done approves the wrap-up. Sending back needs a note, which rejects the wrap-up and goes to the owner.

**Closing.** Closing a Work item closes all its Spaces; reopening restores the item only.

**Later.** Ask a Bot (a bounded question from one Bot to another), Channels and Automation, Space continuation when one grows too long, a cap on sub-items, and versions of a Work item are not built.

## Teams and Spaces

A Space is where talk happens. Inside it each member is its own agent session, and the Reef server routes messages between them.

**Kinds.**
- A Work item's Space. A Work item has one active Space. Phases (design, build, test) are threads inside it or sub-items, never a Space each, because splitting phases splits context.
- A free Space: no Work item, running on the main checkout. Create one with "new Space" and a Team. Attach it to a Work item later: it moves to the item's worktree and its Space memory moves to the item.
- A direct conversation: one Bot, no Team and no Work item. A Bot can have many; continue any of them. They run with the Bot's profile.

**Members.** A Space has its own member list: Bot ids and a lead flag, copied from the Team or the Bot. A member carries no name, tools, MCP servers, permissions or backend of its own: a turn reads the Bot's profile, and a Space has no backend or permission setting. Editing the Team later does not change a running Space's roster.

**Adding a Bot.** In a direct conversation, `@Y` at the start of a line or "add Bot" turns it into a free Space with you, X and Y. The conversation is marked moved and linked. Y gets a summary the lead writes, never the transcript.

**The lead.** One member per Team answers by default. You talk to it naturally; it clarifies and pulls in specialists. Specialists post directly in the Space; the lead does not summarize them back and only speaks when addressed or @mentioned.

**Routing rules** (plain code, not an LLM):

1. A user reply inside a thread goes to whoever owns that thread
2. An @mention at the start of a message or line goes to that bot
3. Anything else from the user goes to the lead

A Team's responsibility text only suggests a Team for a proposed Work item. It routes nothing else.

**How bots talk.** A bot uses `post_to_space` for progress, decisions, blockers and outcomes the whole team needs. It uses `send_to_role` for detailed one-to-one coordination. A direct request wakes one role; that role's ordinary reply returns directly once. Handling the response does not reply again automatically, so another exchange requires an explicit `send_to_role`. Direct exchanges stay under the Space and are inspectable by the user, while only participating roles can read them through Reef tools.

**Example flow.** You queue "Google login" on a Team. The queue starts it and the lead asks @architect for a plan → plan posted as a thread, you reply in the thread with questions → architect hands off to @developer → developer asks @architect a doubt → developer pings @tester → tester reports back → the lead submits the item for Review with the proof.

**Handoffs.** Work passes directly between roles. Caps may pause a Space, and checkpoints let you roll work back. There are no approval gates: Review with proof checks the result.

**Caps** (plain code): max talk-only agent-to-agent hops without a user message (default 6; a turn that calls any tool other than Reef's own resets it) and a token budget per top-level Work item (default 1M, shared by all its Spaces; Continue adds 1M). No cap on tool calls per turn. On a cap the Space pauses, its Work item moves to Needs you, and an item appears in the inbox.

**Crash recovery.** Every turn is recorded as running, done or failed. On restart, running turns are marked failed and a note is posted to the Space; the worktree may be dirty. No automatic retry; you retry or roll back to a checkpoint.

**Bot identity.** Each member connects to Reef's MCP server with its own token, issued when its agent starts. One server, any number of teams and roles.

## Context and memory

Never preload history, only a summary; details are fetched on demand.

**What a bot wakes up with:**

| Layer | Contents | When loaded |
| --- | --- | --- |
| Role | Description, skills, tools, MCPs | Always |
| Brief | Goal, status, key decisions, open questions; rewritten after each session | Always |
| Memory items | Every live decision of the Work item, and the newest findings, project, team and bot items, each part with how many exist | Always |
| Index | One line per sub-item in progress, and a count of open sub-items and earlier Spaces | Always |
| Details | Full memory items, old sub-items, old chats, PR diffs, Space threads | On demand via tools |

One assembler builds the first prompt of every Space in three sections: `## You` (description, skills, role, Work item, project, the user, description, base branch), `## How to work` (team rules, one line on the tools) and `## What we know` (brief, sub-items in progress, memory items, one count line). A part with nothing in it is left out (decisions 85 to 87, 91). A free Space has no brief; a direct conversation loads only the Bot's own items.

**Memory scopes.** Project, Work item, Team, Bot, Space and conversation. Space items live on a free Space and move to the Work item when it is attached. Conversation items are searchable but never loaded at wake.

**Reef as an MCP server.** The on-demand tools (recall, read memory, search sessions, read thread) are exposed by Reef's own MCP server, so Claude Code and Codex query history the same way. The same server holds the sub-item tools, `propose_work_item` and `submit_for_review`.

**Browser tools.** The same MCP server lets bots drive the built-in browser: open and navigate, read the page (accessibility snapshot), click, type, take screenshots, read console and network logs. You watch live in the browser pane and can take over at any time. The tester uses this to verify UI changes.

**Session lifetime.** Within a Space each bot's session persists and resumes. When it gets long the backend compacts it, and the role's next turn gets the full first prompt again (decision 94). Across Spaces sessions are not shared: a bot's memory of other Spaces is the brief, the memory items and the on-demand tools, never the raw session. After 30 to 40 chats the architect still starts small and relevant.

**Brief and memory items.** The agent writes the brief from the conversation and the shared understanding with you; you approve it before it becomes the new brief. A memory item is never edited or deleted, but can be retired or replaced; each has an author and a role, and one an agent saves links to the Space or wrap-up it came from. An item you add in the window or with `reef decide` has no link.

**Capturing what we learn.** Both ways: the agent saves decisions, findings, facts, conventions and lessons as it goes with `remember`, and at wrap-up it proposes as items any it did not save; those are approved with the brief (decision 93).

**Wrapping up.** A Space ends explicitly: you click "wrap up" or type `/wrap`, or the lead submits its Work item for Review. The agent then proposes the brief, memory items and the ids of stale items to retire. No idle timers.

**Reef CLI.** The same brief and memory items are reachable from a terminal (`reef brief`, `reef decide`, `reef memory`, `reef work-item`), so plain Claude Code works when the app is not open.

**Role views.** Same data filtered per role: the tester's brief emphasizes what was built and what to test; the architect's emphasizes decisions and constraints.

## Work outside the board

Not everything is a Work item; these places cover the rest.

- **Free Spaces:** a Space with no Work item, associated with zero or more projects and running in a neutral directory of its own, never the project checkout (decisions 145, 150). Attach it to a Work item when it turns into real work; no brief is written for it.
- **Direct conversations:** talk to one Bot about anything. Many per Bot, any can be continued, nothing splits by itself. They have no project work attached, but under a Project they can propose a Work item.
- **Project memory:** memory items in the project's scope (conventions, facts, decisions) shared by every Work item and free Space in a repo, like a smarter CLAUDE.md. The 10 newest load into every session.
- **Bots and Teams pages:** tune a Bot (description, skills, tools, MCPs) and a Team (roster, responsibility text, queue) separately from project work.
- **Board:** the top-level Work items of a project by status, filtered by type, Team and project. You drag a card between Backlog and Queued and reorder a Team's queue; other moves are the item's own actions.
- **Capture:** a global shortcut drops a Work item into a project's Backlog with a type and a description, or starts a conversation with a Bot.

## Supporting capabilities

| Capability | What it does | Group |
| --- | --- | --- |
| Inbox | What waits on you, in three groups: Needs you (a paused Space, a failed turn, a held wake), Review (a Work item with proof) and FYI (a sub-item created, an item proposed, a queued item started). Desktop notifications | Seamless |
| Worktree per Work item | One worktree per top-level Work item on branch `work-item/<id>`, shared by all its Spaces and its sub-items. Free Spaces use the main checkout | Robust |
| Checkpoints and undo | Snapshot before each bot's changes, one-click rollback | Robust |
| Diff review panel | Inline comments on changes go back to the developer's thread | Robust |
| Permissions per role | Only the developer edits code by default; it may spawn parallel subagents. Other roles are read-only unless you grant write. Only you merge to main | Robust |
| Quick capture | Global shortcut to drop an idea, todo or bug into a project | Seamless |
| Voice input | Talk to the lead by voice | Seamless |
| Daily digest | Morning summary of what teams did, what is blocked, what waits on you | Seamless |
| GitHub integration | PR status and CI results flow into Spaces; failed CI pings the developer | Plug-in |
| Shared skills and MCP library | Define once, attach to any role | Plug-in |
| Inspector | Each bot's prompts, tool calls, tokens and cost | Learning |
| Built-in terminal and browser | One window for terminal, browser, commits, PRs | Later |

## Tech stack

A Go server is the brain, a React UI is the face, and Electron wraps both into the Mac app (no separate browser needed), following T3 Code's structure.

| Layer | Choice | Role |
| --- | --- | --- |
| Backend | Go local server | Router, bots, sessions, memory, Reef MCP server |
| Storage | SQLite | Projects, Work items, Bots, Teams, Spaces, memory items |
| Agent adapters | Claude Code and Codex CLIs in headless JSON-streaming mode, both in the baseline | One common Agent interface (Start, Send, Events, Interrupt, Resume); Jellyfish later |
| UI | React, rendered inside the Mac app window | Talks to the Go server over HTTP and Server-Sent Events on localhost |
| Desktop shell | Electron (the Mac app) | Main process launches the Go server; renderer shows the UI; hosts the built-in browser pane |
| Browser | Electron WebContentsView, driven via Chrome DevTools Protocol | Built-in browser pane; agents control it through Reef's MCP server (Go proxies calls to Electron) |
| Terminal | xterm.js | Built-in terminal pane |

**Reference.** T3 Code's apps/server, apps/web and apps/desktop folders. Its desktop app is Electron, with a preview module for the built-in browser.

## Decisions

Numbers are never reused: code and docs cite them. A row marked Replaced or Amended names the row that did it. Decisions 96 to 142 are the restructure decisions R1 to R54 of [docs/restructure/decisions.md](restructure/decisions.md), which has the R to number table.

| # | Decision | Why |
| --- | --- | --- |
| 1 | Teams and Spaces are core, not optional | Visibility into what each role does, and one window for the whole workflow |
| 2 | Journey stage is a label, never a gate. Replaced by 105: journey stages are dropped | Alignment on where the work is; it never blocks work |
| 3 | Versions of a Work item are a later idea, not baseline | Baseline the product first, ship one Work item through it |
| 4 | Reef data lives in a central app folder, not in repos | Replaces run files and handoffs; smooth transition |
| 5 | Build from scratch in Go, copying T3 Code patterns | Learning goal; use Reef to build Reef once there is a baseline |
| 6 | Brief and role notes are agent-written, user-approved. Amended by 91. | Agent captures the shared understanding; approval keeps it trustworthy |
| 7 | Reef has a CLI alongside the app | Plain Claude Code keeps working when the app is closed |
| 8 | Sessions persist per Space; across Spaces memory is notes + tools, not sessions. Amended by 91. | Continuity inside a Space, small context everywhere else |
| 9 | Only the developer edits code by default; it may run parallel subagents | Avoids two roles writing one worktree; matches poteto-mode auto |
| 10 | Caps: 4 hops, 25 turns, 1M tokens per Space; pause and ask. Amended by 31, 38, 59 and 69 | Stops loops without an LLM judge |
| 11 | Failed turns are marked, never auto-retried | User decides retry vs rollback |
| 12 | Per-teammate MCP tokens on one server | Teams are configurable; ports per role do not scale |
| 13 | Lead does not summarize specialists back | User already sees the post; halves lead cost per handoff |
| 14 | Replaced by 107. Gates default on for architect→developer and developer→tester, off for lead→architect | Gate the steps that cost tokens and touch code |
| 15 | Decisions captured both live (tool) and at wrap-up (proposed, approved). Generalised by 117 | Accuracy plus a backstop |
| 16 | Spaces end explicitly via wrap-up, no idle timers | Rewrite at the right moment, no wasted tokens |
| 17 | One worktree per top-level Work item, shared by its Spaces; free Spaces on the main checkout. Amended by 131 | Follows from single-writer rule |
| 18 | Replaced by 139. Promotion re-parented the Space and the lead wrote the initial brief | Brief exists from day one of the Work item |
| 19 | Claude Code and Codex both in the baseline | Prove the Agent interface with two real backends |
| 20 | One stdio MCP process per teammate on one shared database, each with its own token (amends 12) | Works with the server down, adapters already launch stdio MCP servers, no ports |
| 21 | Live agent output streams to the UI over Server-Sent Events, one GET stream per Space; users send messages with POST | Standard library only, data flows one way, and EventSource reconnects |
| 22 | Routing runs when a turn ends, over the messages that turn stored. Wakes run one at a time per Space | No agent is interrupted, and the message log fixes the order |
| 23 | A message from an agent wakes only the roles it @mentions, also a reply in a thread; the thread owner rule is for the user | Ends a chain, and the lead does not answer every specialist post |
| 24 | Every agent starts when first woken. A specialist's agent stops after each wake and resumes its session next time; the lead stays live | Idle teammates cost nothing, and the lead is who the user talks to |
| 25 | Replaced by 107. A wake across a gate that is on is held as a pending approval: approve runs it, reject drops it, and a restart keeps it | The user decides before tokens are spent, and a crash must not lose the question |
| 26 | The hop count resets when the user sends a message or continues. A wake over the cap is held, and continue runs it. Replaced by 107. The gate-approval clause no longer applies | A user message is new intent. Held, not dropped, so no work is lost |
| 27 | Threads have one level: a reply points at a root message, never at another reply. A teammate woken from a thread replies in it; one woken from a top-level message replies top-level and owns that thread | "Who owns the thread" stays one lookup, and the window stays simple |
| 28 | Wrap-up turns wake nobody | They are the lead talking to the user about the Work item, not a handoff |
| 29 | A team is a JSON file applied with `reef team apply`; a team `solo` (lead only) always exists. Amended by 139: a Space keeps its own member list | Smallest way to configurable teams now; the Teams page edits them |
| 30 | A role's `tools` become Claude's allowed tools; Codex ignores them and is limited by its permission mode | Only Claude has a per-tool allow list |
| 31 | Continue on a Space over its token budget sets the budget to the tokens used plus 1M (amends 10). Amended by 69: the budget is the top-level Work item's | A flat +1M left a Space that used far more than its budget paused again at once |
| 32 | Roles write only when the team file sets `write`; it maps to each backend's own limit (amends 9 and 30) | Checkpoints and the single-writer rule need one flag that does not depend on the backend |
| 33 | A Work item's worktree lives in the data folder and is made at its first wake, from the project's HEAD. Amended by 60, 70 and 131 | Follows decision 4, and a Work item nobody woke costs nothing |
| 34 | Write turns on one tree run one at a time across Spaces | Decision 17's single writer holds with two Spaces on one Work item |
| 35 | Rollback resets the files and the branch to the checkpoint, not the agent's session, and leaves a note in the Space | Sessions cannot be rewound; the note tells the agent |
| 36 | The inbox is a view over pauses, failed turns and who spoke last; no ask-user tool. Replaced by 107. The gate-approval part no longer applies. Amended by 138 | No new state to keep in sync, and Reef cannot tell a question from an answer |
| 37 | Replaced by 107. Default gates also hold a lead's wake of the developer, and a tester's wake of the developer (amends 14) | Decision 14 gates the steps that touch code; an ungated lead→developer or tester→developer edge let either skip the architect→developer gate |
| 38 | Default hop cap is 6, and it counts only talk-only turns: a turn with no tool call, or only Reef's own MCP tools (not its browser tools) and ToolSearch. Any other tool call resets the count (amends 10) | The spec's full flow lead→architect→developer→tester→lead uses 4 hops; one clarifying question must fit without a pause. A developer and a tester that edit and run tests are working, not looping |
| 39 | Retry runs the failed turn's wake as if the user sent it, so it resets the hop count (amends 26) | A retry is the user asking for the same work again, not a continuation of the chain that failed |
| 40 | Verification runs on a scripted `claude` that replays recorded real output and does its side effects; real `claude` runs only for Claude's own behaviour, at slice verification and when `agent/claude` changes | Real runs took 40+ minutes, cost tokens and failed at random; Reef's own code is what the lines check |
| 41 | The app always starts and stops its own `reef serve` | One process owns the data folder; the user never runs `reef serve` by hand on a Mac |
| 42 | `reef mcp`'s browser tools call the Reef app directly over a socket in the data folder, not through `reef serve` | The app owns the browser pane; `reef mcp` works even with the server down, like every other tool |
| 43 | One browser pane for the whole window, shared by every Space | Matches the one-window product: agents and the user look at the same page |
| 44 | A "Take over" button on the browser pane stops every browser tool with `the user has the browser` until "Hand back"; the user's own address bar still works | The user needs the page to itself sometimes, without an agent acting on it underneath |
| 45 | Project memory is one body per project, written by the user, loaded whole into every fresh session. Amended by 91. | Conventions and architecture notes apply to every Work item and free Space, not one brief |
| 46 | Diff review comments go as one user message to the developer's latest thread, or a top-level message starting `@developer` when it has none | Routing wakes the developer by the thread-owner rule without a new routing path |
| 47 | The `/teams` page edits a team's roles and saves with `PUT /teams/{name}`, which applies like `reef team apply`. Replaced by 107. The gates-editor part no longer applies. A role's body that omits `mcpServers` keeps the ones already stored. Duplicate and "New team" send `?from=<source>`: `name` must not exist yet, and an omitted `mcpServers` takes `source`'s role instead | The window never sees a role's `mcpServers`, since they may carry keys; a copy must not silently overwrite a team or lose the source's servers |
| 48 | Failed CI wakes the developer in the Work item's active Space, as author `reef` so the wake counts as a hop like any agent turn | The developer is who acts on a broken build; the Space already carries the PR and check notes, and a wake with no user behind it must still respect the hop cap |
| 49 | A role in the team file may set `face` (one of eleven creatures); empty means the default for its name. Amended by 126: the face belongs to the Teammate | A team of two developers or a custom role should not all look alike, and the file stays the one place a team is set up |
| 50 | The window has a light theme and a Match macOS option, kept in `localStorage` | It is a per-device look, not project data, so the server stores nothing |
| 51 | Below 768 px the same app becomes the phone layout (bottom tab bar, full-screen Space), not a second app | One codebase and one set of routes; a narrow window and a phone need the same screens |
| 52 | Today replaces Home as the landing page | One place for what waits on you and what moved, instead of a list of projects |
| 53 | The desktop window opens on a local splash page and loads the app once `reef serve` listens | The window shows something at once instead of a blank page while the server starts |
| 54 | Project memory (decision 45) is edited on the project page, not on Today. Amended by 91. | It belongs to the project, and Today is about what moved |
| 55 | A role in `GET /teams` lists its MCP servers as `mcp`, each only a name (`mcp1`, ...) and a label (the command, or the url's host); a `PUT` body lists `mcpRemove`, the name and label of each server to drop, and every other stored server is kept; a removal whose name and label no longer match is a 409. The window removes servers, `reef team apply` adds them | The window can show and drop a server without ever holding its `args`, `env` or `url`, which may carry keys |
| 56 | The person using Reef is one `profile` row: a name, an optional occupation and optional working preferences, set on first run and in Settings. Every agent's context starts with `You work with <name>, a <occupation>. They like: <preferences>.`, each part only when set | Agents talk to a person, not "the user"; the fields are chips and one line, not free text, so the line cannot carry instructions |
| 57 | The browser pane starts closed and the window remembers the choice; it also shows while an agent drives it, without changing the remembered choice | A pane that opens at every launch is noise, but an agent's browser work must be seen |
| 58 | One Terminal toggle for the whole window, default off, remembered. It never creates a worktree: a Work item without one shows an Open terminal button, and only that button makes it | Looking at a Work item must not change the user's git repo |
| 59 | A role with `write` has no tool call cap; the other roles keep 25 per turn. Replaced by 69 | The writer does the long work (tests first, installs, checks), and the cap paused it mid-change; read-only roles stay capped so a looping reader stops |
| 60 | A Work item's worktree branches from the repo's default branch (the local branch `origin/HEAD` names, else `main`, else `master`, else HEAD), not from the checkout's HEAD; the Work item diff is against the same ref. Replaces the HEAD part of 33 | A Work item must not inherit the commits of whatever unrelated branch the checkout is on |
| 61 | Approving a wrap-up closes the Space: the server refuses its turns with 409 until the user reopens it, and the sidebar folds it under Wrapped | Decision 16 ends a Space with wrap-up, but nothing stopped a message after it; "record and close" recorded the decisions twice |
| 62 | A Work item can be closed: closing it closes all its Spaces (409 while a turn runs); reopening restores the item only, each Space is reopened by hand | A finished Work item should leave the working list without being deleted, and its Spaces must stop taking turns with it |
| 63 | The user can dismiss a `waiting` item in the inbox; a pause and a failed turn have no dismiss. Replaced by 107. The gate-approval part no longer applies. Amended by 138: `waiting` is an FYI item | Waiting asks for no action and can pile up; the others block work until the user acts. Dismiss hides only the message it was pressed on, so a new agent message brings the item back |
| 64 | Replaced by 98 and 105. A feature had a kind, `journey` (default) or `fixes`, and a `fixes` feature showed `N of M done` instead of the stage bar | Some features were a mix of small fixes, not one piece of work through the eight stages. A Work item's sub-items now carry that |
| 65 | A role may name the `model` its agent runs on, passed to the backend as `--model` (claude) or the thread `model` (codex); empty is the backend's default | The lead can think on Opus while the developer writes on Sonnet, without changing the user's global default |
| 66 | A Work item's PR shows in a Space side panel, opened from the header's PR chip, with a Merge button that runs `gh pr merge --merge` (a merge commit) only when gh calls the PR mergeable; Azure DevOps is a later ticket | The user sees checks and reviews where the work happens and merges without leaving Reef; gh decides whether a merge is allowed and its message is shown when it refuses |
| 67 | Every merge to `main` that passes tests is a numbered release `v0.1.N`; the everyday app is a release build, not the checkout | Reef builds Reef, so the app in use must not change while agents edit the repo |
| 68 | Geist is the font everywhere in the web UI, self-hosted in the build | The user finds it easier to read than the system font; bundling it keeps it working offline and makes the window, the desktop app and a browser match |
| 69 | No role has a tool call cap; a turn runs until the agent ends it, Stop, or the token budget, which belongs to the top-level Work item (default 1M; every Space of the item and its sub-items counts against it, a Space with no item has its own) and is checked only between turns, never stopping a turn mid-work. Replaces 59; amends 10 and 31 | The cap paused read-only roles mid-work as often as it stopped loops; Stop and the token budget already bound a turn. One item is one piece of work, however many Spaces it takes |
| 70 | A project may name a base branch, set on its page. A new Work item worktree fetches it from `origin` and branches from `origin/<base>` (else the local branch); the Work item diff and the agents' context use the same branch. Empty keeps decision 60. Existing branches are not moved. Extends 60 | sm2 work targets `dev`, not the default `main`, and a stale local branch gave stale Work items |
| 71 | A message that arrives while a message chain runs joins that chain's queue and runs after the current turn; turns still run one at a time. A wrap turn running still refuses it. Replaced by 107. The gate-approval part no longer applies. Extends 26 | The user and the lead can talk while a teammate works |
| 72 | Replaced by 107. An approval that arrives while any chain starts or runs, a wrap one included, joins that chain's queue and runs after its turn; if the chain ends or never starts, the approval is pending again. Messages keep decision 71. Extends 71 | Approving a gated wake while the Space was starting or wrapping up was 409 for no reason the user could act on |
| 73 | Teammates may coordinate one-to-one with `send_to_role`; a request gets one automatic direct response, and continuing requires another explicit send. This replaces the "through the Space only" rule | Detailed handoffs stay out of the shared narrative, while the request/response boundary prevents automatic ping-pong loops |
| 74 | An agent proposes a brief mid-space with `propose_brief` when the user and the team agree on the goal, scope or plan. Approving it saves a brief version only: memory items and the open Space stay. Extends 6 | The brief is about the Work item, so it should change when the team agrees, not only when a Space ends |
| 75 | Replaced by 105. An agent moved the feature's stage with the `set_stage` tool, with no approval, and Reef posted a `reef` note in the Space | Agents saw when the work moved; the user did not have to move the label by hand |
| 76 | A decision is never edited or deleted, but it can be retired: `retiredAt`, and `replacedBy` when a newer decision replaces it. `record_decision` and a wrap decision take `replaces`; a wrap proposal takes `retire` for stale or duplicate ones; approving applies both. The context index and the Work item page list live decisions only. Amends the append-only line in Context and memory. Amended by 91. | Stale and duplicate decisions bloated every prompt and the page, with no way to remove them |
| 77 | A wrap-up turn reads every direct exchange of its Space, and only that turn may read a pair's messages besides the pair and the user | The brief and decisions must reflect private work; wrap-up is user-approved, and roles still learn it only through the approved brief, decisions and public posts |
| 78 | One routing pass wakes a role once, whatever threads the messages that name it are in; the texts join and the reply goes where the first one did. Replaced by 107. The held-approval wording no longer applies. Amends 27 | A thread reply and a top-level post that both handed work to the same role woke it twice |
| 79 | A Space created with the new-space form's default name (backend or team, then the time) gets a name from its provider after its first user message: one throwaway turn with no tools, stored as no turn or message, once. A typed name, any rename, or a failed call keeps the name the Space has | The user never has to name a Space, and what they name or rename always wins |
| 80 | Only a handoff @mention wakes a role: an `@name` at the start of a message or of a line, after optional spaces or tabs, and every `@name` of an unbroken run there (`@architect, @tester do X`). An `@name` anywhere else in a sentence wakes nobody, from an agent or the user. Refines 23 | "I'll report to @lead later" woke the lead for a turn that had nothing to do |
| 81 | Replaced by 107. An approved wake resets the hop count, as a user message or continue does. Amends 26 | The user looked at that wake and said yes; pausing right after asked the same question twice |
| 82 | The answer of a background task a role started is posted to the Space as that role's message | The user asked for that work, and it otherwise lands in no turn |
| 83 | On Claude, a role without `write` whose `tools` allow a shell command outside the reading-command allow-list in `space/session.go` (`Bash(git diff:*)`, `Bash(go test:*)`, `Bash(make:*)`) runs as a writer, with the single-writer lock and a checkpoint. Amends 32 | Such commands can change files, so they need the single-writer lock and an undo point |
| 84 | A specialist's process closes after its turn, so a background task it started ends with it and its answer is not posted. Only a role whose process stays alive, the lead, posts a background answer, also while no turn runs. Amends 82 | Keeping a specialist alive for a task it may never finish costs a process per role, and a closed one has no answer to read |
| 85 | One assembler builds every first prompt, a Work item Space's and a free Space's, in three sections in this order: `## You` (persona, skills, role, Work item, project, the user, description, base branch), `## How to work` (team rules, then one line on the tools), `## What we know` (brief, sub-items in progress, memory items, one count line), each part under a `###` label: Brief, In progress, Decisions, Findings, Space, Project, Team, Your items, On demand. The team rules keep their wording; only their `## Team` header goes, under `## How to work`. Amended by 91. | Instructions before knowledge, one header per kind of content, one place that builds it. The Space evals tested the rules' exact text; a rewrite needs its own eval run |
| 86 | An empty part of the first prompt is left out: no "No brief yet.", "No notes yet." or "none"; a section with no part left is left out too | Those lines tell the agent nothing |
| 87 | The first prompt lists only the sub-items in progress. Open sub-items and earlier Spaces are one count line with the tool to use: `23 open sub-items (list_sub_items). 3 earlier Spaces (search_sessions).` | A sub-item has no owner and the wake names the work; the agent only needs to know that more exists |
| 88 | A resumed role's wake holds what changed since its session's previous turn, right after its `[Space N, message N]` header line (at the top when it has none): every decision added since, whoever its author, and every one retired since, as index lines, under `### Changed since your last turn`, and the brief in full under `### New brief` when a newer version exists. A decision added and retired since is named as retired only, as the role may have seen it in its previous turn. Sub-items are not in it. It comes before any `reef wrote:` lines. The first prompt, which reads the team and the role again, and this part are built after the turn's start is stamped, so an item written while the turn waits is in the text or the next one, never in neither. Amended by 92. | A role otherwise never learns of a new decision or brief during its session; a decision's author is only a role name, shared by the lead of every Space, so an own decision cannot be told apart |
| 89 | A Go test builds the first prompt for a Work item with 60 decisions, 40 sub-items and 15 Spaces: no section twice, each limit holds, under 8,000 characters without the persona. Live decisions are not capped, so 60 full-length decision lines exceed it. Another checks the delta holds only the new items. `turn.prompt` stays the only record of what was sent | Size is checked by a test, not by eye; no new storage |
| 90 | `remember` takes an optional scope: `work_item` (default on a Work item's Space), `project` or `teammate`; team items come only from the lead's wrap-up. Amends 118. Amended by 140 | A finding about the whole project or about how one teammate works should not be filed under one Work item; a team item changes how everyone works, so only the wrap-up proposes it and the user approves |
| 91 | Memory items replace project memory, role notes and the decision table as what agents read and write: `remember`, `recall` and `read_memory` instead of `record_decision`, `read_decision` and notes, and retire takes memory ids. `## What we know` holds Brief, Decisions (every live one), Findings (5), Project (10), Team (5) and Your items (5), each the newest accepted first, and each counted part ends `N of M; recall(..., scope=...)`. The old tables are not migrated (109). Amends 6, 8, 45, 54, 76 and 85 | One store with scopes holds what every Work item, project, team and role learns, and the count tells the agent that more is there to recall |
| 92 | The wake's `### Changed since your last turn` lists the memory items accepted or retired in the role's scopes since its session's previous turn. Amends 88 | Decisions are now memory items, and a finding or a project item changes what the role knows as much as a decision |
| 93 | A wrap-up proposes memory items, each with a scope, and the ids of items to retire. Approve saves the items, retires the ids, then saves the brief and closes the Space; it is retry-safe, not one transaction. The lead's `team` items are saved as candidates | Saving an item may call the embedding API, which must not hold the database write lock; an approve that fails part-way is approved again without duplicates, and the user approves what changes how the whole team works |
| 94 | After the backend compacts a role's session, the role's next turn gets the full first prompt again, not the delta | The compacted session no longer holds the first prompt it was sent |
| 95 | A solo Space is not a team: it has no `team` scope, so no Team part and no team items | Team lessons are about working together; a `team:solo` shelf would be one global shelf across every project |
| 96 | Reef stays single-user. The one user creates every Teammate, Team, Space and Work item (R1) | Nobody shares a Reef, so scopes are retrieval filters for relevance, not access control |
| 97 | Memory service first; the prompt-assembly audit runs alongside it (R2) | Memory is the hardest open problem and the assembler is where memory plugs in |
| 98 | Feature stops being an object. It is one Work item type, beside bug, incident, investigation, task and improvement. Replaces 2, 64 and 75 (R3) | Work is broader than features; one container, typed |
| 99 | A Team is a roster of Teammates plus routing rules, a responsibility text, a queue and coordination memory. It has no chat of its own (R4) | "Who works together and how" stays separate from "where talking happens" |
| 100 | A Space is where talk happens. A Work item has one active Space. Free Spaces with no Work item are allowed. Space continuation, for a Space that grows too long, is a later slice (R5) | One place holds the whole story of a piece of work |
| 101 | Phases (design, implementation, testing) are threads inside the Space, or sub-items. Never a Space per phase (R6) | Splitting phases splits context and costs fresh sessions |
| 102 | A Teammate proposes a Work item with `propose_work_item` and suggests a Team from the Teams' responsibility texts. The user creates and assigns. The Team's lead picks it up and opens the Space (R7, R43) | Teammates never assign; the board stays the user's |
| 103 | Serial: one Work item per Team at a time. The top of the queue is next (R8) | Matches one-ticket-at-a-time and the single-writer rule |
| 104 | A direct conversation is with one Teammate. Typing `@Y` or "add Teammate" turns it into a Space with the user, X and Y; the conversation is marked moved and linked; Y gets a summary, never the transcript. Ask a Teammate stays in the chat, and is a later slice (R9) | Adding someone is one motion; the two gestures stay distinct |
| 105 | Work item statuses: Backlog, Queued, In progress, Needs you, Review, Done, Dropped. Only the user sets a top-level Done. Journey stages are dropped. Replaces 2 and 75 (R10) | The Team gets to Review; acceptance is the user's |
| 106 | Proof is part of the Work item: PR, test command and output, screenshots, ticked acceptance lines. The lead fills it at Review. A parent's proof is one end-to-end check by the lead (R11) | Verification becomes first-class instead of a comment |
| 107 | Approval gates are removed. Caps, checkpoints with rollback and Needs you remain. Replaces 14, 25 and 37, and the gate parts of 26, 36, 47, 63, 71, 72, 78 and 81 (R12) | Review with proof checks the result; gates checked intent and stalled the queue |
| 108 | A Team creates sub-items under its Work item without approval; they stay with the same Team and surface as FYI. Inbox kinds: Needs you (blocks), Review (blocks that item), FYI (does not block) (R13) | Work moves while the user is away; the user can still drop or reassign |
| 109 | Clean break. Features, channels, tasks and roles are not migrated; one forward migration drops the old tables (R14) | Nobody uses Reef yet |
| 110 | One `memory` table of items (kind, body, why, scope, author, role, status, retired_at, replaced_by) plus one `memory_link` table (from, rel, to_kind, to_id). A brief stays a versioned prose blob per top-level Work item; role notes and the project-memory blob become items (R15) | Small items with links can be retired, searched and traced one by one; blobs cannot |
| 111 | Kinds: decision, convention, finding, fact, lesson. Scope is one of project, work_item, team, teammate, space, conversation, as two columns on the item. Amended by 140 (R16) | One shelf per item keeps the wake filter a plain WHERE |
| 112 | Link rels: about, source, replaces, related. Targets are plain strings, no foreign keys; `memory/` knows no other table. Links come from the writer's token context (free), the writer's arguments (`about`, `replaces`), and a lookalike check that proposes and never links silently. No model-based entity extraction (R17) | Most links are known at write time; a text check covers the rest |
| 113 | Items are append-only. `replaces` retires the old item with `replaced_by`; `retire` marks it stale with nothing replacing. Retired items never enter the wake prompt; recall may return them marked retired (R18) | Records that point at an item must stay true; history of what was believed stays readable |
| 114 | Search is hybrid: FTS5 with porter stemming (BM25) plus embeddings stored as a blob, fused by reciprocal rank fusion; then expand links one step, never through hubs (project, team). No reranker until evals show the need (R19) | Word-exact and meaning-based search cover each other's gaps; one step brings the neighbourhood without the whole project |
| 115 | Embeddings are optional: a setting names the provider (none, OpenAI, Voyage, Gemini) and key. Without a key recall is FTS only. Embedding is synchronous on write and a failure is reported; a background pass to fill missing vectors is not built (125) (R20) | Reef stays free and local by default; one field turns on better search |
| 116 | Candidate vs accepted: conventions and team lessons need user approval; decisions, findings, facts and lessons are accepted on write by any role, and the user can retire any. Authority is a short table in code, not config or prompting. Amended by 124 (R21) | Approval stays where a wrong item changes how the team works |
| 117 | Extraction is live via `remember` while working plus a wrap-up pass that proposes what was missed. Direct-conversation items get the scope the writer picks; conversation scope is searchable but never loaded at wake (R22) | Same as decisions today, generalised |
| 118 | Agent surface: `remember(kind, body, about?, replaces?, why?)`, `recall(query, limit?, scope?)`, `read_memory(id)`. Amended by 90 (R23) | Three tools, no table knowledge in the agent |
| 119 | Wake prompt: fixed counts per section (project 10, work-item decisions all live, findings 5, team 5, teammate 5, plus the brief), each section ending with `N of M; recall(..., scope=...)`. Sent once per session; later wakes get a delta of accepted items in scope since the role's last turn; compaction refreshes the snapshot (R24) | The prompt is read on every turn for the life of the session, so it stays small and the agent knows how to get more |
| 120 | Team memory: the lead proposes team lessons at wrap-up as candidates; members may write team-scope items mid-Space as candidates (R25). Amended by 90: team items come only from the lead's wrap-up | Few, curated lessons about working together |
| 121 | Evals: a golden set of about 30 scenarios in `go test` asserting recall@10, MRR and that forbidden ids are absent; knowledge-update cases (replace, retire, conflict); token cost per wake recorded; hand-graded set with real embeddings on demand; an LLM judge only if ranking proves weak (R26) | Deterministic checks cover write policy and scope isolation, which published benchmarks skip |
| 122 | Memory is the package `memory/`, with its own tables in `reef.db` and its eval harness under `memory/eval/`; `memory/` knows no other table (R27) | Isolation by package boundary; a service boundary buys nothing for one user on one Mac |
| 123 | Gemini (`gemini-embedding-001`, 768 dimensions, no task type) is a third embedding provider beside OpenAI and Voyage. Each provider has its own cosine floor: Gemini 0.6, OpenAI and Voyage 0.3 (not yet measured) (R28) | Google gives a free tier, so meaning search costs nothing to try. Measured on the eval: related 0.69 to 0.76, unrelated at most 0.57; a task type raised every score without widening the gap. The 0.92 lookalike threshold holds for Gemini: near-copies 0.98, reworded notes 0.84 to 0.87, unrelated 0.52 |
| 124 | A convention by an architect, lead or the user is accepted on write; other roles' conventions are candidates. Any writer may ask for approval (`needs_approval`), which makes the item a candidate whatever the role. Amends 116 (R29) | Architect and lead own how the team works; they can still put a rule they are unsure of in front of the user |
| 125 | There is no background pass that fills missing vectors: a failed embed is reported on write and `reef memory embed <provider>` retries. `Backfill` waits out one 429 per item (its `Retry-After`, 1 to 60 seconds); writes and recall never wait (R30) | Nothing long-lived runs inside the server for this; a silent failure would leave items out of meaning search unseen, so the failure is reported |
| 126 | `teammate` table: persona, skills, backend, model, face, plus default access rules (tools, MCP servers, write, permission mode) used in direct conversations. Adding a Teammate to a Team copies the defaults into a `team_member` row (with the lead flag); later edits to the Teammate do not change memberships. The memory `teammate` scope id is the teammate id. A team member has the name its Team calls it (unique per team): the `@mention` name and the author of what it posts. `reef team apply` makes a Teammate `<team>-<role>` when none sits behind that role (R37) | One agent can sit in several Teams without carrying unrelated context; a rename keeps its items |
| 127 | The `task` table goes. Tasks are Work items of type `task`, usually sub-items. `create_task` becomes `create_sub_item` (R38) | One container for all work; no second list |
| 128 | Full rename: feature → Work item, channel → Space in tables, Go, REST, MCP, UI and docs. One forward migration drops the old tables. "Channel" is kept for the later incoming-event streams (R39) | One word for each thing, and the old word is free for a different one |
| 129 | In the domain-model slice: model, board, Work item page, Spaces, Teammates and Teams, queue, proof and Review, inbox kinds, gate removal, direct conversations and 104. Not in it: Ask a Teammate, Channels and Automation, Space continuation, sub-item cap (R40) | A slice small enough to verify; the rest is open in docs/restructure/index.md |
| 130 | Review frees the Team; the next Queued item starts by itself. Sent back → top of the queue. Needs you does not free the Team: the item still holds its worktree (R41) | The Team's capacity is its one item in work; a held worktree cannot be shared |
| 131 | One worktree, branch and PR per top-level Work item; sub-items share the parent's Space and worktree. The worktree is `<data folder>/worktrees/<project id>/<work item id>` on branch `work-item/<id>`. Amends 17 and 33 (R42) | One piece of work, one diff to review |
| 132 | `propose_work_item` creates a Backlog item "proposed by X" with a suggested Team, plus an FYI note in the proposer's Space. With no matching Team the suggestion is `solo` (R43) | A Teammate can raise work without taking the board from the user |
| 133 | The lead runs the wrap-up when it submits for Review; setting Done approves the wrap-up (R44) | Memory is saved at the moment the work is judged |
| 134 | Many direct conversations per Teammate; any can be continued; nothing splits by itself (R45) | A Teammate is a person to talk to, not one thread |
| 135 | Sub-items run one turn at a time in the Space; subagents inside a turn are fine; true parallel is a later slice (R46) | The single-writer rule and one Space per item |
| 136 | The lead sets Done on sub-items and is the only mover of a sub-item (`set_sub_item_status`); any Teammate may create one; only a top-level Done is the user's. A parent goes to Review only when every sub-item is Done or Dropped. Dropped is final. Amends 105 (R47) | The user judges the whole item; the lead judges its parts |
| 137 | A Work item has a description and acceptance lines. Proof: PR (found from the branch), each line ticked with a note, test commands with output, screenshots. The lead fills it with `submit_for_review` (R48) | The user reviews evidence, not a story |
| 138 | Inbox: Needs you (a cap pause, a failed turn, a held wake), Review (proof and wrap-up), FYI (sub-item created, item proposed, queued item started, and a `waiting` message) (R49) | Blocking items and items to know about stay apart |
| 139 | A Space has its own member list (`space_member`: Teammate, lead flag, access rules copied from the Team membership or the Teammate's defaults) and optionally a Team. A free Space comes from 104 (no Team) or from "new Space" with a Team; it can be attached to a Work item (replaces promote, 18), which moves its `space` items to the `work_item` scope (R50) unless the Space is associated with Projects other than the Work item's, in which case they stay in the Space | A running Space must not change when a Team is edited, and a free Space needs members too |
| 140 | Memory scopes add `space` and `conversation`; `conversation` is never loaded at wake (R51) | A free Space and a direct conversation need a shelf of their own |
| 141 | Type is a label (icon, filter) with no behaviour. Only top-level Work items have a brief (R52) | Behaviour from a label would make types a second state machine |
| 142 | Routing unchanged: user messages go to the lead; an @name at the start of a line hands off. A Team's responsibility text only suggests a Team for a proposed item (R53) | Routing stays plain code that nobody has to guess |
| 143 | Reef Bot, or Bot in the app, replaces Teammate. A Bot has a stable identity and one authoritative profile: name, plain-language description, model, skills, tools, connections and permissions. Renaming preserves references and recorded historical attribution; capabilities are configured separately and descriptions grant no authority. Supersedes identity and copied-access parts of 126 and 139 | One Bot keeps its identity and configuration across conversations. |
| 144 | A Crew replaces a Team as a reusable roster, with name, optional description and suggested coordinator. Adding it deduplicates Bots; changing or deleting it leaves existing Space membership and Bots intact. It has no memory, queue, aliases or capability overrides. Supersedes 99, 103, 120, 126, 130, 139 and Team suggestion parts of 102, 132 and 142 | A roster is not an execution authority. |
| 145 | Personal chats and Spaces are global, persistent conversations associated with zero or more Projects. Creation under a Project associates it immediately; All projects starts unassociated. Associations are visible and editable, with attributed Bot changes; a passing mention is insufficient. Filters never change context, associations or sessions. Multiple chats per Bot remain reopenable; New chat creates a distinct topic. Under a Project, opening a Bot resumes its latest associated chat or offers New chat; under All projects it opens the latest global chat. Pins distinguish Bots, chats and Spaces, and search also finds Work items. Amends 100, 104 and 134 | One history can be found under each relevant Project. |
| 146 | A Space needs at least one Bot to receive messages. The first Bot is coordinator by default; the user can change it. Unmentioned messages go to the coordinator. A Crew never silently replaces it. The coordinator and Work-item owner are distinct; changing one does not change the other. Bot-proposed participant expansion and ownership transfers require user approval; user-requested additions need no second approval; newcomers receive a handover and relevant context; removal waits for the current turn and cancels pending requests. Amends 136 and 142 | Coordination stays visible and user controlled. |
| 147 | Bot edits apply to new sessions or explicit Refresh, not silently to an existing session. Renewal retains adopted settings. Refresh and model switching preserve history and workspace with a short handover and access to earlier records. Revoke now stops all sessions of that Bot; continuation uses replacement sessions with current settings. Session snapshots are history, not editable overrides | Settings changes must not misrepresent a running session's authority. |
| 148 | A Bot may recommend an owner and participants; discovery reports identity, expertise and configured capabilities without granting access or executing work. User approval authorizes the assignment and queues it; Reef schedules it without a second start approval. Backlog is not authorized. Review feedback returns work to Queued; only the user accepts Done. Ordinary handoffs within approved participants need no new approval; expansion or execution delegation outside that assignment does. Supersedes Team assignment and capacity rules in 102, 103, 130 and 132; amends 105 and 107 | Reef schedules authorized work without making Crews queues. |
| 149 | Each execution has a visible retained conversation. A Bot's Space activity is a view of its existing session. Space references read records by default; explicit questions or guidance route to the existing participant session, with linked origin and destination, queued/delivered/failed state and retry-safe delivery. Guidance waits for a turn boundary and takes priority over pending Bot handoffs; Stop interrupts. Consequential user guidance reaches the Space and affected participants. Consultations have their own visible linked history and do not expand assignment authority. Archived history is readable without restarting execution | Users can inspect and steer work without creating hidden execution. |
| 150 | A general personal chat starts in a neutral non-repository directory. Repository work uses an explicit target workspace; association is neither authorization nor a workspace. Quick fixes may proceed without a Work item in isolated worktrees based on the intended origin branch. If fetch fails, use a valid local base and tell the user; if none exists, pause. User direction chooses reuse or a new branch; ambiguous follow-up to an open PR/MR needs clarification. Supersedes a mandatory Work item before repository edits and the single-repository constraint in 131 | Discussion and execution have distinct boundaries. |
| 151 | One coherent Work item or quick fix may span Projects, with separate repository workspaces, branches, verification and results in one visible history. Independent Bot sessions have no arbitrary per-Bot limit; writes to one worktree serialize. GitHub and GitLab are built-in provider connections; local Git handles worktrees and commits. Opening a PR/MR does not authorize merging. Merged or explicitly abandoned workspaces are cleaned up after preserving recoverable uncommitted and unpushed work under the writer lock; preservation failure retains the workspace. Closed without merge remains available. Conversation history and delivery references remain | Cross-repository work remains attributable and recoverable. |
| 152 | A Project directory exposes stable identity, name, purpose, repository and base, plus known relationships. Relationships are sourced Project-memory facts, not a second authoritative store. Retrieve detail on demand. Deleting a Project stops affected execution first, preserves repositories, histories and workspace references, retires its memory, and pauses assignments requiring it. Conversations with only deleted Projects remain readable; mixed conversations remain usable under active Projects | Projects organize context without owning global history. |
| 153 | Memory is captured as useful information emerges in Bot, Work-item, Project or free-Space scope and retrieved on demand; existing conversation memory remains retrievable. A Bot maintains its own memory every seven days when next active, retaining retired history. Any Bot may save shared facts, findings and work decisions; new or replacement shared conventions and retirement or replacement of shared items require user approval. Closing a free Space preserves history and can capture missed memory without a brief, proof or extra closure approval. Supersedes Team-memory parts of 90, 91, 111, 116, 120, 124 and 139 | Shared knowledge stays scoped and user-controlled conventions remain reviewable. |
| 154 | Reef supplies a shared product prompt before the user's plain-language Bot description, followed by current context and delivery instructions. It teaches collaboration, evidence, memory and approved scope; backend instruction facilities are used where supported. Prompt text is not a permission boundary. Reef-managed skills retain supporting files and compatibility; connections are selected per Bot and do not create event listeners or outbound authority. Startup uses lightweight Project metadata and retrieves details on demand. Validate real handoffs, disagreement, blockers, evidence and asynchronous delivery | Users describe expertise without repeating Reef's working rules. |
| 155 | Reef has no users yet: develop the new model against a fresh database, without a legacy Team migration or compatibility program. This clean break does not waive preservation of histories created by the new runtime, session replacement, Project deletion or workspace cleanup | Avoid preserving an obsolete model while protecting future user history. |
| 156 | Failed CI wakes the active Space coordinator by Bot identity. Approved-owner routing belongs to the assignment execution work. Supersedes the named-developer recipient in 48 | A global Bot name is not a role alias. |
| 157 | Approved work whose repositories are ready starts at once; there is no concurrency limit, so Queued holds approved work only while admission waits (no ready repository, or owner missing); order, reorder and review-feedback-first apply among waiting items | Ordering without a limit would only delay ready work. |
| 158 | A turn that posted with `post_to_space` posts the text it wrote after its last successful post as one more message; the text before it stays on the turn. A turn whose post is its answer ends with the post and has nothing more posted. Amends the rule that a posted turn's plain reply is never posted | A Bot posts progress ("Reading the notes now."), then writes its answer as plain text; the old rule hid that answer from the Space |

## Contract checklist and implementation choices

The chronological [design discussion](https://gitlab.com/bhanuprakashsaip/reef/-/issues/144) is the source for decisions 143–155. Later comments supersede earlier proposals: a Reef queue replaces Team queues; explicit Refresh replaces next-turn settings changes; quick fixes do not require Work items; automatic cleanup replaces an offer to clean; local-base fallback replaces a confirmation gate; and a fresh database replaces legacy preservation. A closed unmerged PR/MR is not abandonment. Shared conventions and shared-item retirement or replacement require user approval, regardless of Bot identity.

Checklist against the discussion, in chronological order:

- Initial direction and consolidated proposal: identity, roster, execution, memory and clean break (143–155); the later choices below replace its open recommendations.
- Refresh, membership, writer serialization, cleanup, personal memory and persistent chats (145–147, 151, 153).
- Shared product prompt, ordinary descriptions and four layers (143, 154).
- Skills and connections are selected capabilities; external event automation is deferred (154).
- Bot activity, Space references, steering, consultations and archived history (149).
- Global conversations, visible associations and Project discovery (145, 150, 152).
- Repository targets, neutral directories, deletion, Revoke now, memory authority, base fallback, providers and startup context (147, 150–154).
- Free Space coordinator and closure, and shared-memory replacement approval (146, 153).

Settled behavior is in the numbered decisions. Schema, tool names, UI placement, history page size, session-renewal threshold, provider API versus CLI, handover mechanics and detailed recovery are implementation choices. External-event automation and its reply policy remain deferred. The existing conversation-memory shelf is not decided for removal. These decisions describe a target; they do not claim runtime verification.

## Open questions

- [x] (spike, week 1) Exact event formats from Claude Code and Codex headless modes, mapped to the Agent interface. Answered in [agents.md](agents.md).
- Consultation: nested consultations are deferred; any future limits need a separate decision.
- Space continuation: what triggers a new Space for the same Work item.
- Sub-item cap: whether Bots may create at most N sub-items under one parent without a Review.
- Channel and Automation lifecycle and approval.
