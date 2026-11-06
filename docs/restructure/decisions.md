# Restructure decisions

> Status: done. Every decision below is now a numbered decision in [docs/product.md](../product.md#decisions), written in the new terms. This file is history: the wording below is the original, with Feature and channel where they were then. Re-open a decision only with a new numbered decision in `docs/product.md` and a reason.

Settled in discussion, 2026-10-23. Numbered R1… so they do not collide with `docs/product.md`.

| R | Product decision | R | Product decision | R | Product decision |
| --- | --- | --- | --- | --- | --- |
| R1 | 96 | R19 | 114 | R37 | 126 |
| R2 | 97 | R20 | 115 | R38 | 127 |
| R3 | 98 | R21 | 116 | R39 | 128 |
| R4 | 99 | R22 | 117 | R40 | 129 |
| R5 | 100 | R23 | 118 | R41 | 130 |
| R6 | 101 | R24 | 119 | R42 | 131 |
| R7 | 102 | R25 | 120 | R43 | 132 |
| R8 | 103 | R26 | 121 | R44 | 133 |
| R9 | 104 | R27 | 122 | R45 | 134 |
| R10 | 105 | R28 | 123 | R46 | 135 |
| R11 | 106 | R29 | 124 | R47 | 136 |
| R12 | 107 | R30 | 125 | R48 | 137 |
| R13 | 108 | R31 | 85 | R49 | 138 |
| R14 | 109 | R32 | 86 | R50 | 139 |
| R15 | 110 | R33 | 87 | R51 | 140 |
| R16 | 111 | R34 | 88 and 92 | R52 | 141 |
| R17 | 112 | R35 | 89 | R53 | 142 |
| R18 | 113 | R36 | 85 | R54 | 38 and 69 |

| # | Decision | Why |
| --- | --- | --- |
| R1 | Reef stays single-user. The one user creates every Teammate, Team, Space and Work item | Nobody shares a Reef, so scopes are retrieval filters for relevance, not access control |
| R2 | Memory service first; the prompt-assembly audit runs alongside it | Memory is the hardest open problem and the assembler is where memory plugs in |
| R3 | Feature stops being an object. It becomes one Work item type among bug, incident, investigation, task, improvement | Work is broader than features; one container, typed |
| R4 | A Team is a roster of Teammates plus routing rules, a responsibility text, a queue and coordination memory. It has no chat of its own | "Who works together and how" stays separate from "where talking happens" |
| R5 | A Space is where talk happens. A Work item has one active Space, plus continuations when one grows too long. Free Spaces with no Work item are allowed | One place holds the whole story of a piece of work |
| R6 | Phases (design, implementation, testing) are threads inside the Space, or sub-items. Never a Space per phase | Splitting phases splits context and costs fresh sessions |
| R7 | A Teammate proposes a Work item and suggests a Team from its responsibility text and the project's default team. The user creates and assigns. The Team's lead picks it up and opens the Space | Teammates never assign; the board stays the user's |
| R8 | Serial: one Work item per Team at a time. The top of the queue is next | Matches one-ticket-at-a-time and the single-writer rule |
| R9 | A direct conversation is with one Teammate. Typing `@Y` or "add Teammate" turns it into a Space with the user, X and Y; the conversation is marked moved and linked; Y gets a summary, never the transcript. Ask a Teammate stays in the chat | Adding someone is one motion; the two gestures stay distinct |
| R10 | Work item statuses: Backlog, Queued, In progress, Needs you, Review, Done, Dropped. Only the user sets Done. Journey stages are dropped | The Team gets to Review; acceptance is the user's |
| R11 | Proof is a field on the Work item: PR, test command and output, screenshots, ticked acceptance lines. The lead fills it at Review. A parent's status and proof derive from its children plus one end-to-end check | Verification becomes first-class instead of a comment |
| R12 | Approval gates are removed. Caps, checkpoints with rollback and Needs you remain | Review with proof checks the result; gates checked intent and stalled the queue |
| R13 | A Team creates sub-items under its Work item without approval; they stay with the same Team and surface as FYI. Inbox kinds: Needs you (blocks), Review (blocks that item), FYI (does not block) | Work moves while the user is away; the user can still drop or reassign |
| R14 | Clean break. Today's Features and channels are not migrated | Nobody uses Reef yet |

## Memory service (settled 2026-10-23)

| # | Decision | Why |
| --- | --- | --- |
| R15 | One `memory` table of items (kind, body, why, scope, author, role, status, retired_at, replaced_by) plus one `memory_link` table (from, rel, to_kind, to_id). Brief stays a versioned prose blob per Work item; role notes and the project-memory blob become items | Small items with links can be retired, searched and traced one by one; blobs cannot |
| R16 | Kinds: decision, convention, finding, fact, lesson. Scope is one of project, work_item, team, teammate, space, conversation, as two columns on the item | One shelf per item keeps the wake filter a plain WHERE |
| R17 | Link rels: about, source, replaces, related. Targets are plain strings, no foreign keys; `memory/` knows no other table. Links come from the writer's token context (free), the writer's arguments (`about`, `replaces`), and a lookalike check that proposes and never links silently. No model-based entity extraction | Most links are known at write time; a text check covers the rest |
| R18 | Items are append-only. `replaces` retires the old item with `replaced_by`; `retire` marks it stale with nothing replacing. Retired items never enter the wake prompt; recall may return them marked retired | Records that point at an item must stay true; history of what was believed stays readable |
| R19 | Search is hybrid: FTS5 with porter stemming (BM25) plus embeddings stored as a blob, fused by reciprocal rank fusion; then expand links one step, never through hubs (project, team). No reranker until evals show the need | Word-exact and meaning-based search cover each other's gaps; one step brings the neighbourhood without the whole project |
| R20 | Embeddings are optional: a setting names the provider (none, OpenAI, Voyage) and key. Without a key recall is FTS only. Embedding is synchronous on write, queued on failure, and a background pass fills missing vectors | Reef stays free and local by default; one field turns on better search |
| R21 | Candidate vs accepted: conventions and team lessons need user approval; decisions, findings, facts and lessons are accepted on write by any role, and the user can retire any. Authority is a short table in code, not config or prompting | Approval stays where a wrong item changes how the team works |
| R22 | Extraction is live via `remember` while working plus a wrap-up pass that proposes what was missed. Direct-conversation items get the scope the writer picks; conversation scope is searchable but never loaded at wake | Same as decisions today, generalised |
| R23 | Agent surface: `remember(kind, body, about?, replaces?, why?)`, `recall(query, limit?, scope?)`, `read_memory(id)` | Three tools, no table knowledge in the agent |
| R24 | Wake prompt: fixed counts per section (project 10, work-item decisions all live, findings 5, team 5, teammate 5, plus the brief), each section ending with `N of M; recall(..., scope=...)`. Sent once per session; later wakes get a delta of accepted items in scope since the role's last turn; compaction refreshes the snapshot | The prompt is read on every turn for the life of the session, so it stays small and the agent knows how to get more |
| R25 | Team memory: the lead proposes team lessons at wrap-up as candidates; members may write team-scope items mid-Space as candidates | Few, curated lessons about working together |
| R26 | Evals: a golden set of about 30 scenarios in `go test` asserting recall@10, MRR and that forbidden ids are absent; knowledge-update cases (replace, retire, conflict); token cost per wake recorded; hand-graded set with real embeddings on demand; an LLM judge only if ranking proves weak | Deterministic checks cover write policy and scope isolation, which published benchmarks skip |
| R27 | Build as package `memory/` in the repo, own tables in `reef.db`, eval harness under `memory/eval/`, no call from `channel/` or `server/` until evals pass. First build excludes the context assembler and MCP wiring. Old memory tables are not migrated | Isolation by package boundary; a service boundary buys nothing for one user on one Mac |
| R28 | Gemini (`gemini-embedding-001`, 768 dimensions, no task type) is a third embedding provider beside OpenAI and Voyage. Each provider has its own cosine floor: Gemini 0.6, OpenAI and Voyage 0.3 (not yet measured) | Google gives a free tier, so meaning search costs nothing to try. Measured on the eval: related 0.69 to 0.76, unrelated at most 0.57; a task type raised every score without widening the gap. The 0.92 lookalike threshold holds for Gemini: near-copies 0.98, reworded notes 0.84 to 0.87, unrelated 0.52 |
| R29 | A convention by an architect, lead or the user is accepted on write; other roles' conventions are candidates. Any writer may ask for approval (`needs_approval`), which makes the item a candidate whatever the role. Amends R21 | Architect and lead own how the team works; they can still put a rule they are unsure of in front of the user |
| R30 | The background pass that fills missing vectors (R20) waits for server wiring; until then a failed embed is reported on write and `reef memory embed <provider>` retries. `Backfill` waits out one 429 per item (its `Retry-After`, 1 to 60 seconds); writes and recall never wait | Nothing long-lived runs before the server calls `memory/`; a silent failure would leave items out of meaning search unseen |

## Context assembler (settled 2026-10-27)

Built on today's tables (feature, brief, role notes, project memory, decisions, tasks, channels). The memory slice swaps the sources; the shape stays. Moved to `docs/product.md` decisions 85–89; the rows below are the original wording.

| # | Decision | Why |
| --- | --- | --- |
| R31 | One assembler builds every first prompt, a feature channel's and a quick session's, in three sections in this order: `## You` (persona, skills, role, feature, project, the user, goal, stage, base branch), `## How to work` (team rules, then one line on the tools), `## What we know` (brief, project memory, your notes, live decisions, tasks in progress, one count line) | Instructions before knowledge, one header per kind of content; `projectContext` goes away |
| R32 | An empty part is left out: no "No brief yet.", "No notes yet." or "none" | Those lines tell the agent nothing |
| R33 | Tasks: only those in `doing` are listed. Open tasks and channels are one count line each with the tool to use: `23 open tasks (list_tasks). 3 earlier channels (search_sessions).` | A task has no owner and the wake names the work; the agent only needs to know more exists |
| R34 | A resumed role's wake starts with what changed since its session's previous turn: decisions added (by any author: an author is only a role name, shared across channels) or retired, as index lines right after the wake header, before any `reef wrote:` lines, and the brief in full when a newer version exists. A decision added and retired since is named as retired only. Tasks are not in it. Amended in review 2026-10-27 | A role otherwise never learns of a new decision or brief during its session |
| R35 | A Go test builds the first prompt for a feature with 60 decisions, 40 tasks and 15 channels: no section twice, each limit holds, under 8,000 characters without the persona. Another checks the delta holds only the new items. `turn.prompt` stays the only record of what was sent | Size is checked by a test, not by eye; no new storage |
| R36 | The team rules keep their wording in this run | The channel evals tested that exact text; a rewrite needs its own eval run |

## Domain model (settled 2026-10-27)

| # | Decision |
| --- | --- |
| R37 | `teammate` table: persona, skills, backend, model, face, plus default access rules (tools, MCP servers, write, permission mode) used in direct conversations. Adding a Teammate to a Team copies the defaults into a `team_member` row (with the lead flag); later edits to the Teammate do not change memberships. Memory `teammate` scope id is the teammate id |
| R38 | The `task` table goes. Tasks are Work items of type `task`, usually sub-items. `create_task` becomes `create_sub_item` |
| R39 | Full rename: feature → Work item, channel → Space in tables, Go, REST, MCP, UI and docs. One forward migration drops the old tables. "Channel" is kept for the later incoming-event streams |
| R40 | In this slice: model, board, Work item page, Spaces, Teammates and Teams, queue, proof and Review, inbox kinds, gate removal, direct conversations and R9. Out: Ask a Teammate, Channels and Automation, Space continuation, sub-item cap |
| R41 | Review frees the Team; the next Queued item starts by itself. Sent back → top of the queue. Needs you does not free the Team: the item still holds its worktree |
| R42 | One worktree, branch and PR per top-level Work item; sub-items share the parent's Space and worktree |
| R43 | `propose_work_item` creates a Backlog item "proposed by X" with a suggested Team, plus an FYI |
| R44 | The lead runs the wrap-up when it submits for Review; setting Done approves the wrap-up |
| R45 | Many direct conversations per Teammate; any can be continued; nothing splits by itself |
| R46 | Sub-items run one turn at a time in the Space; subagents inside a turn are fine; true parallel is a later slice |
| R47 | The lead sets Done on sub-items; only a top-level Done is the user's. A parent goes to Review only when every sub-item is Done or Dropped (amends R10) |
| R48 | A Work item has a description and acceptance lines. Proof: PR (found from the branch), each line ticked with a note, test commands with output, screenshots. The lead fills it with `submit_for_review` |
| R49 | Inbox: Needs you (cap pause, failed turn, held wake), Review (proof + wrap-up), FYI (sub-item created, item proposed, queued item started) |
| R50 | A Space has its own member list (`space_member`: teammate, lead flag, access rules copied from the Team membership or the Teammate's defaults) and optionally a Team. A free Space comes from R9 (no Team) or from "new Space" with a Team; it can be attached to a Work item (replaces promote), which moves its `space` items to the `work_item` scope |
| R51 | Memory scopes add `space` and `conversation`; `conversation` is never loaded at wake |
| R52 | Type is a label (icon, filter) with no behaviour. Only top-level Work items have a brief |
| R53 | Routing unchanged: user messages go to the lead; an @name at the start of a line hands off. A Team's responsibility text only suggests a Team for a proposed item |
| R54 | The hop cap counts only talk-only turns: no tool call, or only Reef's own MCP tools. Any other tool call resets it. Default 6. Token budget per Work item (1M; Continue adds 1M) is the backstop |
