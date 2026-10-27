# Server

`reef serve` keeps Reef's data in one SQLite file and serves it as a JSON REST API. The
store is [`store/`](../store), the routes are [`server/`](../server), the entry point is
[`cmd/reef/main.go`](../cmd/reef/main.go). The code is the source of truth. This doc says
what it does and why.

## Run

```
reef serve [--addr 127.0.0.1:7700] [--data DIR]
```

- `--data` is the data folder. It holds `reef.db` and `reef.lock`, and is created if
  missing. The default is `~/Library/Application Support/Reef`. Use `--data` for tests or a
  second instance.
- One server owns a data folder. `reef serve` locks `reef.lock` for as long as it runs; the
  lock is in `cmd/reef`, not in the store. A second `reef serve` on the same `--data` exits
  with `data folder ... is in use by another reef`, before it touches the data, whatever its
  `--addr`. A second instance needs its own `--data` and its own `--addr`.
- Other `reef` commands (the CLI, `reef mcp`, `reef chat`) do not take the lock. They open
  `reef.db` directly with `store.Open` while the server runs. `reef chat` takes its own
  `chat-<space id>.lock` while it runs, and so does the server for a live Space;
  restart recovery reads it (see below). WAL mode
  and a 5 second busy timeout let the processes share the file. Two processes that open a fresh folder at once
  both succeed: each migration runs in one `BEGIN IMMEDIATE` transaction and re-reads
  `user_version` inside it, so a migration is applied once.
- `--addr` is where to listen. The default is `127.0.0.1:7700`. The API has no login, so
  `reef serve` refuses an `--addr` whose host is not `localhost` or a loopback IP,
  including an empty host such as `:7700`, before it opens the data folder. It also refuses
  a request whose `Host` is not `127.0.0.1`, `localhost` or `::1` (403), and a `POST`,
  `PUT` or `PATCH` whose `Content-Type` is not `application/json` (415). This stops a web
  page on another origin from writing to it, including through DNS rebinding.
- The file opens in WAL mode with foreign keys on. Migrations in
  [`store/migrations`](../store/migrations) run at start, in order, each once. They only
  go forward. A file whose version is newer than the newest migration (made by a newer
  reef) is refused: `database is newer than this reef`.
- SIGINT or SIGTERM stops the server: it interrupts every running Space turn (stored
  `failed`, `interrupted`), stops every agent, then gives requests in flight one second.
  The agents stop in parallel, so stop time is about one agent stop (about a second), not
  one per live Space.

## Web UI

`reef serve` also serves the React app in [`web/`](../web). `web/dist` is embedded in the
binary with `go:embed` ([`web/embed.go`](../web/embed.go)). A fresh clone builds without
npm: `web/dist` holds only `.keep`, and a page load answers 503 `UI not built: run npm run
build in web/` until the app is built.

```
cd web && npm ci && npm run build
cd .. && go build -o reef ./cmd/reef
./reef serve
```

Then open `http://127.0.0.1:7700`. Build the web app before `go build`, because the binary
embeds `web/dist` as it was at build time. A Space's agent is the `claude` or `codex` CLI,
logged in and on `PATH`.

- Sidebar: a top row with the brand and a New Space button, search, the project switcher,
  then Today, Needs you and a Work items section: its header has a button to add a work item, and
  each top-level work item is a group, collapsible, over one row per Space
  (teammate creatures, name, a preview of the newest message from `lastMessage`, its time);
  quick sessions are listed after the work items. A work item's closed Spaces are folded under "Wrapped (N)" below its open ones, and closed work items under "Closed (N)" at the bottom. The list scrolls; the foot is fixed and shows your name beside the Teams and
  Settings links. Below 768 px the sidebar is a
  bottom tab bar (Today, Spaces, Needs you, Teams) and a Space opens full screen.
