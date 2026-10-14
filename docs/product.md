# Reef: product

## Vision

Reef is a personal Mac app where every feature keeps its own memory, and a team of AI agents works on it together in one window.

**The problem.** Big features get split across sessions: plan in one, implement in another. Today the context is carried by hand, through a handoff file the agent writes so the next session can resume. Sessions pile up per project, and the history of why things were done is lost.

**The idea.** Organize work around features, not chats. Each feature holds its tickets, decisions, sessions, PRs and post-release gaps. A team of agents (lead, architect, developer, tester) works on it in channels, and every session starts already knowing the goal and what was decided.

**Why a team, not one agent.** Two reasons beyond memory. Visibility: a single Claude Code session is a black box; separate roles in a channel show who is doing what. One place: sessions, todos, features, PRs and browser live in one window, so there is no switching between tools.

**Relation to Jellyfish.** Jellyfish is the Go agent harness (a learning project on agents and MCP). Reef is the home where agents live and work together. Reef orchestrates Claude Code and Codex; Jellyfish can plug in as another backend.

**Inspiration.** T3 Code (pingdotgg/t3code): a server that runs coding agents, a web UI, and an Electron desktop shell.

## Core entities

Everything hangs off Projects and Features; Teams are defined once and plugged in anywhere.

| Entity | What it is | Belongs to |
| --- | --- | --- |
| Project | A repo on disk | Top level |
| Feature | Long-lived home for one piece of work, with versions | Project |
| Task | A todo or ticket, tagged with project and optional feature | Global list |
| Team | Reusable group of roles (lead, architect, developer, tester) | Global |
| Role | Persona, skills, tools, MCP servers, backing agent (Claude Code or Codex) | Team |
| Channel | What the user opens as a "session": a message log with a team in it | Feature or project |
| Teammate session | One role's real agent session inside a channel | Channel |
| Decision | A recorded choice: what, who, why, when | Feature |
| Artifact | Branch, PR, commit, test run, bug | Feature |

Hierarchy: Project → Features (versions, journey, channels, decisions, artifacts) and Quick sessions. Tasks show in one global list and board, filtered by tags.

**Themed vocabulary** (brand only; the UI keeps plain terms): a bot is a jelly, the app is Reef.

## Feature lifecycle

A feature is a product area that evolves through versions, not a one-time chat.

**A feature contains:** About (goal, description, becomes the agent brief) · Journey stage · Tickets · Channels and their history · Decision log · Branches and PRs · Gaps and deferred · Issues found after release.

**Journey stages** are a label only, never a gate. They keep the user and the agents aligned on where the feature is. Each stage suggests the right role:

1. Ideation: lead and user shape the idea
2. Grill: architect challenges the idea
3. Plan: architect writes the design
4. Tickets: plan broken into tasks
5. Implement: developer builds
6. Testing: tester verifies
7. Live testing: real use after deploy
8. Post-release: gaps, deferred items and issues collected

**Gaps and deferred.** Things found after release, and things knowingly pushed out of scope. Agents can add to this list ("deferred: rate limiting, out of scope for v1").

**Starting a feature.** Either create the feature and open its first channel with the lead (ideation), or start a quick session and promote it to a feature once it turns into real work. The feature then holds the goal, open questions and status.

**Versions (later idea).** When v1 goes live, its gaps and deferred items seed v2. v2 inherits the brief and decisions and starts a fresh journey at Ideation. v1 history is archived but stays searchable. Not part of the baseline.

## Teams and channels

A channel is what the user opens as a session; inside, each teammate is its own agent session, and the Reef server routes messages between them.

**Starting a channel.** Pick a team. The team joins as a package: the lead is ready, specialists are set up with their persona, skills and MCPs but sleep until needed. A "solo" team (lead only) behaves like a normal session, for quick work.

**The lead.** One role per team that answers by default. The user talks to it naturally; it clarifies and pulls in specialists. Specialists post directly in the channel; the lead does not summarize them back and only speaks when addressed or @mentioned.

