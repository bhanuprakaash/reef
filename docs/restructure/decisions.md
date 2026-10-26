# Restructure decisions

Settled in discussion, 2026-10-23. Numbered R1… so they do not collide with `docs/product.md`; they move there when implementation starts. Re-open one only with a new numbered decision and a reason.

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
| R28 | Gemini (`gemini-embedding-001`) is a third embedding provider beside OpenAI and Voyage | Google gives a free tier, so meaning search costs nothing to try |
| R29 | A convention by an architect, lead or the user is accepted on write; other roles' conventions are candidates. Any writer may ask for approval (`needs_approval`), which makes the item a candidate whatever the role. Amends R21 | Architect and lead own how the team works; they can still put a rule they are unsure of in front of the user |
| R30 | The background pass that fills missing vectors (R20) waits for server wiring; until then a failed embed is reported on write and `reef memory embed <provider>` retries. A 429 from the vendor is retried once after its `Retry-After`, at most 60 seconds | Nothing long-lived runs before the server calls `memory/`; a silent failure would leave items out of meaning search unseen |