- Today (`/`): what waits on you, the project's work items, and capture.
- Space: bubbles with each author's creature, replies folded under their thread root with a
  count, and a side panel for the team, a thread, the inspector, the pull request or the diff (its expand button opens
  the review page). The header's `PR #N` chip opens the pull request panel: title, state, base and head, review, checks and
  Open on GitHub; Merge asks to confirm, then does `POST /work-items/{id}/pr/merge`. The running turn names its
  teammate. Teammates' direct messages (`GET /spaces/{id}/direct-messages`) are never bubbles: each conversation
  between two roles (from one public source message, or from none) is one compact item at the time of its
  first message, saying working, paused, failed, stopped or completed (the state of
  its newest chain) with its message count; it opens a Coordination panel with the exchange, the running direct turn's text and tool calls as they stream, a link to
  the public message it began from, and Continue or Retry for its state. A pause from the hop cap shows in
  the cap banner. A Space with rollbackable turns (`GET /turns?space=ID&rollbackable=1`)
  lists Checkpoints; Roll back on a row asks to confirm, then does `POST /turns/{id}/rollback`.
  A role whose newest ended turn has a tool call that claude refused for lack of permission (the
  result says it "requested permissions to use X" or "requires approval", or that an edit tool "is
  disabled for this session"), read from `GET /spaces/{id}/inspector`, gets a card
  "<Role> was denied <tool>" under the messages. A `Bash` call shows as `Bash(<first word>:*)`, and a
  compound command shows no card. Add to role reads `GET /teams/{name}`, appends the pattern to that role's
  `tools` and saves with `PUT /teams/{name}`; the card then says Added. The next wake of a specialist
  starts with it in `--allowedTools`; the lead keeps its agent between turns, so Add for the lead also
  does `POST /spaces/{id}/stop`. Add is disabled while a turn runs. A read-only role refused an
  edit tool shows "Needs write access" and a codex role "Codex ignores allowed tools", both with no Add.
  A checkpoint taken in another tree, such as a free Space's turn after an attach, is not
  listed. A stopped or failed turn shows `Failed: <error>` with its tool rows; its partial
  reply is the stored message, so the live block does not repeat it.
- A closed Space shows "Wrapped" and Reopen where the Wrap up button and composer were.
- Work item page (`/work-items/{id}`), with Close work item (asks to confirm) or Reopen, and its diff review page (`/work-items/{id}/review`); project
  page (`/projects/{id}`) with its work items. The work item page lists the work item's accepted `work_item` memory items and the project page the project's, each with Retire; the project page also adds an item (kind and body). The window reads and writes them through `/memory`, not `/decisions` or `/projects/{id}/memory`.
- Needs you (`/inbox`): what waits on you from [`/inbox/events`](#inbox), with a count and
  the action of each item (Approve or Reject, Continue, Retry, Open). The window asks once,
  on a click, to show desktop notifications and then shows one for each item that is new
  since the page loaded.
- Teams (`/teams`): lists teams and edits one's roles and faces, saving with
  `PUT /teams/{name}`. An opened role shows its persona, skills, allowed tools, permission
  mode and MCP servers. Skills are picked from the current project's `GET /skills` list for the
  role's backend; a picked name that is not on disk shows as missing. Allowed tools are
  checkboxes (`Read`, `Grep`, `Glob`, `Edit`, `Write`, `Bash`, `WebFetch`, `WebSearch`, and
  `mcp__<name>__*` for each MCP server) plus a field for patterns such as `Bash(git log:*)`,
  and are hidden on a Codex role. A server can be removed there, not added (use `reef team apply`).
  Duplicate applies the same roles and MCP servers under a new name with `?from=`,
  rejected if that name is taken. "New team" asks for a name and a team to start from and
  sends the same `?from=` request. The hop and token caps are not team fields, so the
  page has no Caps section.
- Settings (`/settings`): your name, occupation and working preferences (chips), the current project's skills read from disk, grouped Claude Code and Codex, each with its description, the Providers section (the claude and codex found on `PATH`, each with path, version and models, and added installs with a folder and Remove; add one with a kind, a name and a folder, typed or picked with Choose… on desktop; a role picks one in Teams), theme (dark, light or Match macOS, kept in `localStorage`)
  and notifications.
- First run: one page with two steps. Your name with an optional occupation and working preferences (saved with `PUT /profile`), then the first
  project. A name with no project starts at the project step; a project with no name asks only
  for the name.

Client routes such as `/features/3` and `/inbox` have the same paths as the REST API. In the Go server, `GET /` and `HEAD /` always get the app's `index.html`; other `GET` or `HEAD` requests get it when the `Accept` header includes `text/html`. Every other request goes to the API, so `curl /projects` and the app's own `fetch` calls (`Accept: application/json`) are unchanged. A file that exists in `web/dist`, such as `/assets/...`, is served as is. The Host and Content-Type checks guard the API only, not these files.

Dev mode: run `reef serve --addr 127.0.0.1:7701 --data ~/Library/Application\ Support/Reef\ Dev`,
then `cd web && npm run dev` and open `http://localhost:5173`. Vite proxies the API paths to
`127.0.0.1:7701`, the dev copy's port and folder, so it never reaches the installed app on 7700.

## Errors

Every error has a JSON body: `{"error": "work item 9: not found"}`.

| Status | When |
| --- | --- |
| 400 | Bad JSON, an unknown field or data after the JSON body, a bad or missing field, a non-number id, an unknown parent id inside a body (`unknown project 9`) |
| 403 | The `Host` is not `127.0.0.1`, `localhost` or `::1`. Body is `{"error":"forbidden host"}` |
| 404 | The id in the path (or a required query filter) matches nothing, or no route knows the path. Body for an unknown path is `{"error":"not found"}` |
| 405 | The path exists, the method does not. Body is `{"error":"method not allowed"}` |
| 409 | A unique rule broke. A send to a Space with a wrap turn running, to a Space paused by a cap other than the hop cap or to a Space open in a `reef chat`; `continue` on a Space that is not paused or has a turn running; `retry` of a turn that is not [retryable](#spaces-live), or on a Space with a turn running or paused; `rollback` on a Space with a turn running or open in a `reef chat`, on a tree other than the checkpoint's, or on a tree a turn runs on; an approval that is not pending, or one approved while the Space is paused |
| 415 | A `POST`, `PUT` or `PATCH` without `Content-Type: application/json`. Charset parameters are fine |
| 503 | A send after the server began to stop. Body is `{"error":"server is stopping"}` |
| 500 | Anything else. Body is `{"error":"internal error"}`; the detail goes to the log |

An id in the path that matches nothing is 404; an id in the body that matches nothing is
400. Unknown JSON fields in a request are rejected (`{"error":"invalid: bad JSON: json: unknown
field \"feature_id\""}`), so a misspelled field never turns into a silent default. The
server-set fields an entity returns (such as `id` and `createdAt`) are accepted and ignored. A parent
deleted while a write is in flight gives the same 404 or 400 as one that never existed.

## Entities

Ids are integers and are never reused after a delete. Instants (`createdAt`, `startedAt`, ...) are RFC 3339 UTC strings.
A type with `?` may be `null`. The headings below are the SQL tables in
[`001_init.sql`](../store/migrations/001_init.sql), migration 2
[`002_token.sql`](../store/migrations/002_token.sql) (`token`), migration 3
[`003_proposal.sql`](../store/migrations/003_proposal.sql) (`proposal`), migration 6
[`006_team.sql`](../store/migrations/006_team.sql) (`team`, `space.team_id`),
migration 7 [`007_thread.sql`](../store/migrations/007_thread.sql) (`message.parent_id`) and
migration 8 [`008_approval.sql`](../store/migrations/008_approval.sql) (`gate`, `approval`; the `gate` table is dropped again by migration 43) and
migration 9 [`009_approval_kind.sql`](../store/migrations/009_approval_kind.sql) (`approval.kind`) and
migration 10 [`010_role_write.sql`](../store/migrations/010_role_write.sql) (`role.write`) and
migration 11 [`011_turn_wake.sql`](../store/migrations/011_turn_wake.sql) (`turn.wake_text`, `turn.wake_parent`, `turn.retried`) and
migration 12 [`012_turn_checkpoint.sql`](../store/migrations/012_turn_checkpoint.sql) (`turn.checkpoint`) and
migration 18 [`018_profile.sql`](../store/migrations/018_profile.sql) (`profile`) and
migration 32 [`032_provider.sql`](../store/migrations/032_provider.sql) (`provider`, `role.provider`) and
migration 33 [`033_space_name_auto.sql`](../store/migrations/033_space_name_auto.sql) (`space.name_auto`) and
migration 36 [`036_turn_scopes.sql`](../store/migrations/036_turn_scopes.sql) (`turn.scopes`, `approval.scopes`) and
migration 37 [`037_turn_session_totals.sql`](../store/migrations/037_turn_session_totals.sql) (`turn.session_cost_usd`, `turn.session_input_tokens` and the other three token counts) and
migration 38 [`038_proposal_items.sql`](../store/migrations/038_proposal_items.sql) (`proposal.items`; drops `proposal.notes` and `proposal.decisions`, clears `proposal.retire` and rejects the wrap proposals still pending) and
migration 39 [`039_turn_compacted.sql`](../store/migrations/039_turn_compacted.sql) (`turn.compacted`) and
migration 43 [`043_drop_gates.sql`](../store/migrations/043_drop_gates.sql) (drops `gate`; deletes the `approval` rows of kind `gate`).
Migrations 4 and 5 add columns to `space` (see [Space](#space)).

### project

| Field | JSON | Notes |
| --- | --- | --- |
| `id` | number | |
| `name` | string | required |
| `path` | string | required; the repo folder, stored as an absolute path with symlinks resolved |
| `createdAt` | string | |

### profile

The person using Reef: one row, no id. The window asks for the name on first run and shows it
in the sidebar.

| Field | JSON | Notes |
| --- | --- | --- |
| `name` | string | required; trimmed |
| `occupation` | string | default `""`; trimmed. The window offers Developer, Designer, Product manager, Founder, Student and Other |
| `style` | string array | default `[]`; trimmed, empty entries dropped, kept as one newline-joined column. The window offers six working preferences |
| `updatedAt` | string | |

The server stores `occupation` and `style` as given, but a line break in `name`, `occupation` or a style entry is 400 (the context prompt is built from them); the window owns the option lists. The context prompt names the person and, when set, the occupation (except `Other`) and style (see [memory.md](memory.md#the-context-prompt)).

### project_memory

One body per project, kept as a column on `project` (migration 14
[`014_project_memory.sql`](../store/migrations/014_project_memory.sql)). Nothing loads or shows it
since decision 91: the context prompt and the project page read the project's memory items instead
(see [memory.md](memory.md#the-context-prompt)). Its routes still read and write it.

| Field | JSON | Notes |
| --- | --- | --- |
| `projectId` | number | from the path |
| `body` | string | default `""` |

### work_item

One row per Work item. A Work item has a `type` that is only a label; it never changes how it behaves. One level of nesting: a sub-item has a `parentId` and cannot have sub-items.

| Field | JSON | Notes |
| --- | --- | --- |
| `id` | number | |
| `projectId` | number | required on create; never changes. Deleting the project deletes the item |
| `parentId` | number? | `null` for a top-level item. Must be a top-level item of the same project, else `invalid: work item 2 is a sub-item and cannot have sub-items`; never changes. Deleting the parent deletes its sub-items |
| `type` | string | `work item` (default), `bug`, `incident`, `investigation`, `task` or `improvement`, else `invalid`. A `PUT` without `type` keeps the old one |
| `title` | string | required |
| `description` | string | default `""` |
| `status` | string | `backlog` (default), `queued`, `in_progress`, `needs_you`, `review`, `done` or `dropped`, else `invalid`. The store only keeps it; the rules for who may move it where are not enforced yet. A `PUT` without `status` keeps the old one |
| `teamId` | number? | the Team that works on it |
| `proposedBy` | string | default `""`; set on create only |
| `suggestedTeamId` | number? | |
| `queuePosition` | number? | `null` when not queued |
| `tokenBudget` | number | default 1,000,000; a `PUT` with 0 keeps the old one |
| `createdAt` | string | |
| `closedAt` | string? | `null` while the item is open. Set by `POST /work-items/{id}/close`, cleared by `/reopen`; never set by a client |
| `acceptanceLines` | array | `{position, text, checked, note}` in order, `position` counting from 1. On create and `PUT`, the array replaces all lines (`position` is the index in it); a `PUT` without `acceptanceLines` keeps them. A line with no `text` is `invalid` |

A `PUT` replaces every editable field: `title`, `description`, `type`, `status`, `teamId`, `suggestedTeamId`, `queuePosition`, `tokenBudget`, `acceptanceLines`. A field left out clears `teamId`, `suggestedTeamId` and `queuePosition`, so send the whole item back.

### work_item_github

The last GitHub PR state the [GitHub poll](#github-poll) posted about a Work item, kept across
a restart so it does not repeat a note. No REST route; tables `work_item_github` (one row per
item: `pr_number`, `pr_state`, `pr_url`) and `work_item_github_check` (one row per
`(work item, check name, run key)` of the item's PR: `run_key`, `woken`; a row's existence
means it was noted), migration 42
[`042_work_item.sql`](../store/migrations/042_work_item.sql).

### brief

One row per version. A work item's brief is its latest version.

| Field | JSON | Notes |
| --- | --- | --- |
| `id` | number | version order |
| `workItemId` | number | from the path |
| `body` | string | required |
| `author` | string | required |
| `createdAt` | string | |

### role_notes

One row per work item and role. Nothing reads them since decision 91: a role's own items are in
scope `teammate`. Their routes still read and write them.

| Field | JSON | Notes |
| --- | --- | --- |
| `workItemId` | number | from the path |
| `role` | string | from the path |
| `body` | string | may be empty |
| `updatedAt` | string | |

### Space

| Field | JSON | Notes |
| --- | --- | --- |
| `id` | number | |
| `kind` | string | `work_item` (the one active Space of a work item), `free` (no work item) or `conversation` (a direct conversation with one Teammate). Set by the server |
| `projectId` | number | required on create; never changes |
| `workItemId` | number? | set on create. It changes only from `null`, once, when [attach](#attach-a-space-to-a-work-item) gives a free Space its work item. `null` is a free Space or a conversation. Must be a work item of the same project, and a work item has one active Space (`409` for a second). Deleting the work item deletes the Space |
| `name` | string | required |
| `role` | string | the name of the lead role of the Space's team. Empty when not set. For a Space from before this field existed, the role of its newest turn |
| `backend` | string | `claude` or `codex`, the agent that runs the Space. Empty when not set. For an older Space, the backend of its newest turn |
| `permissionMode` | string | passed to the backend as is when the Space's agent starts or resumes. Empty means the backend default |
| `paused` | string | why the Space is paused (see [Caps](#caps)). Empty when it is not paused. Set by the server only; a client cannot set it |
| `tokenBudget` | number | tokens the Space may use before it pauses. Starts at 1,000,000. Set by the server only |
| `team` | string | the name of the Team the Space's members were copied from. `solo` when none is given. Empty for a Space made from one Teammate. Set on create, never changes. The Space keeps its own member list (`space_member`: the Teammate, its lead flag and its access rules), so editing the Team later does not change a running Space |
| `createdAt` | string | |
| `closedAt` | string? | `null` while the Space is open. Set when its wrap-up is approved and when its work item is closed, cleared by `POST /spaces/{id}/reopen`. A closed Space takes no turn (409) |

### message

| Field | JSON | Notes |
| --- | --- | --- |
| `id` | number | |
| `spaceId` | number | required |
| `parentId` | number? | the id of the thread root this message replies to. `null` for a top-level message. It must be a top-level message of the same Space: a reply to a reply is 400. Set on create |
| `author` | string | required; free text, for example `user` or a role |
| `body` | string | required |
| `createdAt` | string | |

A top-level message is the root of a thread. The author of the root owns the thread.

### turn

One agent run in a Space.

| Field | JSON | Notes |
| --- | --- | --- |
| `id` | number | |
| `spaceId` | number | required |
| `role` | string | required |
| `backend` | string | required; `claude` or `codex` |
| `sessionId` | string | default `""`; set on create, while the turn runs, or when it ends |
| `status` | string | `running`, `done` or `failed`. A new turn is always `running` |
| `error` | string | default `""` |
| `usage` | object | `inputTokens`, `outputTokens`, `cacheReadTokens`, `cacheWriteTokens` (numbers, all four always present). The SQL columns are `input_tokens` and so on. The turn's own tokens: for claude, the session total the turn reported minus the total the session's previous turn reported (see `sessionUsage`), so a resumed turn's subagent and side-call tokens count; a running turn holds the sum of its messages so far |
| `startedAt` | string | |
| `endedAt` | string? | `null` while running |
| `wakeText` | string | default `""`; the text the turn ran: the unread messages of the wake, without the context prompt. Empty for a turn from before this field existed and for a wrap-up turn |
| `wakeParent` | number? | the thread the wake was in, `null` for the top level. Set on create with `wakeText` |
| `directMessageId` | number? | the direct request or response the turn was woken by; `null` for a public wake. Kept so retry preserves private reply routing |
| `retried` | boolean | `true` once a retry of this failed turn has started. Set by `retry`, never on create |
| `checkpoint` | string? | the git ref of the snapshot taken before the turn, `refs/reef/checkpoints/<data folder>/<turn id>`. `null` for a turn of a role without `write` and without a file-changing shell command in its `tools` (see [Spaces live](#spaces-live)), for a turn in a folder that is not a git checkout or has no commit, and for a turn from before checkpoints. Set by the server, never by a request |
| `model` | string | default `""`; the backend's model name, set from the agent's first event once the turn ends |
| `costUsd` | number? | `null` until the turn ends with a cost; codex turns never get one. A failed turn gets one too, when claude reported one. The turn's own cost: the session's running total the turn reported minus the total the session's previous turn reported. A session stored before totals were kept (all its turns have `sessionCostUsd` null) is taken to have cost, so far, its largest `costUsd` plus the `costUsd` of the turns stored after it: a resumed process held a running total, a long-lived one stored its first total and then each turn's own share |
| `sessionCostUsd`, `sessionUsage` | number?, object? | not in the turn's own JSON; the running totals of the turn's whole session as claude reported them when the turn ended (`sessionUsage` has the four token counts). `null` for a turn from before migration 37 [`037_turn_session_totals.sql`](../store/migrations/037_turn_session_totals.sql), for a turn that ended with no report (a stopped or crashed turn) and for every codex turn. The SQL columns are `session_cost_usd`, `session_input_tokens`, `session_output_tokens`, `session_cache_read_tokens`, `session_cache_write_tokens`. A turn's usage is its total minus the latest total before it plus the `usage` of the stopped turns after that one, so a stopped turn's tokens are not counted twice |
| `prompt` | string | not in the turn's own JSON (`GET /turns`, `GET /turns/{id}`); the full text sent to the agent (context preamble and notes included), stored when it is sent, so a running turn has it; a turn whose send fails stores it when it ends. Empty for a turn that failed before its prompt was built. `GET /spaces/{id}/inspector` has its own view of it |
| `reply` | string | not in the turn's own JSON; the plain reply of a done turn that already posted with `post_to_space`, kept here and not stored as a message (see [Routing](#routing)). `""` for every other turn. `GET /spaces/{id}/inspector` has its own view of it |

### tool_call

One tool call of a turn. Input and result are cut to 4 KB each. There is no update or
delete; deleting the turn deletes its tool calls.

| Field | JSON | Notes |
| --- | --- | --- |
| `id` | number | |
| `turnId` | number | required |
| `name` | string | the tool's name |
| `input` | string | cut to 4 KB |
| `result` | string | default `""` until the call's result arrives, then cut to 4 KB |
| `parentId` | number? | the `Agent` call a claude subagent's call nests under, set from the event's `parentToolId`; absent for a top-level call, and for a call whose parent was not seen (shown flat) |

### decision

No agent, CLI command or window page reads or writes this table since decision 91: a decision is a
memory item of kind `decision` (`remember`, `reef decide`, `POST /memory`). Its routes still work.

| Field | JSON | Notes |
| --- | --- | --- |
| `id` | number | |
| `workItemId` | number | required |
| `messageId` | number? | optional. Must be a message in a Space of the same work item. `null` if that message is later deleted |
| `body` | string | required |
| `why` | string | default `""` |
| `author` | string | required |
| `createdAt` | string | |
| `retiredAt` | string? | `null` while the decision is live. Set when it is retired: stale, duplicate or replaced. A retired decision is never edited or deleted, and still reads |
| `replacedBy` | number? | the decision that replaced it, if one did; `null` for a plain retire |
| `replaces` | array? | input to create, echoed in its response and never in a read: ids of live decisions of the same work item that the new decision retires, with their `replacedBy` set to it. Any other id is `invalid: decision 9 is not a live decision of work item 1` and nothing is written |

### team

A named set of roles. A team has exactly one lead role. `reef
team apply` (see [memory.md](memory.md#teams)) and `PUT /teams/{name}` both write a team,
through the same validation. Migration 6 seeds the team `solo`, with one role, `lead` on
`claude`, and points every Space that exists at it.

| Field | JSON | Notes |
| --- | --- | --- |
| `name` | string | required; unique; letters, digits, `_` and `-` only. Applying a name that exists replaces that team's roles in one transaction |
| `roles` | array | [roles](#role), the lead first, then the others in the order of the file |

### teammate

One persistent agent that can sit in several teams. Migration 41
([`041_teammate.sql`](../store/migrations/041_teammate.sql)) drops the `role` table and
adds `teammate` and `team_member`; nothing is migrated except the seed: one default teammate `lead`
(`claude`, `write`), the lead of `solo`.

| Field | JSON | Notes |
| --- | --- | --- |
| `id` | integer | the id of its memory `teammate` scope, so a rename keeps its items |
| `name` | string | required; unique; same rule as a role name |
| `persona`, `skills`, `backend`, `model`, `provider`, `face` | | as a [role](#role) |
| `tools`, `mcpServers`, `write`, `permissionMode` | | default access rules. Joining a team copies them into a `team_member` row; editing them later does not change any membership. REST shows `mcp` (name and label) instead of `mcpServers` |

A teammate that sits in a team cannot be deleted: the error is `invalid:` and names the team. A team has at
most one lead; a second one is `invalid`. `solo` keeps its one lead and takes no member changes.
A team member row holds the name the team calls the teammate (`name`, unique in the team; the role name below),
`lead`, and its own `tools`, `mcpServers`, `write` and `permissionMode`. The rest of a role is read from the teammate.
`reef team apply` and `PUT /teams/{name}` write the teammate behind each role: the one already behind that
role name in the team, else a new one called `<team>-<role>`.

### role


| Field | JSON | Notes |
| --- | --- | --- |
| `name` | string | required; unique within the team; letters, digits, `_` and `-` only, and not `user`. It is the `author` of what the role posts, and the name an `@mention` uses |
| `lead` | boolean | exactly one role of a team is the lead. The lead answers a message that nobody else owns |
| `write` | boolean | default `false`. Only a role with `write` may change files; it gets a checkpoint and the tree's write lock. claude: without it the role never gets `Edit`, `Write`, `MultiEdit` or `NotebookEdit` (they are dropped from `tools` and passed as `--disallowedTools`, so an allow rule in the user's or project's `.claude/settings.json` cannot grant them) and runs in the `default` permission mode, so `Bash` runs only as `tools` lists it. A claude role without `write` cannot list plain `Bash`, `Bash(*)` or `Bash(:*)` in `tools`: it would still change files through the shell, so applying the team is `invalid`; list patterns such as `Bash(go test:*)` instead. A role saved before this check existed has it dropped from `tools` at session start too, the same as the edit tools. `Bash` patterns are trusted as given, so `Bash(git diff:*)` also lets `git diff --output=file` write a file: such a role counts as a writer (see the write lock below). With it the role keeps its `tools` and permission mode (it still needs `acceptEdits`, or `Edit` in `tools`, to edit unasked) and also gets `Task`. codex: without it the sandbox is `read-only`; with it `workspace-write` unless the role's `permissionMode` is set. The seeded `solo` lead has `write` |
| `backend` | string | required; `claude` or `codex` |
| `permissionMode` | string | `""` is the backend default; claude: `default` or `acceptEdits`; codex: `read-only` or `workspace-write`. The same rule as a Space's `permissionMode` |
| `model` | string | optional; the model the role runs on, passed to the backend when its agent starts or resumes: claude `--model NAME` (`opus`, `sonnet`, `haiku` or a full model id), codex the thread `model`. Left out or `""` is the backend default |
| `provider` | string | optional; the `name` of an added [provider](#provider) whose binary the role runs instead of the `claude` or `codex` on `PATH`. It must exist and have `kind` equal to the role's `backend`, else the team is `invalid`. Left out or `""` runs the one on `PATH` |
| `persona` | string | goes at the start of the role's first prompt, under `## You`, and again, marked as changed, at its next wake once it or `skills` differs from what the role's session last got |
| `skills` | array of strings | names of skills, one per entry, trimmed; each is letters, digits, `:`, `_` or `-`, and an empty, repeated or other name is `invalid`. They go after the persona, under `Your skills:`. claude: a role that picks skills may use only these. `--allowedTools` gets `Skill(name)` for each and `--disallowedTools` gets `Skill(name)` for every other claude skill on disk (the scan of `GET /skills`, taken when the role's session starts, with the role's working folder). A role that picks none keeps every skill and gets no skill rule. Skill rules only add entries to the `write` and `tools` rules. codex: prompt only. A team file or `PUT` body that gives `skills` as a string is `invalid` |
| `face` | string | optional; the creature the window draws for the role: `jellyfish`, `whale`, `penguin`, `shark`, `octopus`, `turtle`, `seahorse`, `otter`, `clownfish`, `manta-ray` or `pebble`. Left out, the window picks one by the role's name (`lead` jellyfish, `architect` whale, `developer` penguin, `tester` shark, else `pebble`). Any other value is `invalid` |
| `tools` | array of strings | claude: passed as `--allowedTools` with `mcp__reef__*`, so these tools and the reef tools run without asking; the role's own `mcpServers` do not (list `mcp__mcp1__*` here to allow one). codex: ignored |
| `mcpServers` | array | extra MCP servers for the role, each `command`, `args`, `env` (stdio) or `url` (HTTP). They are named `mcp1`, `mcp2`, ... next to the `reef` server |

A Space's lead takes the Space's own `backend` and `permissionMode`, which `POST
/spaces` filled from the lead role unless the request set them. The other roles use their
own.

### provider

A claude or codex install the user added, next to the ones found on `PATH`. The ones on `PATH`
are not stored: `GET /providers` finds them each time, with `exec.LookPath`, the same lookup the
adapters use when a role names no provider.

| Field | JSON | Notes |
| --- | --- | --- |
| `name` | string | required; unique; letters, digits, `_` and `-` only, and not `claude` or `codex` (those name the ones on `PATH`) |
| `kind` | string | required; `claude` or `codex` |
| `folder` | string | required; an absolute path to an existing folder that holds an executable named like the kind: `FOLDER/KIND`, else `FOLDER/bin/KIND` |
| `createdAt` | string | |

A role runs a provider's binary by naming it in `provider`. The check runs when the team is
applied (the provider exists, its `kind` is the role's `backend` and its folder still holds the
binary) and again when the role's agent starts, because the binary may be gone from the folder
after that. A Space's lead takes the Space's `backend`; when that differs from the lead
role's, the role's `provider` is ignored and the `PATH` binary runs. A start that fails the check
is a `failed` turn whose `error` says why, for example `role developer: no claude binary in
/opt/work`; nothing falls back to the `PATH` binary, as that would run a different one than the
user picked. A role with no `provider` is unchanged. A provider a role names cannot be removed.

### approval

A wake that a pause holds. The SQL table is `approval`; its rows have kind `hold`.

| Field | JSON | Notes |
| --- | --- | --- |
| `id` | number | |
| `spaceId` | number | |
| `from` | string | the role whose message woke it |
| `to` | string | the role that would run |
| `messageId` | number | the message that woke it; `0` for a direct wake |
| `directMessageId` | number? | the direct message that woke it, when the wake is direct |
| `text` | string | the wake as it was routed, for example `[Space 5, message 28]\nlead wrote: @architect design a form`; the turn's own text is built when it runs |
| `status` | string | `pending`, or `approved` once `continue` has claimed it, or `rejected` once a user message dropped it |
| `createdAt` | string | |

A hold is never listed or decided through the API, see [Caps](#caps). Deleting the Space
deletes its approvals.

### token

Lets one teammate's `reef mcp` act as one role in one Space. No REST route: `reef
token`, `reef chat` and the server (one per teammate session) make them, `reef mcp` reads
them, and the one that made a token deletes it when its session ends. See
[memory.md](memory.md#tokens).

| Field | JSON | Notes |
| --- | --- | --- |
| `token` | string | the key; 64 hex characters from 32 random bytes |
| `spaceId` | number | required. Deleting the Space deletes the token |
| `role` | string | required; the author of what the teammate writes |
| `createdAt` | string | never expires |

### proposal

A wrap-up proposal: the new brief and the memory items to save.
`propose_wrap` makes one. `propose_brief` makes a brief proposal: only the brief, made
mid-space. Approving it adds a brief version and leaves the memory items and the
Space as they are; the Space's agent is not stopped. A Space has one pending proposal at most: a new wrap rejects the older pending ones of any kind, a new brief rejects the older pending briefs and is refused with `invalid: space 1 has a wrap-up waiting for approval` while a wrap is pending, and approving a brief on a closed Space is `invalid: space 1 is closed`. `reef chat` prompts for approval,
and `reef wrap approve` or `reef wrap reject` decides it; the server offers the same over REST (see [Wrap-up and
attach](#wrap-up-and-attach)). See [memory.md](memory.md#wrap-up).

| Field | JSON | Notes |
| --- | --- | --- |
| `id` | number | |
| `spaceId` | number | required; must be a Space of a work item. Deleting the Space deletes the proposal |
| `role` | string | required; author of the brief and items it applies |
| `kind` | string | `wrap` (default) or `brief`; any other is `invalid: unknown proposal kind "x"` |
| `brief` | string | required, not empty; the full new brief |
| `items` | array | `{"kind", "body", "why", "scope", "replaces"}` each; `body` required. `scope` is `work_item` (the default `propose_wrap` fills in), `project`, `teammate` or `team`, and `team` only when the proposing role leads the Space's team. `replaces` is optional, ids of live memory items it replaces. `[]` by default. The SQL column holds them as JSON text. Approving saves each with `remember`: see [memory.md](memory.md#wrap-up) |
| `retire` | array | ids of live memory items that are stale or duplicate; approving retires them. `[]` by default. The SQL column holds them as JSON text. `propose_wrap` checks that each `retire` and `replaces` id is an accepted item in a scope the role can read (`invalid: memory 9 is not a live item in this space's scopes`) and stores nothing otherwise; `CreateProposal` itself does not read memory. An id claimed twice (in `retire` and a `replaces`, or two `replaces`) is kept for its first claim, replaces before `retire`. At approve an id that is already retired is skipped |
| `status` | string | `pending` (default), `approved` or `rejected`. Decided once |
| `createdAt` | string | |
| `decidedAt` | string? | `null` while pending |

## REST

`{id}` is a number. Lists come back as `[]` when empty, ordered by id (oldest first).
Create returns 201 and the row. Update returns 200 and the row. Delete returns 204 and no
body. Deleting a row deletes what hangs off it by foreign key: a project takes its
work items, Spaces; a work item takes its briefs, notes, decisions and Spaces;
a Space takes its messages, turns, tokens, proposals and approvals; a turn takes its
tool calls.

| Route | Filters and body | OK | Notable errors |
| --- | --- | --- | --- |
| `GET /health` | | 200 `{"status":"ok"}` | |
| `GET /status` | | 200 `{"running":N}`, N the Spaces whose live state is not idle (a turn is starting, running or stopping); the desktop app polls it to update when idle | |
| `GET /projects` | | 200 | |
| `POST /projects` | `name`, `path` | 201 | 400 missing field, or a `path` that is not an absolute path to an existing folder |
| `GET /projects/{id}` | | 200 | 404 |
| `PUT /projects/{id}` | `name`, `path`, `baseBranch` (optional; the branch a new work item starts from and its PRs target; empty means the repo's default branch) | 200 | 400 (same `path` rule, or a `baseBranch` that is not a branch name), 404 |
| `DELETE /projects/{id}` | | 204; the worktrees of its work items and the checkpoint refs of the turns of all its Spaces are removed, its branches are kept | 404, 409 a work item's worktree has uncommitted changes (nothing is removed) |
| `GET /projects/{id}/memory` | | 200; `body` is `""` when none has been set. Unused since decision 91 | 404 |
| `PUT /projects/{id}/memory` | `body`; replaces the old memory | 200. Unused since decision 91 | 404 |
| `GET /profile` | | 200 | 404 no name set yet |
| `PUT /profile` | `name`, `occupation`, `style`; replaces the old profile | 200 | 400 empty name or a line break in a field |
| `GET /work-items` | `?project=ID`, `?parent=ID`, `?status=S`, `?type=T` | 200, with each item's `acceptanceLines` | 400 bad id, status or type |
| `POST /work-items` | `projectId`, `parentId`, `type`, `title`, `description`, `status`, `acceptanceLines` and the other fields above | 201 | 400 unknown project, unknown parent, a parent that is a sub-item, bad type or status |
| `GET /work-items/{id}` | | 200 | 404 |
| `PUT /work-items/{id}` | the editable fields above | 200 | 400, 404 |
| `POST /work-items/{id}/close` | no body needed | 204; sets `closedAt` on the work item and on each of its Spaces, and stops their agents | 404, 409 a Space has a turn running (nothing is closed) |
| `POST /work-items/{id}/reopen` | no body needed | 204; clears the work item's `closedAt` only, its Spaces stay closed | 404 |
| `DELETE /work-items/{id}` | | 204; the work item's worktree and the checkpoint refs of its turns are removed, its branch is kept | 404, 409 the worktree has uncommitted changes (nothing is removed) |
| `GET /work-items/{id}/brief` | | 200 latest version | 404 no work item, or no brief yet |
| `PUT /work-items/{id}/brief` | `body`, `author` | 201 new version | 400, 404 |
| `GET /work-items/{id}/brief/versions` | | 200 oldest first | 404 |
| `GET /work-items/{id}/notes/{role}` | | 200. Unused since decision 91 | 404 no work item, or no notes for the role |
| `PUT /work-items/{id}/notes/{role}` | `body`; replaces the old notes | 200. Unused since decision 91 | 404 no work item |
| `DELETE /work-items/{id}/notes/{role}` | | 204. Unused since decision 91 | 404 no work item, or no notes for the role |
| `GET /work-items/{id}/worktree` | | 200 `{"path": "...", "branch": "work-item/<id>"}`; the path of the work item's worktree, or `""` when it has none yet, and its branch. A sub-item answers with its parent's. Never makes one | 404 |
| `POST /work-items/{id}/worktree` | no body needed | 200 `{"path": "...", "branch": "..."}`; makes the work item's worktree (a sub-item's is its parent's) the same way the first wake of one of its Spaces would, reusing it when it already exists | 404 |
| `GET /work-items/{id}/diff` | query `scope=current` for the changes against the worktree's HEAD only | 200 the worktree's changes against the commit its branch and the project's base branch last shared, uncommitted and untracked files included, as `[{"path", "status": "added"\|"modified"\|"deleted", "hunks": [{"header", "lines": [{"kind": "add"\|"del"\|"ctx", "old", "new", "text"}]}]}]`; `[]` when the work item has no worktree yet | 404 |
| `GET /work-items/{id}/pr` | | 200 `{"pr": {"number", "title", "state": "OPEN"\|"MERGED"\|"CLOSED", "url", "isDraft", "reviewDecision", "base", "head", "canMerge", "checks": [{"name", "state": "pass"\|"fail"\|"pending"}], "body", "additions", "deletions", "files": [{"path", "additions", "deletions"}]}}`, from `gh pr view work-item/<id>` run in the project's folder; `canMerge` is true only when `state` is `OPEN` and gh calls it `MERGEABLE`; `{"pr": null}` when the branch has no PR | 404, 502 gh is not on `PATH` or failed, with its text |
| `POST /work-items/{id}/pr/merge` | no body needed | 200 the same body as `GET /work-items/{id}/pr`, after `gh pr merge <number> --merge` (a merge commit) | 404 no work item or no PR, 409 gh refused, with its text, 502 |
| `GET /skills?project={id}` | `project` is required | 200 `{"claude":[...],"codex":[...]}`, each a list of `{name, description}` read from disk, never stored. claude: `~/.claude/skills`, the project's `.claude/skills` and the skills of the plugins in `~/.claude/plugins/installed_plugins.json` installed for the user or for the project path, named `plugin:skill`. codex: `~/.agents/skills` and the project's `.agents/skills`. A name found twice is listed once; each list is ordered by name. A skill whose name is not letters, digits, `:`, `_` or `-`, or whose file cannot be read, is left out, and a malformed `installed_plugins.json` gives no plugin skills; each is logged and the rest are listed | 400 `project` missing or not a number, 404 |
| `GET /providers` | | 200 the detected `claude` and `codex` (those found on `PATH`), then the added ones oldest first, each `{name, kind, detected, folder, path, version, createdAt}`. `detected` is `true` for the ones on `PATH`, which have no `folder` or `createdAt`. `path` is the binary that would run (left out when an added install's binary is gone). `version` is the trimmed output of `path --version`, run for all of them at once with a 3 s limit, and left out when it fails or is late. Models and login state are not listed | |
| `POST /providers` | `name`, `kind`, `folder` | 201 the added provider, in the shape `GET` gives | 400 bad name, `kind` not `claude` or `codex`, `folder` not an absolute path to a folder, or no executable of that kind in it; 409 `name` is taken, or is `claude` or `codex` |
| `DELETE /providers/{name}` | | 204 | 400 `claude` or `codex` (the ones on `PATH` cannot be removed), 404, 409 a teammate still names it (the error names the teammate; change that teammate first) |
| `GET /teams` | | 200 all teams by name, each with its roles; a role has `mcp`, the name and label of each MCP server, not `mcpServers`, whose `args`, `env` and `url` may hold API keys | |
| `GET /teams/{name}` | | 200, with `mcp` instead of `mcpServers` as above; each role shows `write` | 404 |
| `PUT /teams/{name}` | the team JSON `reef team apply` takes (`name`, when given, must match the path; a role may also list `mcpRemove`, the `{name, label}` of each stored MCP server to drop, as `GET` showed it; servers it does not list are kept); applies it the same way, with the same validation. `?from=<source>` instead copies `source`: a role's body that omits `mcpServers` takes `source`'s role of the same name, and `name` must not exist yet, checked and inserted in one statement so two concurrent copies to the same name cannot both win | 200 the applied team, as `GET /teams/{name}` | 400 the same `invalid: ...` text `reef team apply` gives, or a body name that does not match the path; a role's body that omits `mcpServers` keeps the role's stored ones, minus those in its `mcpRemove`; 404 `?from=` names a team that does not exist; 409 `?from=` names a team that already exists, or a server in `mcpRemove` no longer matches a stored one by name and label (the team changed since it was loaded; nothing is applied) |
| `GET /teammates` | | 200 every teammate by name, each `{id, name, persona, skills, backend, model, provider, face, tools, mcp, write, permissionMode}` | |
| `POST /teammates` | the teammate fields above, `mcpServers` instead of `mcp` | 201 the teammate | 400 the same `invalid: ...` text a role gets, 409 name taken |
| `GET /teammates/{id}` | | 200 | 404 |
| `PUT /teammates/{id}` | every field; `mcpServers` left out keeps the stored ones, `[]` clears them | 200 | 400, 404, 409 name taken |
| `DELETE /teammates/{id}` | | 204 | 400 the teammate is in a team (names it), 404 |
| `POST /teams/{name}/members` | `teammateId`, `lead` | 201 the role as `GET /teams/{name}` shows it, with the teammate's default access copied | 400 unknown teammate, a second lead, or `solo`; 404 team; 409 already a member |
| `PUT /teams/{name}/members/{id}` | `lead`, `tools`, `mcpServers` (left out keeps the stored ones), `write`, `permissionMode` | 200 the role | 400 as `PUT /teams`, a second lead; 404 |
| `DELETE /teams/{name}/members/{id}` | | 204 | 400 `solo`; 404 |
| `GET /spaces` | `?project=ID`, `?workItem=ID` | 200 each Space also has `lastMessage`: `{author, body, createdAt}` of its newest message (a thread reply counts; `body` is cut to its first 200 characters), `null` when it has none | 400 bad id |
| `POST /spaces` | `projectId`, `workItemId`, `name`, `team` (a team name; default `solo`), `backend` (`claude` or `codex`; default the team lead's), `permissionMode` (`""`; claude: `default` or `acceptEdits`; codex: `read-only` or `workspace-write`; default the lead's when `backend` is the lead's), `role` (empty or `lead`; the Space's `role` becomes the name of the team's lead role), `teammateId` (instead of `team`: a Space with no Team and one member, that Teammate as lead, with its default access rules; `kind` `conversation` makes it a direct conversation, which cannot have a work item). A `name` is required; one that is the new-space form's default, `<backend or team> <hh:mm>` (`claude 14:32`, `claude 02:32 PM`), marks the Space as waiting for a provider name, see [Space names](#space-names) | 201 | 400 unknown project, work item or team, work item of another project, bad backend or permission mode, a role other than `lead` |
| `GET /spaces/{id}` | | 200 | 404 |
| `PUT /spaces/{id}` | `name` (required), `permissionMode`; project, work item, role and backend stay unchanged. An omitted permission mode stays unchanged. A set permission mode is validated for the Space's backend and applies the next time its agent starts. A name that differs from the stored one ends any wait for a provider name | 200 | 400 bad JSON or permission mode, 404 |
| `DELETE /spaces/{id}` | stops the Space first if it is live; a send during the delete is 404 | 204 | 404 |
| `POST /spaces/{id}/send` | `text`, `parent` (optional: the id of a top-level message of this Space; the message is stored as a reply in its thread) | 202, no body; while a message chain starts or runs the message is stored and its wakes [queue](#routing) behind the current turn | 400 empty text, an unknown field (`role` is not a field), a `parent` that is not a top-level message of this Space, or `/wrap` on a quick session, 404 (also while the Space is being deleted), 409 wrap or promote turn running, Space closed, Space paused (a hop cap pause does not count), open elsewhere or stopped before its turn began, 503 |
| `GET /spaces/{id}/events` | | 200 `text/event-stream` | 404 |
| `GET /spaces/{id}/inspector` | | 200 per role: `role`, `turns`, `usage` (summed), `costUsd` (summed, `null` when no turn of the role has a cost); per turn: `id`, `role`, `startedAt`, `status`, `model`, `usage`, `costUsd`, `prompt`, `reply`, `toolCalls` (`name`, `input`, `result`, and `children` when it is an `Agent` call: its subagent's calls, same shape, not repeated at top level) | 404 |
| `GET /spaces/{id}/direct-messages` | `?after=ID` (only newer ids) | 200 all direct requests and response attempts in the Space, oldest first; each has `id`, `spaceId`, `chainId`, `replyToId`, `sourceMessageId?`, `from`, `to`, `body`, `status` (`sent`, `done` or `failed`), `createdAt` | 400 bad id, 404 |
| `GET /search` | `?workItem=ID&q=TEXT` (both required), `limit=N` (default 20) | 200 hits across the work item's public messages and all its direct messages, newest first, `[]` when none. Each hit has `spaceId`, `messageId`, `kind` (`public` or `direct`), `author` and `snippet` (about 120 characters around the match, case ignored for ASCII); a direct hit also has `from`, `to` and `chainId`, and its `messageId` is the direct message id | 400 missing or bad `work item`, missing `q` |
| `POST /spaces/{id}/reopen` | no body needed | 204; clears `closedAt` | 404 |
| `POST /spaces/{id}/stop` | no body needed | 204, also when the Space is not live or does not exist | |
| `POST /spaces/{id}/continue` | no body needed | 204, no body | 404, 409 not paused or a turn running |
| `POST /spaces/{id}/wrap` | no body needed | 202, no body | 400 a quick session (no work item); 404, 409 and 503 as send |
| `POST /spaces/{id}/attach` | `workItemId` (required) | 200 the Space | 400 no `workItemId`, Space already has a work item or is a conversation, work item of another project, project not a git repo with a commit; 404; 409 a turn running, or the work item already has an active Space |
| `GET /spaces/{id}/members` | | 200 the Space's members, lead first: the role fields of a [team](#team) role | 404 |
| `POST /spaces/{id}/members` | `teammateId` (required) | 201 the member, a copy of the Teammate's default access rules | 400 unknown Teammate or a conversation, 404, 409 already a member or its name is taken |
| `GET /spaces/{id}/proposal` | | 200 the Space's pending proposal | 404 none pending |
| `POST /proposals/{id}/approve` | no body needed | 200 the proposal, `itemsAdded` and `warnings` (a line for each saved item that looks like a stored one or has no vector; left out when none); a wrap closes the proposal's Space and stops its agent, a brief proposal does neither | 400 already decided, 404 |
| `POST /proposals/{id}/reject` | no body needed | 200 the proposal | 400 already decided, 404 |
| `GET /messages` | `?space=ID` (required), `?after=ID` (only newer ids) | 200, each with `parentId` | 400 no Space, 404 unknown Space |
| `POST /messages` | `spaceId`, `parentId`, `author`, `body` | 201 | 400 unknown Space, missing field, or a `parentId` that is not a top-level message of that Space |
| `GET /turns` | `?space=ID`, `?status=running\|done\|failed`, `?retryable=1` (only the turns `retry` accepts), `?rollbackable=1` (needs `space`; only the turns with a `checkpoint` taken in the tree the Space runs in now, so `rollback` is not a 409 for the tree) | 200 | 400 bad id or status, or `rollbackable` without a Space, 404 `rollbackable` on an unknown Space |
| `POST /turns` | `spaceId`, `role`, `backend`, `sessionId`, `wakeText`, `wakeParent`, `directMessageId` | 201 | 400 bad backend or unknown Space, public message or direct message |
| `GET /turns/{id}` | | 200 | 404 |
| `PATCH /turns/{id}` | `status` (`done` or `failed`), `error`, `usage`, `sessionId`. With no `status`, only `sessionId` is set and the turn stays `running` | 200 | 400 bad status, negative usage, empty `sessionId` without a status, `error` or `usage` without a status, or already ended; 404 |
| `POST /turns/{id}/retry` | no body needed | 202, no body; the wake of the failed turn runs again as a new turn of its role, and the failed turn is `retried` | 404, 409 not retryable (see [Spaces live](#spaces-live)), or as send for a Space with a turn running or paused, 503 |
| `GET /inbox` | | 200 the items waiting on the user, newest first; `[]` when none. See [Inbox](#inbox) | |
| `GET /inbox/events` | | 200 `text/event-stream` | |
| `POST /spaces/{id}/dismiss` | `messageId` | 204, no body; the Space's `waiting` item for that message is hidden, see [Inbox](#inbox) | 400 bad body, 404 unknown Space or a message that is not the Space's |
| `POST /turns/{id}/rollback` | no body needed | 204, no body; the Space's worktree is as it was before the turn, and `reef` posts `Rolled back to before turn N.` | 400 the turn has no `checkpoint`, 404, 409 the Space has a turn running, a `reef chat` has it open, its tree is not the one the checkpoint was taken in, or a turn of any role runs on the tree, 503 |
| `GET /decisions` | `?workItem=ID` (required) | 200, retired decisions too, with `retiredAt` and `replacedBy`. Unused since decision 91 | 400 no work item, 404 unknown work item |
| `POST /decisions` | `workItemId`, `messageId`, `body`, `why`, `author`, `replaces` | 201. Unused since decision 91 | 400 missing field, unknown work item, a message of another work item, or a `replaces` id that is not a live decision of the work item |
| `GET /decisions/{id}` | | 200. Unused since decision 91 | 404 |
| `GET /memory` | `?scope=KIND:ID` (required, such as `work_item:3` or `project:1`) | 200 the accepted items of that scope, the most recently accepted first, as objects (`id`, `kind`, `body`, `why`, `scope`, `author`, `role`, `status`, `retiredAt`, `replacedBy`, `createdAt`; no `links`); `[]` when none | 400 no scope or one that is not `kind:id` |
| `POST /memory` | `kind`, `body`, `why`, `scope` (`KIND:ID`) | 201 the item as for `GET /memory`, written with author and role `user`, so always accepted, plus `similar` (`[{id, body}]`, stored items that look like it) and `embedError` (why no vector was stored), each left out when empty | 400 unknown kind or scope kind, empty body, bad scope |
| `POST /memory/{id}/retire` | no body needed | 204, no body; sets `retiredAt` | 400 not an accepted item (already retired, a candidate), 404 |

A `POST` with no body (`stop`, `continue`, `wrap`, `approve`, `reject`) still needs
`Content-Type: application/json`, or the guard answers 415. These routes ignore the body,
so an empty one or `{}` both work. `send` and `attach` read a JSON body, and an empty one
is 400.

Create requests take the whole object from the table above. A field you leave out takes
its default. Other `PUT` requests replace the whole object. Update behavior is
route-specific: `PUT /spaces/{id}` keeps an omitted `permissionMode`, while
`PATCH /turns/{id}` is the only partial update for other resources.

## Spaces live

`POST /spaces/{id}/send` runs the Space's agents through
[`space.Open`](../space/session.go), the code `reef chat` uses. A Space has a
[team](#team): the lead answers the user, and the other roles are specialists that run when
a message wakes them.

- The server stores the user message, then answers 202. The turns run in the background, not
  in the request. The text `/wrap` runs the wrap-up turn (400 on a Space with no work item, before any
  agent turn). Every other text is a plain message. A send with `parent` stores the message as a reply in that
  thread.
- Each role of the team has its own agent session, its own `reef mcp` token, its own turns and
  its own session id. The role's backend, permission mode, tools and MCP servers are set up
  for it (the lead uses the Space's backend and permission mode). Its first prompt starts with `## You`,
  then its persona and skills (see [memory.md](memory.md#the-context-prompt)).
- An agent starts when its first turn runs, not when the Space opens. A first message that
  wakes a specialist starts no lead. A role that already has a session resumes it by its own
  newest session id (see [memory.md](memory.md#reef-chat)). An agent that fails to start, for any
  reason (the agent, its session lookup, its token, its folder or its first prompt), shows
  as one `failed` turn of its role, with the start error, or `interrupted` when a stop cut the
  start short. So does a wake whose text cannot be built. A wake for a role the team no longer has is a `failed`
  turn too, with the wake text, on the Space's backend. The wake text of a wake that never started is the
  text the wake was routed with, not the messages it would have sent. The server logs every failed turn. When the backend says the session is
  missing (claude `No conversation found`, codex `no rollout found for thread id`), that turn
  fails and the role's next wake starts a new session, with its intro, not the dead one.
- An agent of a Space under a work item runs in the work item's worktree,
  `<data folder>/worktrees/<project id>/<work item id>`, on branch `work-item/<work item id>`. A
  sub-item has none of its own: its worktree, branch and PR are its parent's, so the diff and
  PR routes of a sub-item answer for the parent, and deleting a sub-item keeps the tree. It is made at
  the first start of any agent of the work item, from the project's base branch (fetched from
  `origin` first, a failed fetch is only logged; `origin/<branch>` if it exists, else the local
  branch) or the repo's default branch when none is set, with `git worktree add`; a worktree or branch that exists is reused, so every Space of the work item shares
  one tree. A branch that is checked out elsewhere (in the main checkout, or in the worktree
  of another project on the same repo) cannot be added: the start fails with `branch
  work-item/<id> is checked out at <path>; switch that checkout to another branch`. A
  quick session runs in the project path. The work item is read at each agent
  start, so an attached Space moves to the worktree at its next wake. A project that is not
  a git repo, or a repo with no commit, has no worktree: the start fails as a `failed` turn
  whose error is `work items need the project folder to be a git repo with a commit: <path>:`
  and git's error, and the server goes on. A worktree whose folder was deleted is pruned and
  made again at the next wake: an agent running in the missing folder is closed, and the
  next one resumes the session in the new worktree.
- Before each turn of a role with `write` in a git checkout (a work item's worktree, or the
  project path of a quick session that is a git repo), the server stores a checkpoint: a commit
  of HEAD plus the whole working tree, untracked files included and ignored ones left out, at
  `refs/reef/checkpoints/<data folder>/<turn id>`, where `<data folder>` is the first 12 hex
  digits of the sha256 of the absolute data folder path, so two Reef copies on one repo never
  overwrite each other's. Turns from before keep their stored ref. It is built in a temporary index that starts as a copy of
  the real one, so the index, HEAD and the files do not change. The ref is on the turn's
  `checkpoint`. If the checkpoint fails, the turn is stored `failed` with git's error and the
  agent gets no prompt. A repo with no commit has nothing to snapshot: its turns run with
  `checkpoint` null. A role without `write` gets none, unless it is a writer in all but name
  (below). The checkpoint commit has two parents:
  HEAD, then a commit of the index as it was, so a rollback can bring back what was staged.
  Its message records the directory and the branch it was taken in (`dir:` and `branch:`
  lines; no branch when HEAD was detached).
- One write turn at a time runs on a tree, across all Spaces and processes on the data
  folder: the work item's worktree, or the project path for quick sessions. A wake of a role with `write` waits for
  the write turn that runs on its tree, then runs. Its turn row is created when it starts, so
  `startedAt` is after the other turn's `endedAt`. A stop ends the wait, and the turn is
  stored `failed` as `interrupted`. A claude role without `write` whose tools allow a shell
  command outside a short list of reading commands (`git status`, `ls`, `cat`, `head`,
  `tail`, `grep`, `wc`, `pwd`) counts
  as a writer: it takes the lock and gets a checkpoint, as `Bash(git commit:*)` or `Bash(rm:*)`
  could change files. `rg` is not a reading command: `rg --pre` runs a program. Neither are
  `git log`, `git diff`, `git show` and `git blame`: `--output=<file>` makes them write a file. Only the
  tools the role keeps count: a plain `Bash` or `Bash(*)` is dropped from a role without `write`,
  so it does not make the role a writer. A role without `write` and with only reading commands is not held: it runs beside a
  write turn, and counts as a reader of the tree for its turn, which only a rollback
  respects. The first start
  of a work item's worktree takes a lock of its own, so two Spaces waking at once add it once.
  The locks are `flock` files in `<data folder>/locks/`, named from a hash of the tree path
  (`.lock` for the writer, `.read` for readers), so `reef serve` and `reef chat` share them.
  A waiting write turn polls its lock every 20 ms. A rollback takes the writer lock without
  waiting and checks the reader file, so a read-only turn of any process refuses it.
- `POST /turns/{id}/rollback` puts the Space's tree (the work item's worktree, or the project
  path of a quick session) back as the turn's checkpoint saw it: the branch the checkpoint was
  taken on is checked out at the HEAD it had, so commits made since, and a branch the turn
  switched to, are left behind (a detached checkpoint resets whatever is checked out); tracked
  and untracked files get the content they had, the same changes are staged, and files made
  since are removed. Ignored files are left alone. It claims the Space like a send. It is
  409, never a wait, when the Space's tree is not the directory the checkpoint was taken in
  (a free Space's turn after an attach), when a `reef chat` holds the Space, or when a
  turn of any role of any Space runs on the tree (the write lock is held, or a read-only
  turn is a reader). A reader that starts during the restore is not held back. A turn with
  no `checkpoint` is 400. Then `reef` posts `Rolled back
  to before turn N.` in the Space. The agent's session is not rewound. When a turn of
  a role that has run before in the Space is sent, its prompt gets the lines `reef wrote: ...`
  that `reef` posted since that role's last turn began, after the header line, so the role
  learns of the rollback (and of a restart failure note) even when its wake is a retry; a role
  that never ran gets none. The wake kept as `wakeText` does not have them.
- `DELETE /work-items/{id}` cleans what Reef made for the work item. It stops the agents of the
  work item's Spaces first (a running turn is stopped, as when a Space is deleted). A
  worktree with uncommitted changes (untracked files count) is 409 with `work item <id> has
  uncommitted changes in <path>; commit or discard them first`, and nothing is removed.
  Otherwise the worktree is removed with `git worktree remove`, the work item rows are
  deleted, and last the `refs/reef/checkpoints/<data folder>/<turn id>` refs of every turn of its Spaces
  are deleted. The worktree goes first because it is the step that can refuse or fail, and a
  later wake of a work item makes it again; the refs go after the rows that name them, so a
  failure there leaves refs that no turn points to, and the delete is 500. The branch
  `work-item/<id>` is kept: it is the user's work. A work item with no worktree folder (no agent
  ever started) only has its rows deleted. Deleting a Space keeps worktrees and refs.
  Right after the rows go, the memory items of the work item's `work_item:<id>` scope are
  closed: pending candidates are rejected and accepted items retired, so nothing of it is left
  in `reef memory pending`. Retired and rejected items are kept and still read.
- `DELETE /projects/{id}` does the same for every work item of the project, and deletes the
  checkpoint refs of the turns of every Space of the project, quick sessions included (their
  refs are taken in the project folder). It checks every worktree first: one with uncommitted
  changes is 409 for that work item, and nothing is removed. Then it removes all worktrees,
  deletes the project rows, and last deletes the refs, in the same order and with the same
  failure rule as a work item delete. Branches are kept. Its memory items are closed the same
  way, in the project's `project:<id>` scope and in the `work_item:<id>` scope of each of its
  work items.
- The lead stays alive between turns, until `POST /spaces/{id}/stop` or the server stops. A
  specialist's agent is stopped, and its token deleted, when its turn ends. Its next wake
  resumes its session. A background task a specialist started ends with its agent, and its
  answer is not posted. While no turn runs, the lead's events are read, so the answer of its
  background task is posted as its message when it arrives (decision 84).
- The server holds `chat-<id>.lock` while the Space has a live session, so `reef chat
  --space ID` fails with `space 1 is open elsewhere: conflict`. A send to a Space a
  `reef chat` holds is 409 too.
- `stop` interrupts a running turn (stored `failed`, `interrupted`), drops the wakes still
  waiting (so does a turn that fails: its chain ends and the wakes behind it do not run; the
  next send starts a chain that posts a message from reef naming the roles of those wakes, and
  a restart before that send holds them instead), then stops every agent of the Space, deletes their tokens and releases the lock.
  It also works while the send is still checking the caps or starting an agent: the send then
  answers 409 `space 1 was stopped`, and no turn runs. Once the send has changed something
  or stored the user message, it does not give up: it answers 202 as
  usual, and its turn runs with the stop already in effect, so it is stored `failed`,
  `interrupted` and the agent gets no prompt. A turn that fails for another reason also stops
  every agent of the Space; the next turn opens them again, each by its own session id.
- `DELETE /spaces/{id}` does the same as `stop` before it deletes the Space.
  `DELETE /work-items/{id}` and `DELETE /projects/{id}` do it for each of their Spaces.
  From the start of that stop until the delete is done, a send to the Space is 404
  `space 1 is being deleted`, so no agent starts for a Space that is going away.
- Server shutdown does the same for every Space before it stops listening.
- A turn keeps the wake it ran as `wakeText`, `wakeParent` and `directMessageId`. `POST /turns/{id}/retry` on a
  `failed` turn runs that wake again for the same role as a new turn in a new chain, the way an
  approve does, with no new user message: the new turn answers in the same public thread or
  direct exchange, and what its
  reply wakes is routed as usual. The failed turn is marked `retried` before the turn starts, so a
  second retry is 409; if the turn cannot start, the mark is taken back. Nothing retries a
  turn by itself. A turn is retryable when it is `failed`, not `retried`, has a `wakeText`, and
  no later turn of its role ran in the Space: a turn that a later one supersedes is 409. A
  turn the user stopped (`interrupted`) is retryable, since the user may want its wake run
  again. A routed wake that never sent is built again from the
  messages not yet sent, so a retry never resends a whole thread; a turn that sent runs its kept text again. A wrap-up turn keeps no wake, so it is never retryable: the user wraps
  again. `GET /turns?retryable=1` lists the same turns, which is what the window shows
  as failed turns with a Retry button. The retry counts as a message of the user for the hop cap.

### Routing

When a turn ends, the server reads the messages stored since the last one it read and routes
those written by the role of the turn that ended: its reply, which is stored as a message from
the role, and any message it posted with the `post_to_space` tool (see
[memory.md](memory.md#reef-mcp)). A turn that posted with `post_to_space` has one message: its
plain reply at the end of the turn is not stored as a message, so it wakes no one a second time.
The turn keeps it (`reply`, shown by the inspector). A post counts when its tool call did not
fail. A turn that did not post, a failed or stopped turn, and a direct turn (its reply is the
direct response) store the reply as before. Other messages, such as a `POST /messages` from the user in
the middle of a chain, are skipped: they start no turn and do not reset the hop count. Each
routed message goes through the router, [`space.Route`](../space/route.go), plain code with
no model. In order:

1. A reply from the user in a thread goes to the thread's owner, the author of the root. When
   the user owns the thread, it goes to the lead. An agent's reply in a thread has no owner
   rule: it wakes only the roles it mentions.
2. Otherwise every `@name` that is a role of the team goes to that role, each once, when it
   is a handoff: it starts the body or a line (spaces and tabs may come before it), or it
   follows other `@name`s at the start with only spaces or commas between them, as in
   `@architect, @tester do X`. A name later in a sentence wakes no one: `I'll report to @lead
   later` and the second name of `@architect and @tester do X` are not handoffs. The sender
   and unknown names are skipped. The message is stored once. When it wakes more than one
   role, each wake's header is followed by `Asked together: @a, @b` naming all of them.
3. Otherwise a message from the user goes to the lead.
4. Otherwise nobody: a message from an agent that hands off to no one wakes no one, in a thread or not.

The lead is the Space's `role`: on a Space whose role is `developer`, `@lead` names no
role, so the message goes to `developer` and no `lead` session starts.

A role never wakes itself. The user's own message is routed by the same rules, so a send
with `parent` on an architect's thread goes to the architect, and `@architect ...` goes to the
architect with no lead turn. `POST /messages` stores a message and wakes nobody. The `/wrap`
turn is not routed (see below).

A wake is a turn of the woken role. Routing gives it the text it was routed with: a line
`[Space 3, message 12]` for a top-level message, or `[Space 3, message 12, thread 9]` for a
reply (the Space, the waking message and, for a reply, its thread root), then the user's
text as it is, or `<author> wrote: <body>` for an agent's message. That text is what the wake keeps if it never starts. The wake also carries its scopes, the threads
(0 for the top level) of the messages it was routed with, as data from routing, never read back
from message text, so a message that quotes a header changes nothing. The turn's own text is built
when the turn starts, not when the message is routed, from what the role has not been sent in its session:
the messages of the thread (or, for a top-level message, the Space's top level) that no
earlier turn of the role in that session sent, oldest first, and never the role's own
messages or reef's. It starts with a line naming the newest of them, in the same form. When
there is more than one, all but the newest come after `New in the thread:` (or `New in the
Space:`) as `<author>: <body>`, and the newest follows as the user's text or `<author>
wrote: <body>`. A message stored after the wake was routed, before it runs, is in the text.
A turn keeps what it sent for each scope (`turn.scopes`, the newest message of each): the role's
later turns in the same session skip every message up to it, so a thread is sent once, and a
new session, after a lost one, gets the thread again once. A turn from before this column
kept nothing, so the first wake after an upgrade sends each thread once more. A scope with
nothing read, a new session's first wake, shows only its last 20 unread messages, with a line
saying the older ones are in `read_thread`. A wake that finds nothing unread runs no turn and starts no agent: an earlier turn
already sent what woke it. One routing pass wakes a role once: when several messages,
top-level or in any thread, wake the same role, for example two `post_to_space` calls of
one turn that name `@architect`, they are one wake, and its text has one block for each thread, with its `Asked together` line. The woken
role's reply is stored in the thread the header of the first message names (`parentId` is the
thread root). When a top-level message woke the role, the reply is top-level too: it is the
root of a thread the role owns, and the user's reply in it goes to the role.

The first turn and the wakes after it are one chain. They run one at a time, in the order
the messages were stored, and the messages each turn stores are routed in turn and queued at
the end. The Space has a turn running until the chain is done, so a `wrap`, an `attach`, a
`continue`, a `rollback` or a `close` during the chain is 409. A `send` during a message chain
is accepted: the message is stored, and its wakes join the end of the chain's queue and run after the current turn and the wakes it
queued. A `send` that finds a message chain starting waits until it runs, so the message is stored
after the one the chain starts with and ids follow the order of the sends; if the chain never
starts, the send starts a chain of its own. A `send` while a wrap, an attach, a `continue` or a
`rollback` starts is 409. Turns still run one at a time. If the chain ends first (a failed turn, a stop, a
routing failure), a queued message's wakes are held, not run. When the hop cap or the token
budget stops the next wake, it is held and `continue` runs it (see [Caps](#caps)). A turn that fails ends the chain; the wakes still waiting are dropped. An example chain: the user asks the lead; the
lead's reply says `@architect ...`; the architect runs and posts a plan, a new thread it
owns; the chain ends because the plan names no one. The lead gets no turn after the plan.

`send_to_role` stores a direct request outside the public `message` table. When its sender's
turn ends, only the named role wakes. Reaching the
hop or token cap pauses the chain with the held direct wakes. Direct and public wakes share
the Space's hop count, so switching between them never resets it. Its prompt starts `[Space N, direct message N, chain
N, source: direct request]`, names the sender, and carries the body. The recipient's ordinary
reply is one direct response in the same chain (`replyToId` points to the request) and wakes
the sender with `source: direct response`. The sender's ordinary reply to that response is
not stored or routed. A further exchange requires an explicit `send_to_role`. The user reads
all direct records with `GET /spaces/{id}/direct-messages`; an MCP token reads only records
whose `from` or `to` is its role. `GET /search` finds public and direct messages together for
the user; the MCP `search_sessions` returns a direct hit only to its sender or recipient.
A stop that sheds a queued direct wake, and a user
message that drops a held direct wake after a hop cap pause, each give the request a `failed`
response with body `stopped` or `dropped`, so the window tells them from a delivery that failed.
A request its sender sent in a turn that ends stopped or failed, or that a restart cuts, gets a `failed`
response with body `stopped` or `failed` (a restart gives `failed`). Wrap-up prompts list the
Space's direct messages, marking a `failed` attempt `(failed)`. Partial output from a failed recipient turn is appended as
a `failed` response attempt: it is inspectable but never routed. Retry appends a `done`
response; a partial unique index allows only one `done` response per request, and only that
response wakes the sender. `sourceMessageId` is the public message whose wake the sender was answering when it called
`send_to_role`, read from the sender's running turn; a request sent from a direct wake keeps the
source of that wake, and a request sent from a turn with no public wake has `null`. `send_to_role` is
refused with `invalid:` unless its sender has a running turn. A response keeps the source of
its request. Public `GET /messages` history is unchanged.

### Space names

A Space created with the new-space form's default name has `space.name_auto` set to 1.
The first user message sent to it (`POST /spaces/{id}/send`, not a wrap command)
takes the one naming attempt (`name_auto` 2) and asks the Space's backend, in the background, for a name: one
throwaway turn in one shared empty folder (`reef-name` in the temp folder), with no tools (claude on `haiku`, codex
`read-only`), so it makes no turn or message. The reply is cut to the convention: its first
line, lowercase words of letters and digits (a hyphen splits words), at most four words and 40 characters
(`fix login redirect loop`). The name is stored only if the attempt is still the Space's, in one
statement, and `name_auto` goes to 0; a `PUT /spaces/{id}` with another name, which sets
`name_auto` to 0, got there first and wins. A failed call, an empty name or a stopped server keeps the
current name and is not tried again. Spaces that existed before the column keep `name_auto` 0.

### Events

`GET /spaces/{id}/events` is a Server-Sent Events stream. It works for a Space that
is not live: it waits. A `: keepalive` comment comes every 15 seconds. Seven kinds of event:

- `turn`: `{"role":"architect","status":"running"}` when a turn of that role starts, then
  `{"role":"architect","status":"done"}` or `{"role":"architect","status":"failed","error":"interrupted"}`
  when it ends. A chain of wakes sends one pair for each turn, with no `idle` between them. A
  wrap turn in which the agent made no proposal ends `{"role":"lead","status":"done","note":"no
  proposal made"}`. `{"status":"idle"}` has no role. It is sent only when a subscriber
  connects and no turn runs. A turn woken by a direct message carries `"direct":true` in each of its frames; the Space shows such a turn
  as working only, without its text or tool calls; the open Coordination panel of its exchange shows its text and tool rows live while it runs, and only the stored direct messages once it ends.
- `agent`: one [agent event](agents.md) as JSON, as it comes, from the turn that is running.
- `pause`: `{"reason":"..."}` when a cap pauses the Space: when a send finds the
  token budget used, or when the hop cap holds a wake. `{"reason":""}` when `continue`, or a user message after a hop cap pause, clears the pause.
- `note`: `{}`, sent when a message is posted outside a turn, such as a [GitHub
  poll](#github-poll) note, so a subscriber knows to re-read `GET /messages`.
- `name`: `{}`, sent when the provider's name replaces a Space's default name, so a subscriber knows to re-read `GET /spaces`.
- `direct`: `{}`, sent when a chain ends without running a queued direct wake and gives it a
  `failed` response after the last `turn` event, so a subscriber knows to re-read `GET /spaces/{id}/direct-messages`. Every
  other change to a direct message happens before a `turn` event that ends its turn.

A subscriber is brought up to date when it connects, so a reconnect needs no other
request to know the state. The server keeps the events of the running chain in memory. A
subscriber that connects during a chain first gets `turn` running and every `agent` event
of the chain so far, then the live ones. One that connects between chains first gets
`turn` idle. Either gets `pause` with the reason when the Space is paused. It does not get
The stored messages
(`GET /messages`) are the history. A slow subscriber only lags; it never holds
up the agent. When the server stops, a subscriber gets the last frames before the
stream ends.

```
event: turn
data: {"role":"lead","status":"running"}

event: agent
data: {"kind":"tool_call","toolId":"t1","toolName":"Bash","input":{"command":"ls"}}

event: agent
data: {"kind":"text","text":"@architect design the form"}

event: turn
data: {"role":"lead","status":"done"}
```

### Inbox

`GET /inbox` lists what waits on the user in every Space. It is read from the rows of
Spaces, turns and messages, so it has no table and a restart keeps it. Each
item has `kind`, `spaceId`, `workItemId` (null for a quick session), `text` and `at`, and the
id to act with:

| `kind` | Listed while | `text` | Also has | Cleared by |
| --- | --- | --- | --- | --- |
| `paused` | the Space's `paused` is not empty | the reason: hop cap, token budget or a routing or store failure | | `continue`, or a user message after a hop cap pause |
| `failed` | a turn is retryable (see [Spaces live](#spaces-live)) and its error is not `interrupted`: a turn the user stopped is a decision, not an item | `architect failed: server restarted` | `turnId` | `retry`, or any later turn of that role, so of several failed turns only the newest shows |
| `waiting` | the Space's newest message is from an agent (not `user` or `reef`) and woke nobody: no turn running, no other item for the Space, and the Space's newest turn was not stopped (the part of a reply a stop kept is not waiting) | the first 200 characters of that message | `messageId` | any new message, such as a send; or `POST /spaces/{id}/dismiss` with the item's `messageId`, which hides it until a newer message arrives |

`at` is the time of the failed turn's end, of the message, and for `paused`
the end of the Space's newest turn. A Space that has a chain running lists no `waiting`,
`paused` or `failed` item, since the chain is not over and `continue` or `retry` would be 409. A hold of a pause is not an item of its own: its Space is `paused`.

`GET /inbox/events` is one Server-Sent Events stream for all Spaces, with a
`: keepalive` every 15 seconds. Its only event is `inbox`, whose data is the whole list as
`GET /inbox` returns it. The first one comes when the stream connects; after that one comes each
time the list differs from the last one sent. The list is read again after every event of every
Space (turn, agent, pause), when a chain ends, and after every request that is not a
`GET`, since a delete or a `POST /messages` can change it with no Space event. A change
a `reef` process makes straight in the database shows at the next keepalive tick at the latest.
When the server stops, a stream sends the last list and ends.

```
event: inbox
data: [{"kind":"waiting","spaceId":5,"workItemId":2,"text":"Plan: add the form and a test.","at":"2026-10-14T09:00:10.123456Z","messageId":28}]
```

## Wrap-up and attach

`POST /spaces/{id}/wrap` does what a send of the text `/wrap` does: it runs the wrap-up
turn of [`Session.Wrap`](../space/session.go) (see [memory.md](memory.md#wrap-up)). The
agent calls `propose_wrap`; when the turn is done, `GET /spaces/{id}/proposal` returns the
pending proposal, or 404 when the agent made none. `POST /proposals/{id}/approve` and
`/reject` decide it with the same effect as `reef wrap approve` and `reject`. Approve
saves the proposal's items, retires its `retire` ids, closes the Space (`closedAt`) and stops its agent, and returns `itemsAdded`, the items it saved (one already saved in its scope with the same body is skipped; each carries a `source` link to the proposal), and `warnings`. Two approvals of one proposal at the same moment run one after the other: the second is `400 already approved`. Saving the items comes first and is not part of the transaction that saves the brief and closes the Space: when it fails, the proposal stays pending and the Space open, and approving again finishes without saving an item twice; a closed Space refuses sends, wraps and wakes with 409 until `POST /spaces/{id}/reopen`. A brief proposal (`propose_brief`, `kind` `brief`) does not close the Space or block its turns or a wrap; only a
pending wrap blocks another wrap (409 `space 2 has a wrap-up waiting for approval`). A proposal that is already decided is
400, like `invalid: proposal 1 is already approved` in the CLI.

### Attach a Space to a work item

`POST /spaces/{id}/attach` gives a free Space a work item: `{"workItemId": 3}`, a work item
of the Space's project. A Space that has a work item, and a direct conversation, are 400.
The Space must be idle (409 while a turn runs), the project a git repo with a commit, and
the work item without another active Space (409 `work item 3 already has an active Space`);
a refused attach changes nothing. The attach sets the Space's `workItemId` and `kind`, moves
its `space` memory items to the `work_item` scope of the work item, stops the Space's agents,
so the next wake of each role resumes its session in the work item's worktree, and posts a
message from `reef`: `This Space is now attached to work item "<title>" (<id>). It works in
<worktree path> on work-item/<id>.`, with ` Uncommitted changes stay on the main checkout.`
after it when the main checkout has uncommitted changes. No agent turn runs.

## Caps

Two caps pause a Space. All are constants in the code, not settings.

- Tokens per Space: the input, output and cache write tokens of all the Space's turns,
  of every role, cache reads not counted, against `tokenBudget` (1,000,000). A running turn
  counts the usage it has streamed so far. A send, or a wake in a chain, that finds the sum at or
  over the budget pauses the Space with `token budget of 1000000 used: 1000312 tokens`. The budget is
  checked only between turns: a turn that crosses it runs to its end.
- Agent hops: a hop is a turn that an agent's message woke. The turn that answers the user's
  message is not a hop. The server counts hops per Space since the last user message. The
  7th hop does not run: the Space is paused with `hop cap: 6 agent hops without a user
  message`, and the `turn` event of the turn before it is followed by a `pause` event. The wake
  that did not run, and the wakes behind it, are stored as holds.

A send to a paused Space is 409 with the reason in the error, except after the hop cap: that
pause is for agents, so a user message clears it, resets the hop count to 0, drops the held
wakes and runs. `POST /spaces/{id}/continue` clears the pause. When the token sum is at or
over the budget it also sets `tokenBudget` to the sum plus 1,000,000. `continue` on a Space that is not paused, or has a turn running,
is 409.

Held wakes: when a wake cannot run because of a pause (the hop cap, or a token budget that ran
out in the middle of a chain), the server stores it and the wakes behind it as `hold`
rows of the `approval` table. `continue` claims the Space's pending holds once, resets the
hop count to 0 and runs them in order as a new chain, with `pause` `{"reason":""}` as the
chain starts. If the chain cannot start, the Space stays paused and keeps the holds. That
chain can reach the hop cap again, and then pauses again. Sending a user message resets the
hop count to 0 too, and drops the holds; a dropped direct request gets a `failed` response with
body `dropped`. The holds are stored, so
they survive a restart; the hop count is not, so a chain that runs after a restart counts from 0.

The same pause holds work when the server cannot
read the messages or the direct messages a turn stored to route them, or when a store error
stops the check before the next wake. The Space pauses with the error as the reason. These wakes are held in memory only, since the database failed, until the server stops and stores them. `continue` does the routing read again from
the message and the direct message before the turn's reply, and runs what that wakes. If it fails again the Space
stays paused.

No role has a tool call cap: a turn runs until the agent ends it, the user stops it, or the
token budget is used. Before each line, `reef chat` runs the same pause and token budget check as send (`space.CheckCaps`)
and prints the reason instead of running the line. The line `/continue` does what
`continue` does (`space.Continue`). See [memory.md](memory.md#reef-chat). Hops do not
exist in `reef chat`: it wakes nobody.

## GitHub poll

`reef serve` polls GitHub for each top-level work item that has a worktree, every `REEF_GH_INTERVAL`
(default 60s), once right away and then on that interval until the server closes: `gh pr
view work-item/<id> --json number,title,url,state,isDraft,mergeable,reviewDecision,headRefName,baseRefName,statusCheckRollup,body,additions,deletions,files`, run in the project's
folder. `statusCheckRollup` items are either a `CheckRun` (`name`, `status`, `conclusion`,
`detailsUrl`, `completedAt`) or a `StatusContext` (`context`, `state`, `targetUrl`,
`startedAt`); both are read.

When the PR first appears, changes state (`open`, `merged`, `closed`) or a check run
finishes (a `CheckRun` whose `status` is `COMPLETED`, or a `StatusContext` whose `state` is
not `PENDING`/`EXPECTED`), the work item's most recent Space (the one with the newest
message across the work item, or, when it has none, its most recently created Space) gets
one message from author `reef`: `PR #12 open: <url>` or `<check name> succeeded`/`failed`.
A check run is identified by its name plus its `detailsUrl`/`targetUrl` (falling back to
`completedAt`/`startedAt`) as its run key; a rerun of the same check name, such as CI run
again on a later push, is a different run key and is noted again, and two runs of the same
name in one rollup (a matrix job, two workflows) are two rows, both noted. The state posted
is stored one row per `(work item, name, run key)` (migration 16
[`016_work_item_github.sql`](../store/migrations/016_work_item_github.sql), tables
`work_item_github` and `work_item_github_check`), so a restart does not repeat it and a second
poll with the same answer posts nothing. No `gh` on `PATH`: nothing is posted, logged once.
`gh` erroring (not logged in, no PR for the branch): nothing is posted, logged once per
work item for that cause; logged again once the cause changes. A work item with a worktree but no
Space yet has nowhere to post: the note is recorded as sent only once it is actually posted,
so it is not lost and posts once the first Space exists.

Every failed run in a rollup (a `CheckRun` whose `conclusion` is `FAILURE`, `ERROR`,
`TIMED_OUT` or `STARTUP_FAILURE`, or a `StatusContext` whose `state` is anything but
`SUCCESS`/`PENDING`/`EXPECTED`) also wakes the `developer` role in that Space, once per run,
also across restarts (`work_item_github_check.woken`): several failed runs in the same rollup,
not yet woken, are one wake, not one each, its text naming every one of them plus the last 50
lines of `gh run view <run id> --log-failed` for each run whose id can be read from the
`CheckRun`'s `detailsUrl` (`/actions/runs/<id>/`); no run id, no log for that run. A Space
whose team has no `developer` role gets the note only. The wake text, including any log
fetch, is built before the Space is claimed, so a user send that arrives during the fetch
finds the Space still idle and is not refused. The wake runs as its own chain, the way a
retried turn does (decision 39), attributed to author `reef` rather than `user`: it counts as a hop like any agent-originated wake, so a run of
failing checks still pauses at the hop cap instead of resetting the count every time; the
Space's other caps (pause, token budget) also still apply. A Space with a turn running, or
paused (including by the hop cap), cannot take the wake on that poll; the note still posts,
the wake is retried on a later poll once the Space is idle, and the busy cause is logged
once, not every poll.

## Append-only

The API has no way to change or remove these rows.

- Messages and decisions: create and read only. `PUT`, `PATCH` and `DELETE` on
  `/messages/{id}` and `/decisions/{id}` return 405. The only way a message or decision
  goes away is a cascade from its Space or work item. Deleting a Space does not delete
  its decisions; they only lose `messageId`. A decision's body is never edited, but it can
  be retired (`retiredAt`, `replacedBy`) by a new decision that replaces it or by an
  approved proposal; see decision 76.
- Briefs: `PUT /work-items/{id}/brief` adds a version and never edits one. Old versions
  stay in `/brief/versions`. Role notes are not versioned; `PUT` replaces them.
- Turns: created `running`, may get a `sessionId` while running, ended once with `PATCH`
  to `done` or `failed`. After that every `PATCH` returns 400. A turn has no delete route
  and no other edit, except that a `failed` turn is marked `retried` once, by `retry`. Restart
  recovery keeps a session id set this way.

## Restart recovery

A turn is `running` only while a server owns it. If the server stops, the agent process
dies with it, so a `running` turn in the file can no longer finish. At start, once it
holds the data folder and has bound `--addr` but before it serves requests, `reef serve`
fails them all, except the turns of a live `reef chat`. A second `reef serve` never gets
this far on a folder in use, so it cannot fail the turns of the server that owns it:

- Each `running` turn becomes `failed`, with `error` `server restarted` and `endedAt` set.
- Each affected Space gets one message from author `reef`, naming the turns. For turns 2
  (developer) and 3 (tester) in one Space:

  `Turn 2 by developer, turn 3 by tester failed: server restarted. The worktree may be dirty; retry or roll back.`

- With one turn: `Turn 2 by developer failed: server restarted. The worktree may be dirty; retry or roll back.`
- The server logs `marked N running turns failed: server restarted`.
- A `running` turn in a Space whose `chat-<space id>.lock` a live `reef chat` holds is
  left `running`, and that Space gets no message from `reef`. `reef chat` is a separate
  process, so a server restart does not kill its agent, and its turn can still finish.
  The count in the log does not include it. A chat that died leaves its lock free, so
  its turn is failed like any other.

An agent may have left half-done edits in the worktree, so the message tells the user to
check before retrying. A turn failed this way keeps its `wakeText`, so `POST /turns/{id}/retry`
can run it again (see [Spaces live](#spaces-live)).

A chain of wakes does not run on after a restart, but its waiting wakes are kept. When the
server stops, the wakes a chain had queued behind the running turn, and the wakes held in memory
because the database failed, are stored as holds, and the Space is paused with `app restarted:
wakes were waiting to run` unless it is paused already. The pause shows in the inbox, and
`continue` runs the holds. A routing read that failed before the stop is done again then, and what it
wakes is held too; if it fails again, the Space gets a message from `reef` saying which messages woke
nobody. Only the hop count is lost: a chain that runs after a restart counts
from 0. A pause is stored with its holds, so both survive too.
A direct request that did not run still gets its `failed` response.

## Try it

Start a fresh server, then run these in order. Ids and timestamps follow from an empty
data folder; your timestamps will differ.

```
reef serve --data /tmp/reef-demo
```

```
$ curl -s -X POST -H 'Content-Type: application/json' localhost:7700/projects -d '{"name":"shop","path":"/tmp"}'
{"id":1,"name":"shop","path":"/tmp","baseBranch":"","createdAt":"2026-10-13T09:00:00.123456Z"}

$ curl -s -X POST -H 'Content-Type: application/json' localhost:7700/work-items -d '{"projectId":1,"type":"bug","title":"Login fails","acceptanceLines":[{"text":"no 500"}]}'
{"id":1,"projectId":1,"parentId":null,"type":"bug","title":"Login fails","description":"","status":"backlog","teamId":null,"proposedBy":"","suggestedTeamId":null,"queuePosition":null,"tokenBudget":1000000,"createdAt":"2026-10-13T09:00:01.123456Z","closedAt":null,"acceptanceLines":[{"position":1,"text":"no 500","checked":false,"note":""}]}

$ curl -s -X POST -H 'Content-Type: application/json' localhost:7700/work-items -d '{"projectId":1,"parentId":1,"type":"task","title":"Reproduce"}'
{"id":2,"projectId":1,"parentId":1,"type":"task","title":"Reproduce","description":"","status":"backlog","teamId":null,"proposedBy":"","suggestedTeamId":null,"queuePosition":null,"tokenBudget":1000000,"createdAt":"2026-10-13T09:00:01.223456Z","closedAt":null,"acceptanceLines":[]}

$ curl -s -X PUT -H 'Content-Type: application/json' localhost:7700/work-items/1/brief -d '{"body":"Email and password login","author":"user"}'
{"id":1,"workItemId":1,"body":"Email and password login","author":"user","createdAt":"2026-10-13T09:00:02.123456Z"}

$ curl -s -X POST -H 'Content-Type: application/json' localhost:7700/spaces -d '{"projectId":1,"workItemId":1,"name":"build","backend":"claude"}'
{"id":1,"projectId":1,"workItemId":1,"name":"build","role":"lead","backend":"claude","permissionMode":"","paused":"","tokenBudget":1000000,"team":"solo","createdAt":"2026-10-13T09:00:03.123456Z","closedAt":null}

$ curl -s -X POST -H 'Content-Type: application/json' localhost:7700/messages -d '{"spaceId":1,"author":"user","body":"Start with the form"}'
{"id":1,"spaceId":1,"parentId":null,"author":"user","body":"Start with the form","createdAt":"2026-10-13T09:00:04.123456Z"}

$ curl -s -X POST -H 'Content-Type: application/json' localhost:7700/memory -d '{"kind":"decision","body":"Use sessions, not JWT","why":"One server, no token refresh","scope":"work_item:1"}'
{"id":1,"kind":"decision","body":"Use sessions, not JWT","why":"One server, no token refresh","scope":"work_item:1","author":"user","role":"user","status":"accepted","retiredAt":null,"replacedBy":null,"createdAt":"2026-10-13T09:00:05.123456Z"}

$ curl -s -X POST -H 'Content-Type: application/json' localhost:7700/turns -d '{"spaceId":1,"role":"developer","backend":"claude"}'
{"id":1,"spaceId":1,"role":"developer","backend":"claude","sessionId":"","status":"running","error":"","usage":{"inputTokens":0,"outputTokens":0,"cacheReadTokens":0,"cacheWriteTokens":0},"startedAt":"2026-10-13T09:00:07.123456Z","endedAt":null,"wakeText":"","wakeParent":null,"retried":false,"checkpoint":null}

$ curl -s -X PATCH -H 'Content-Type: application/json' localhost:7700/turns/1 -d '{"status":"done","sessionId":"3c1296cc","usage":{"inputTokens":4,"outputTokens":123,"cacheReadTokens":22898,"cacheWriteTokens":0}}'
{"id":1,"spaceId":1,"role":"developer","backend":"claude","sessionId":"3c1296cc","status":"done","error":"","usage":{"inputTokens":4,"outputTokens":123,"cacheReadTokens":22898,"cacheWriteTokens":0},"startedAt":"2026-10-13T09:00:07.123456Z","endedAt":"2026-10-13T09:00:08.123456Z","wakeText":"","wakeParent":null,"retried":false,"checkpoint":null}

$ curl -s -X PATCH -H 'Content-Type: application/json' localhost:7700/turns/1 -d '{"status":"failed"}'
{"error":"invalid: turn 1 already ended"}

$ curl -s "localhost:7700/messages?space=1&after=1"
[]

$ curl -s -X DELETE -w ' %{http_code}\n' localhost:7700/messages/1
{"error":"method not allowed"}
 405
```

To see restart recovery: create a turn, stop the server with Ctrl-C, start it again. The
log shows `marked 1 running turns failed: server restarted`, `GET /turns/2` is `failed`, and
`GET /messages?space=1` ends with the message from `reef`.