**Routing rules** (plain code, not an LLM):

1. A user reply inside a thread goes to whoever owns that thread
2. An @mention goes to that teammate
3. Anything else from the user goes to the lead

**How teammates talk.** Through the channel only, never directly. Each teammate gets an MCP tool from Reef, such as post\_to\_channel, to post results and @mention others. The router wakes the mentioned teammate; idle teammates cost nothing.

**Example flow.** User asks lead for Google login → lead asks @architect for a plan → plan posted as a thread, user replies in the thread with questions → architect hands off to @developer → developer asks @architect a doubt → developer pings @tester → tester reports back.

**Approval gates.** The user can require approval before work passes between roles. Defaults: lead → architect is free; lead → developer, architect → developer and developer → tester require approval. Each gate can be switched off per team.

**Caps** (plain code): max agent-to-agent hops without a user message (default 4), max turns per teammate per wake (default 25), token budget per channel (default 1M). On any cap the channel pauses and an item goes to the needs-you inbox.

**Crash recovery.** Every turn is recorded as running, done or failed. On restart, running turns are marked failed and a note is posted to the channel; the worktree may be dirty. No automatic retry; the user retries or rolls back to a checkpoint.

**Teammate identity.** Each teammate connects to Reef's MCP server with its own token, issued when the channel starts. One server, any number of teams and roles.

## Context and memory

Never preload history, only a summary; details are fetched on demand.

**What a teammate wakes up with:**

| Layer | Contents | When loaded |
| --- | --- | --- |
| Role | Persona, skills, tools, MCPs | Always |
| Feature brief | Goal, status, key decisions, open questions; rewritten after each session | Always |
| Role notes | This role's own notes for this feature, updated after each chat | Always |
| Index | One line per decision, session, PR | Always |
| Details | Full decisions, old chats, PR diffs, channel threads | On demand via tools |

**Reef as an MCP server.** The on-demand tools (read decision, search sessions, get PR diff, read thread) are exposed by Reef's own MCP server, so Claude Code and Codex query history the same way.

**Browser tools.** The same MCP server lets teammates drive the built-in browser: open and navigate, read the page (accessibility snapshot), click, type, take screenshots, read console and network logs. The user watches live in the browser pane and can take over at any time. The tester uses this to verify UI changes.

**Session lifetime.** Within a channel each teammate's session persists and resumes. When it gets long it is compacted into role notes and continues fresh. Across channels sessions are not shared: a teammate's memory of other channels is its role notes, the brief and the on-demand tools, never the raw session. After 30 to 40 chats the architect still starts small and relevant.

**Brief and role notes.** The agent writes them from the conversation and the shared understanding with the user; the user approves before they become the new brief. Decisions are append-only with author and source message.

**Capturing decisions.** Both ways: the agent records decisions as it goes via `record_decision` (with the source message), and at wrap-up it proposes any it noticed but did not record; those are approved with the brief.

**Wrapping up.** A channel ends explicitly: the user clicks "wrap up" or types `/wrap`. The agent then proposes the brief, role notes and decisions. No idle timers.

**Reef CLI.** The same brief, decisions and tasks are reachable from a terminal (`reef brief`, `reef decide`, `reef task`), so plain Claude Code works when the app is not open.

**Role views.** Same data filtered per role: the tester's brief emphasizes what was built and what to test; the architect's emphasizes decisions and constraints.

## Work outside features

Not everything needs a feature; four lighter places cover the rest.

- **Quick sessions:** a channel under a project with no feature, running on the main checkout. Promoting it creates the feature, re-parents the channel, and the lead reads the session to write the initial brief (costs one turn). It can also be attached to an existing feature.
- **Project memory:** conventions and architecture notes shared by every feature in a repo, like a smarter CLAUDE.md. A short version loads into every session.
- **Team workshop:** tune teams and roles (personas, skills, tools, MCPs) separately from project work.
- **Global tasks and board:** one Task entity tagged with project and optional feature, shown as a global todo list and as a kanban board. Tasks with no feature are personal todos; a feature's tickets are the same tasks filtered by tag.

## Supporting features

| Feature | What it does | Group |
| --- | --- | --- |
| Needs-you inbox | Every question, approval or finished task waiting on the user, with desktop notifications | Seamless |
| Worktree per feature | One worktree per feature on branch `feature/<slug>`, shared by all its channels. Quick sessions use the main checkout | Robust |
| Checkpoints and undo | Snapshot before each teammate's changes, one-click rollback | Robust |
| Diff review panel | Inline comments on changes go back to the developer's thread | Robust |
| Permissions per role | Only the developer edits code by default; it may spawn parallel subagents. Other roles are read-only unless the user grants write. Only the user merges to main | Robust |
| Quick capture | Global shortcut to drop an idea, todo or bug into a project | Seamless |
| Voice input | Talk to the lead by voice | Seamless |
| Daily digest | Morning summary of what teams did, what is blocked, what waits on the user | Seamless |
| GitHub integration | PR status and CI results flow into channels; failed CI pings the developer | Plug-in |
| Shared skills and MCP library | Define once, attach to any role | Plug-in |
| Inspector | Each teammate's prompts, tool calls, tokens and cost | Learning |
| Built-in terminal and browser | One window for terminal, browser, commits, PRs | Later |

## Tech stack

A Go server is the brain, a React UI is the face, and Electron wraps both into the Mac app (no separate browser needed), following T3 Code's structure.

| Layer | Choice | Role |
| --- | --- | --- |
| Backend | Go local server | Router, teammates, sessions, memory, Reef MCP server |
| Storage | SQLite | Projects, features, tasks, channels, decisions |
| Agent adapters | Claude Code and Codex CLIs in headless JSON-streaming mode, both in the baseline | One common Agent interface (Start, Send, Events, Interrupt, Resume); Jellyfish later |
| UI | React, rendered inside the Mac app window | Talks to the Go server over HTTP and Server-Sent Events on localhost |
| Desktop shell | Electron (the Mac app) | Main process launches the Go server; renderer shows the UI; hosts the built-in browser pane |
| Browser | Electron WebContentsView, driven via Chrome DevTools Protocol | Built-in browser pane; agents control it through Reef's MCP server (Go proxies calls to Electron) |
| Terminal | xterm.js | Built-in terminal pane |

**Reference.** T3 Code's apps/server, apps/web and apps/desktop folders. Its desktop app is Electron, with a preview module for the built-in browser.

## Decisions

| # | Decision | Why |
| --- | --- | --- |
| 1 | Teams and channels are core, not optional | Visibility into what each role does, and one window for the whole workflow |
| 2 | Journey stage is a label, never a gate | Alignment on where the feature is; gating is the approval-gate feature |
| 3 | Versions are a later idea, not baseline | Baseline the product first, ship one feature through it |
| 4 | Reef data lives in a central app folder, not in repos | Replaces run files and handoffs; smooth transition |
| 5 | Build from scratch in Go, copying T3 Code patterns | Learning goal; use Reef to build Reef once there is a baseline |
| 6 | Brief and role notes are agent-written, user-approved | Agent captures the shared understanding; approval keeps it trustworthy |
| 7 | Reef has a CLI alongside the app | Plain Claude Code keeps working when the app is closed |
| 8 | Sessions persist per channel; across channels memory is notes + tools, not sessions | Continuity inside a channel, small context everywhere else |
| 9 | Only the developer edits code by default; it may run parallel subagents | Avoids two roles writing one worktree; matches poteto-mode auto |
| 10 | Caps: 4 hops, 25 turns, 1M tokens per channel; pause and ask | Stops loops without an LLM judge |
| 11 | Failed turns are marked, never auto-retried | User decides retry vs rollback |
| 12 | Per-teammate MCP tokens on one server | Teams are configurable; ports per role do not scale |
| 13 | Lead does not summarize specialists back | User already sees the post; halves lead cost per handoff |
| 14 | Gates default on for architect→developer and developer→tester, off for lead→architect | Gate the steps that cost tokens and touch code |
| 15 | Decisions captured both live (tool) and at wrap-up (proposed, approved) | Accuracy plus a backstop |
| 16 | Channels end explicitly via wrap-up, no idle timers | Rewrite at the right moment, no wasted tokens |
| 17 | One worktree per feature, shared by its channels; quick sessions on main checkout | Follows from single-writer rule |
| 18 | Promotion re-parents the channel and the lead writes the initial brief | Brief exists from day one of the feature |
| 19 | Claude Code and Codex both in the baseline | Prove the Agent interface with two real backends |
| 20 | One stdio MCP process per teammate on one shared database, each with its own token (amends 12) | Works with the server down, adapters already launch stdio MCP servers, no ports |
| 21 | Live agent output streams to the UI over Server-Sent Events, one GET stream per channel; users send messages with POST | Standard library only, data flows one way, and EventSource reconnects |
| 22 | Routing runs when a turn ends, over the messages that turn stored. Wakes run one at a time per channel | No agent is interrupted, and the message log fixes the order |
| 23 | A message from an agent wakes only the roles it @mentions, also a reply in a thread; the thread owner rule is for the user | Ends a chain, and the lead does not answer every specialist post |
| 24 | Every agent starts when first woken. A specialist's agent stops after each wake and resumes its session next time; the lead stays live | Idle teammates cost nothing, and the lead is who the user talks to |
| 25 | A wake across a gate that is on is held as a pending approval: approve runs it, reject drops it, and a restart keeps it | The user decides before tokens are spent, and a crash must not lose the question |
| 26 | The hop count resets when the user sends a message or continues, not when the user approves a gate. A wake over the cap is held, and continue runs it | A user message is new intent; an approval only lets the same chain go on. Held, not dropped, so no work is lost |
| 27 | Threads have one level: a reply points at a root message, never at another reply. A teammate woken from a thread replies in it; one woken from a top-level message replies top-level and owns that thread | "Who owns the thread" stays one lookup, and the window stays simple |
| 28 | Wrap-up and promote turns wake nobody | They are the lead talking to the user about the feature, not a handoff |
| 29 | A team is a JSON file applied with `reef team apply`; a team `solo` (lead only) always exists; a channel keeps the team it was created with | Smallest way to configurable teams now; the team workshop can edit them later |
| 30 | A role's `tools` become Claude's allowed tools; Codex ignores them and is limited by its permission mode | Only Claude has a per-tool allow list |
| 31 | Continue on a channel over its token budget sets the budget to the tokens used plus 1M (amends 10) | A flat +1M left a channel that used far more than its budget paused again at once |
| 32 | Roles write only when the team file sets `write`; it maps to each backend's own limit (amends 9 and 30) | Checkpoints and the single-writer rule need one flag that does not depend on the backend |
| 33 | A feature's worktree lives in the data folder and is made at its first wake, from the project's HEAD | Follows decision 4, and a feature nobody woke costs nothing |
| 34 | Write turns on one tree run one at a time across channels | Decision 17's single writer holds with two channels on one feature |
| 35 | Rollback resets the files and the branch to the checkpoint, not the agent's session, and leaves a note in the channel | Sessions cannot be rewound; the note tells the agent |
| 36 | The needs-you inbox is a view over approvals, pauses, failed turns and who spoke last; no ask-user tool | No new state to keep in sync, and Reef cannot tell a question from an answer |
| 37 | Default gates also hold a lead's wake of the developer (amends 14) | Decision 14 gates the steps that touch code; an ungated lead→developer edge let the lead skip the architect→developer gate |

## Open questions

- [x] (spike, week 1) Exact event formats from Claude Code and Codex headless modes, mapped to the Agent interface. Answered in [agents.md](agents.md).
