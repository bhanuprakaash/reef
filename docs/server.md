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
  `chat-<channel id>.lock` while it runs, and so does the server for a live channel;
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
- SIGINT or SIGTERM stops the server: it interrupts every running channel turn (stored
  `failed`, `interrupted`), stops every agent, then gives requests in flight one second.
  The agents stop in parallel, so stop time is about one agent stop (about a second), not
  one per live channel.

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
embeds `web/dist` as it was at build time. Add a project (an absolute path to an existing
folder), then a feature, then a channel. A channel's agent is the `claude` or `codex` CLI,
logged in and on `PATH`. The new channel form picks a team from `GET /teams`. The `/teams`
page lists teams and edits one's roles and gates, saving with `PUT /teams/{name}`; "Copy as
new team" applies the same roles, gates and MCP servers under a new name with `?from=`,
rejected if that name is taken. Each message
shows its author, replies are folded under their thread root with a count, and a thread
opens in a pane where you can reply. The running turn names its teammate. A wake held at a gate
shows Approve and Reject, and a pause from the hop cap shows in the cap banner. A channel
with rollbackable turns (`GET /turns?channel=ID&rollbackable=1`) has a Checkpoints list; Rollback on a row asks to confirm, then does
`POST /turns/{id}/rollback`. A checkpoint taken in another tree, such as a quick session's turn after a promote, is not listed. A stopped or failed turn shows `Failed: <error>` with its tool rows; its partial reply is the stored message, so the live block does not repeat it.
The Inbox link lists what waits on you from [`/inbox/events`](#inbox), with a count and the
action of each item (Approve or Reject, Continue, Retry, Open). The window asks once, on a click,
to show desktop notifications and then shows one for each item that is new since the page loaded.

Client routes such as `/features/3`, `/tasks` and `/inbox` have the same paths as the REST API. In the Go server, `GET /` and `HEAD /` always get the app's `index.html`; other `GET` or `HEAD` requests get it when the `Accept` header includes `text/html`. Every other request goes to the API, so `curl /projects` and the app's own `fetch` calls (`Accept: application/json`) are unchanged. A file that exists in `web/dist`, such as `/assets/...`, is served as is. The Host and Content-Type checks guard the API only, not these files.

Dev mode: run `reef serve` on `:7700`, then `cd web && npm run dev` and open
`http://localhost:5173`. Vite proxies the API paths to `127.0.0.1:7700`.

## Errors

Every error has a JSON body: `{"error": "feature slug \"login\" is taken in its project: conflict"}`.

| Status | When |
| --- | --- |
| 400 | Bad JSON, an unknown field or data after the JSON body, a bad or missing field, a non-number id, an unknown parent id inside a body (`unknown project 9`) |
| 403 | The `Host` is not `127.0.0.1`, `localhost` or `::1`. Body is `{"error":"forbidden host"}` |
| 404 | The id in the path (or a required query filter) matches nothing, or no route knows the path. Body for an unknown path is `{"error":"not found"}` |
| 405 | The path exists, the method does not. Body is `{"error":"method not allowed"}` |
| 409 | A unique rule broke: a feature slug is taken in its project. A send to a channel with a turn running, to a channel paused by a cap other than the hop cap or to a channel open in a `reef chat`; `continue` on a channel that is not paused or has a turn running; `retry` of a turn that is not [retryable](#channels-live), or on a channel with a turn running or paused; `rollback` on a channel with a turn running or open in a `reef chat`, on a tree other than the checkpoint's, or on a tree a turn runs on; an approval that is not pending, or one approved while a turn runs or the channel is paused |
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
[`006_team.sql`](../store/migrations/006_team.sql) (`team`, `role`, `channel.team_id`),
migration 7 [`007_thread.sql`](../store/migrations/007_thread.sql) (`message.parent_id`) and
migration 8 [`008_approval.sql`](../store/migrations/008_approval.sql) (`gate`, `approval`) and
migration 9 [`009_approval_kind.sql`](../store/migrations/009_approval_kind.sql) (`approval.kind`) and
migration 10 [`010_role_write.sql`](../store/migrations/010_role_write.sql) (`role.write`) and
migration 11 [`011_turn_wake.sql`](../store/migrations/011_turn_wake.sql) (`turn.wake_text`, `turn.wake_parent`, `turn.retried`) and
migration 12 [`012_turn_checkpoint.sql`](../store/migrations/012_turn_checkpoint.sql) (`turn.checkpoint`).
Migrations 4 and 5 add columns to `channel` (see [channel](#channel)).

### project

| Field | JSON | Notes |
| --- | --- | --- |
| `id` | number | |
| `name` | string | required |
| `path` | string | required; the repo folder, stored as an absolute path with symlinks resolved |
| `createdAt` | string | |

### project_memory

One body per project, kept as a column on `project` (migration 14
[`014_project_memory.sql`](../store/migrations/014_project_memory.sql)). The user writes it; it
is loaded into every fresh session's context prompt (see [memory.md](memory.md#the-context-prompt)).

| Field | JSON | Notes |
| --- | --- | --- |
| `projectId` | number | from the path |
| `body` | string | default `""` |

### feature

| Field | JSON | Notes |
| --- | --- | --- |
| `id` | number | |
| `projectId` | number | required on create; never changes. Deleting the project deletes the feature |
| `slug` | string | required; lowercase letters, digits and `-`, not starting with `-` (`^[a-z0-9][a-z0-9-]*$`) and not only digits (`invalid: feature slug "2024" must contain a letter or -`), because it becomes a branch and folder name and a digit-only `<feature>` ref means an id; unique within the project (409 if taken); fixed after create, a slug in a `PUT` body is ignored |
| `name` | string | required |
| `goal` | string | default `""` |
| `stage` | string | default `ideation`; not checked against a list |
| `createdAt` | string | |

### feature_github

The last GitHub PR state the [GitHub poll](#github-poll) posted about a feature, kept across
a restart so it does not repeat a note. No REST route; tables `feature_github` (one row per
feature: `pr_number`, `pr_state`, `pr_url`) and `feature_github_check` (one row per
`(feature, check name, run key)` of the feature's PR: `run_key`, `woken`; a row's existence
means it was noted), migration 16
[`016_feature_github.sql`](../store/migrations/016_feature_github.sql).

### brief

One row per version. A feature's brief is its latest version.

| Field | JSON | Notes |
| --- | --- | --- |
| `id` | number | version order |
| `featureId` | number | from the path |
| `body` | string | required |
| `author` | string | required |
| `createdAt` | string | |

### role_notes

One row per feature and role.

| Field | JSON | Notes |
| --- | --- | --- |
| `featureId` | number | from the path |
| `role` | string | from the path |
| `body` | string | may be empty |
| `updatedAt` | string | |

### channel

| Field | JSON | Notes |
| --- | --- | --- |
| `id` | number | |
| `projectId` | number | required on create; never changes |
| `featureId` | number? | set on create. It changes only from `null`, once, when a promote gives a quick session its feature. `null` is a quick session. Must be a feature of the same project. Deleting the feature deletes the channel |
| `name` | string | required |
| `role` | string | the name of the lead role of the channel's team. Empty when not set. For a channel from before this field existed, the role of its newest turn |
| `backend` | string | `claude` or `codex`, the agent that runs the channel. Empty when not set. For an older channel, the backend of its newest turn |
| `permissionMode` | string | passed to the backend as is when the channel's agent starts or resumes. Empty means the backend default |
| `paused` | string | why the channel is paused (see [Caps](#caps)). Empty when it is not paused. Set by the server only; a client cannot set it |
| `tokenBudget` | number | tokens the channel may use before it pauses. Starts at 1,000,000. Set by the server only |
| `team` | string | the name of the channel's [team](#team). `solo` when none is given, and for every channel from before teams. Set on create, never changes |
| `createdAt` | string | |

### message

| Field | JSON | Notes |
| --- | --- | --- |
| `id` | number | |
| `channelId` | number | required |
| `parentId` | number? | the id of the thread root this message replies to. `null` for a top-level message. It must be a top-level message of the same channel: a reply to a reply is 400. Set on create |
| `author` | string | required; free text, for example `user` or a role |
| `body` | string | required |
| `createdAt` | string | |

A top-level message is the root of a thread. The author of the root owns the thread.

### turn

One agent run in a channel.

| Field | JSON | Notes |
| --- | --- | --- |
| `id` | number | |
| `channelId` | number | required |
| `role` | string | required |
| `backend` | string | required; `claude` or `codex` |
| `sessionId` | string | default `""`; set on create, while the turn runs, or when it ends |
| `status` | string | `running`, `done` or `failed`. A new turn is always `running` |
| `error` | string | default `""` |
| `usage` | object | `inputTokens`, `outputTokens`, `cacheReadTokens`, `cacheWriteTokens` (numbers, all four always present). The SQL columns are `input_tokens` and so on |
| `startedAt` | string | |
| `endedAt` | string? | `null` while running |
| `wakeText` | string | default `""`; the text the turn ran: the wake, without the context prompt. Empty for a turn from before this field existed and for a wrap-up or promote turn |
| `wakeParent` | number? | the thread the wake was in, `null` for the top level. Set on create with `wakeText` |
| `retried` | boolean | `true` once a retry of this failed turn has started. Set by `retry`, never on create |
| `checkpoint` | string? | the git ref of the snapshot taken before the turn, `refs/reef/checkpoints/<turn id>`. `null` for a turn of a role without `write`, for a turn in a folder that is not a git checkout or has no commit, and for a turn from before checkpoints. Set by the server, never by a request |
| `model` | string | default `""`; the backend's model name, set from the agent's first event once the turn ends |
| `costUsd` | number? | `null` until the turn ends with a cost; codex turns never get one. A failed turn gets one too, when claude reported one |
| `prompt` | string | not in the turn's own JSON (`GET /turns`, `GET /turns/{id}`); the full text sent to the agent (context preamble and notes included), set once the turn ends. `GET /channels/{id}/inspector` has its own view of it |

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

### decision

| Field | JSON | Notes |
| --- | --- | --- |
| `id` | number | |
| `featureId` | number | required |
| `messageId` | number? | optional. Must be a message in a channel of the same feature. `null` if that message is later deleted |
| `body` | string | required |
| `why` | string | default `""` |
| `author` | string | required |
| `createdAt` | string | |

### task

The list is global. A task belongs to a project and may name a feature.

| Field | JSON | Notes |
| --- | --- | --- |
| `id` | number | |
| `projectId` | number | required on create; never changes |
| `featureId` | number? | must be a feature of the same project. `null` if that feature is later deleted |
| `title` | string | required |
| `body` | string | default `""` |
| `status` | string | `todo` (default), `doing` or `done` |
| `createdAt` | string | |
| `updatedAt` | string | set on every update |

### team

A named set of roles and the gates between them. A team has exactly one lead role. `reef
team apply` (see [memory.md](memory.md#teams)) and `PUT /teams/{name}` both write a team,
through the same validation. Migration 6 seeds the team `solo`, with one role, `lead` on
`claude`, and points every channel that exists at it.

| Field | JSON | Notes |
| --- | --- | --- |
| `name` | string | required; unique; letters, digits, `_` and `-` only. Applying a name that exists replaces that team's roles and gates in one transaction |
| `roles` | array | [roles](#role), the lead first, then the others in the order of the file |
| `gates` | array | [gates](#gate); left out when the team lists none |

### role

| Field | JSON | Notes |
| --- | --- | --- |
| `name` | string | required; unique within the team; letters, digits, `_` and `-` only, and not `user`. It is the `author` of what the role posts, and the name an `@mention` uses |
| `lead` | boolean | exactly one role of a team is the lead. The lead answers a message that nobody else owns |
| `write` | boolean | default `false`. Only a role with `write` may change files; it gets a checkpoint and the tree's write lock. claude: without it the role never gets `Edit`, `Write`, `MultiEdit` or `NotebookEdit` (they are dropped from `tools` and passed as `--disallowedTools`, so an allow rule in the user's or project's `.claude/settings.json` cannot grant them) and runs in the `default` permission mode, so `Bash` runs only as `tools` lists it. A claude role without `write` cannot list plain `Bash`, `Bash(*)` or `Bash(:*)` in `tools`: it would still change files through the shell, so applying the team is `invalid`; list patterns such as `Bash(go test:*)` instead. A role saved before this check existed has it dropped from `tools` at session start too, the same as the edit tools. `Bash` patterns are trusted as given: `Bash(git diff:*)` also lets `git diff --output=file` write a file. With it the role keeps its `tools` and permission mode (it still needs `acceptEdits`, or `Edit` in `tools`, to edit unasked) and also gets `Task`. codex: without it the sandbox is `read-only`; with it `workspace-write` unless the role's `permissionMode` is set. The seeded `solo` lead has `write` |
| `backend` | string | required; `claude` or `codex` |
| `permissionMode` | string | `""` is the backend default; claude: `default` or `acceptEdits`; codex: `read-only` or `workspace-write`. The same rule as a channel's `permissionMode` |
| `persona` | string | goes at the start of the role's first prompt, and again, marked as changed, at its next wake once it or `skills` differs from what the role's session last got |
| `skills` | string | goes after the persona, under `Your skills:` |
| `tools` | array of strings | claude: passed as `--allowedTools` with `mcp__reef__*`, so these tools and the reef tools run without asking; the role's own `mcpServers` do not (list `mcp__mcp1__*` here to allow one). codex: ignored |
| `mcpServers` | array | extra MCP servers for the role, each `command`, `args`, `env` (stdio) or `url` (HTTP). They are named `mcp1`, `mcp2`, ... next to the `reef` server |

A channel's lead takes the channel's own `backend` and `permissionMode`, which `POST
/channels` filled from the lead role unless the request set them. The other roles use their
own.

### gate

A switch for the wakes one role sends another: a wake from role `from` to role `to`.

| Field | JSON | Notes |
| --- | --- | --- |
| `from` | string | required; a role of the team |
| `to` | string | required; a role of the team |
| `on` | boolean | `true`: the wake waits for the user's approval. `false`: it runs at once |

An edge the team does not list uses the default: `lead` to `developer`, `architect` to
`developer`, `developer` to `tester` and `tester` to `developer` are on, every other edge is
off. A team file can list an edge with `on: false` to switch a default off.

### approval

A wake that waits at a gate. The SQL table is `approval`.

| Field | JSON | Notes |
| --- | --- | --- |
| `id` | number | |
| `channelId` | number | |
| `from` | string | the role whose message woke it |
| `to` | string | the role that would run |
| `messageId` | number | the message that woke it |
| `text` | string | the prompt the wake will run, for example `[channel 5, message 28]\nlead wrote: @architect design a form` |
| `status` | string | `pending`, `approved` or `rejected`. Decided once |
| `createdAt` | string | |

The table also keeps the wakes a pause holds, see [Caps](#caps): a row of kind `hold` is never
listed, approved or rejected through the API. Deleting the channel deletes its approvals.

### token

Lets one teammate's `reef mcp` act as one role in one channel. No REST route: `reef
token`, `reef chat` and the server (one per teammate session) make them, `reef mcp` reads
them, and the one that made a token deletes it when its session ends. See
[memory.md](memory.md#tokens).

| Field | JSON | Notes |
| --- | --- | --- |
| `token` | string | the key; 64 hex characters from 32 random bytes |
| `channelId` | number | required. Deleting the channel deletes the token |
| `role` | string | required; the author of what the teammate writes |
| `createdAt` | string | never expires |

### proposal

A wrap-up proposal: the new brief, the role's notes and the decisions to record.
`propose_wrap` makes one. `reef chat` prompts for approval, and `reef wrap approve` or
`reef wrap reject` decides it; the server offers the same over REST (see [Wrap-up and
promotion](#wrap-up-and-promotion)). See [memory.md](memory.md#wrap-up).

| Field | JSON | Notes |
| --- | --- | --- |
| `id` | number | |
| `channelId` | number | required; must be a channel of a feature. Deleting the channel deletes the proposal |
| `role` | string | required; author of the brief, notes and decisions it applies |
| `brief` | string | required, not empty; the full new brief |
| `notes` | string | may be empty; replaces the role's notes |
| `decisions` | array | `{"body", "why"}` each, `body` required. The SQL column holds them as JSON text |
| `status` | string | `pending` (default), `approved` or `rejected`. Decided once |
| `createdAt` | string | |
| `decidedAt` | string? | `null` while pending |

## REST

`{id}` is a number. Lists come back as `[]` when empty, ordered by id (oldest first).
Create returns 201 and the row. Update returns 200 and the row. Delete returns 204 and no
body. Deleting a row deletes what hangs off it by foreign key: a project takes its
features, channels and tasks; a feature takes its briefs, notes, decisions and channels;
a channel takes its messages, turns, tokens, proposals and approvals; a turn takes its
tool calls.

| Route | Filters and body | OK | Notable errors |
| --- | --- | --- | --- |
| `GET /health` | | 200 `{"status":"ok"}` | |
| `GET /projects` | | 200 | |
| `POST /projects` | `name`, `path` | 201 | 400 missing field, or a `path` that is not an absolute path to an existing folder |
| `GET /projects/{id}` | | 200 | 404 |
| `PUT /projects/{id}` | `name`, `path` | 200 | 400 (same `path` rule), 404 |
| `DELETE /projects/{id}` | | 204; the worktrees of its features and the checkpoint refs of the turns of all its channels are removed, its branches are kept | 404, 409 a feature's worktree has uncommitted changes (nothing is removed) |
| `GET /projects/{id}/memory` | | 200; `body` is `""` when none has been set | 404 |
| `PUT /projects/{id}/memory` | `body`; replaces the old memory | 200 | 404 |
| `GET /features` | `?project=ID` | 200 | 400 bad id |
| `POST /features` | `projectId`, `slug`, `name`, `goal`, `stage` | 201 | 400 unknown project, 409 slug taken |
| `GET /features/{id}` | | 200 | 404 |
| `PUT /features/{id}` | `name`, `goal`, `stage` | 200 | 400, 404 |
| `DELETE /features/{id}` | | 204; the feature's worktree and the checkpoint refs of its turns are removed, its branch is kept | 404, 409 the worktree has uncommitted changes (nothing is removed) |
| `GET /features/{id}/brief` | | 200 latest version | 404 no feature, or no brief yet |
| `PUT /features/{id}/brief` | `body`, `author` | 201 new version | 400, 404 |
| `GET /features/{id}/brief/versions` | | 200 oldest first | 404 |
| `GET /features/{id}/notes/{role}` | | 200 | 404 no feature, or no notes for the role |
| `PUT /features/{id}/notes/{role}` | `body`; replaces the old notes | 200 | 404 no feature |
| `DELETE /features/{id}/notes/{role}` | | 204 | 404 no feature, or no notes for the role |
| `POST /features/{id}/worktree` | no body needed | 200 `{"path": "..."}`; makes the feature's worktree the same way the first wake of one of its channels would, reusing it when it already exists | 404 |
| `GET /features/{id}/diff` | | 200 the worktree's changes against the commit its branch and the project's HEAD last shared, uncommitted and untracked files included, as `[{"path", "status": "added"\|"modified"\|"deleted", "hunks": [{"header", "lines": [{"kind": "add"\|"del"\|"ctx", "old", "new", "text"}]}]}]`; `[]` when the feature has no worktree yet | 404 |
| `GET /teams` | | 200 all teams by name, each with its roles and gates; a role has no `mcpServers`, whose `args` and `url` may hold API keys | |
| `GET /teams/{name}` | | 200, without `mcpServers` as above; each role shows `write` | 404 |
| `PUT /teams/{name}` | the team JSON `reef team apply` takes (`name`, when given, must match the path); applies it the same way, with the same validation. `?from=<source>` instead copies `source`: a role's body that omits `mcpServers` takes `source`'s role of the same name, and `name` must not exist yet, checked and inserted in one statement so two concurrent copies to the same name cannot both win | 200 the applied team, as `GET /teams/{name}` | 400 the same `invalid: ...` text `reef team apply` gives, or a body name that does not match the path; a role's body that omits `mcpServers` keeps the role's stored ones; 404 `?from=` names a team that does not exist; 409 `?from=` names a team that already exists |
| `GET /channels` | `?project=ID`, `?feature=ID` | 200 | 400 bad id |
| `POST /channels` | `projectId`, `featureId`, `name`, `team` (a team name; default `solo`), `backend` (`claude` or `codex`; default the team lead's), `permissionMode` (`""`; claude: `default` or `acceptEdits`; codex: `read-only` or `workspace-write`; default the lead's when `backend` is the lead's), `role` (empty or `lead`; the channel's `role` becomes the name of the team's lead role) | 201 | 400 unknown project, feature or team, feature of another project, bad backend or permission mode, a role other than `lead` |
| `GET /channels/{id}` | | 200 | 404 |
| `PUT /channels/{id}` | `name` (required), `permissionMode`; project, feature, role and backend stay unchanged. An omitted permission mode stays unchanged. A set permission mode is validated for the channel's backend and applies the next time its agent starts | 200 | 400 bad JSON or permission mode, 404 |
| `DELETE /channels/{id}` | stops the channel first if it is live; a send during the delete is 404 | 204 | 404 |
| `POST /channels/{id}/send` | `text`, `parent` (optional: the id of a top-level message of this channel; the message is stored as a reply in its thread) | 202, no body | 400 empty text, an unknown field (`role` is not a field), a `parent` that is not a top-level message of this channel, or `/wrap` on a quick session, 404 (also while the channel is being deleted), 409 turn running, channel paused (a hop cap pause does not count), open elsewhere or stopped before its turn began, 503 |
| `GET /channels/{id}/events` | | 200 `text/event-stream` | 404 |
| `GET /channels/{id}/inspector` | | 200 per role: `role`, `turns`, `usage` (summed), `costUsd` (summed, `null` when no turn of the role has a cost); per turn: `id`, `role`, `startedAt`, `status`, `model`, `usage`, `costUsd`, `prompt`, `toolCalls` (`name`, `input`, `result`) | 404 |
| `POST /channels/{id}/stop` | no body needed | 204, also when the channel is not live or does not exist | |
| `POST /channels/{id}/continue` | no body needed | 204, no body | 404, 409 not paused or a turn running |
| `GET /channels/{id}/approvals` | | 200 the channel's pending gate approvals, oldest first; `[]` when none | 404 |
| `POST /channels/{id}/approvals/{aid}` | `approve` (required: `true` runs the held wake, `false` drops it) | 204, no body | 400 no `approve`, 404 no such approval in this channel, 409 not pending, or approve while a turn runs, the channel is paused or 6 hops ran (it stays pending), 503 |
| `POST /channels/{id}/wrap` | no body needed | 202, no body | 400 a quick session (no feature); 404, 409 and 503 as send |
| `POST /channels/{id}/promote` | `slug` and `name` for a new feature, or `featureId` for an existing one; exactly one form | 202 the channel | 400 bad body, channel already has a feature, feature of another project; 404; 409 slug taken, or as send; 503 |
| `GET /channels/{id}/proposal` | | 200 the channel's pending proposal | 404 none pending |
| `POST /proposals/{id}/approve` | no body needed | 200 the proposal and `decisionsAdded` | 400 already decided, 404 |
| `POST /proposals/{id}/reject` | no body needed | 200 the proposal | 400 already decided, 404 |
| `GET /messages` | `?channel=ID` (required), `?after=ID` (only newer ids) | 200, each with `parentId` | 400 no channel, 404 unknown channel |
| `POST /messages` | `channelId`, `parentId`, `author`, `body` | 201 | 400 unknown channel, missing field, or a `parentId` that is not a top-level message of that channel |
| `GET /tasks` | `?project=ID`, `?feature=ID`, `?status=todo\|doing\|done` | 200 | 400 bad id or status |
| `POST /tasks` | `projectId`, `featureId`, `title`, `body`, `status` | 201 | 400 |
| `GET /tasks/{id}` | | 200 | 404 |
| `PUT /tasks/{id}` | `title`, `body`, `status`, `featureId` | 200 | 400, 404 |
| `DELETE /tasks/{id}` | | 204 | 404 |
| `GET /turns` | `?channel=ID`, `?status=running\|done\|failed`, `?retryable=1` (only the turns `retry` accepts), `?rollbackable=1` (needs `channel`; only the turns with a `checkpoint` taken in the tree the channel runs in now, so `rollback` is not a 409 for the tree) | 200 | 400 bad id or status, or `rollbackable` without a channel, 404 `rollbackable` on an unknown channel |
| `POST /turns` | `channelId`, `role`, `backend`, `sessionId`, `wakeText`, `wakeParent` | 201 | 400 bad backend or unknown channel or message |
| `GET /turns/{id}` | | 200 | 404 |
| `PATCH /turns/{id}` | `status` (`done` or `failed`), `error`, `usage`, `sessionId`. With no `status`, only `sessionId` is set and the turn stays `running` | 200 | 400 bad status, negative usage, empty `sessionId` without a status, `error` or `usage` without a status, or already ended; 404 |
| `POST /turns/{id}/retry` | no body needed | 202, no body; the wake of the failed turn runs again as a new turn of its role, and the failed turn is `retried` | 404, 409 not retryable (see [Channels live](#channels-live)), or as send for a channel with a turn running or paused, 503 |
| `GET /inbox` | | 200 the items waiting on the user, newest first; `[]` when none. See [Inbox](#inbox) | |
| `GET /inbox/events` | | 200 `text/event-stream` | |
| `POST /turns/{id}/rollback` | no body needed | 204, no body; the channel's worktree is as it was before the turn, and `reef` posts `Rolled back to before turn N.` | 400 the turn has no `checkpoint`, 404, 409 the channel has a turn running, a `reef chat` has it open, its tree is not the one the checkpoint was taken in, or a turn of any role runs on the tree, 503 |
| `GET /decisions` | `?feature=ID` (required) | 200 | 400 no feature, 404 unknown feature |
| `POST /decisions` | `featureId`, `messageId`, `body`, `why`, `author` | 201 | 400 missing field, unknown feature, or a message of another feature |
| `GET /decisions/{id}` | | 200 | 404 |

A `POST` with no body (`stop`, `continue`, `wrap`, `approve`, `reject`) still needs
`Content-Type: application/json`, or the guard answers 415. These routes ignore the body,
so an empty one or `{}` both work. `send` and `promote` read a JSON body, and an empty one
is 400.

Create requests take the whole object from the table above. A field you leave out takes
its default. Other `PUT` requests replace the whole object. Update behavior is
route-specific: `PUT /channels/{id}` keeps an omitted `permissionMode`, while
`PATCH /turns/{id}` is the only partial update for other resources.

## Channels live

`POST /channels/{id}/send` runs the channel's agents through
[`channel.Open`](../channel/session.go), the code `reef chat` uses. A channel has a
[team](#team): the lead answers the user, and the other roles are specialists that run when
a message wakes them.

- The server stores the user message, then answers 202. The turns run in the background, not
  in the request. The text `/wrap` runs the wrap-up turn (400 on a quick session, before any
  agent turn). Every other text, `/promote` included, is a plain message: only `POST
  /channels/{id}/promote` promotes. A send with `parent` stores the message as a reply in that
  thread.
- Each role of the team has its own agent session, its own `reef mcp` token, its own turns and
  its own session id. The role's backend, permission mode, tools and MCP servers are set up
  for it (the lead uses the channel's backend and permission mode). Its first prompt starts
  with its persona and skills (see [memory.md](memory.md#the-context-prompt)).
- An agent starts when its first turn runs, not when the channel opens. A first message that
  wakes a specialist starts no lead. A role that already has a session resumes it by its own
  newest session id (see [memory.md](memory.md#reef-chat)). An agent that fails to start shows
  as a `failed` turn of its role, with the start error, or `interrupted` when a stop cut the
  start short. The server logs every failed turn.
- An agent of a channel under a feature runs in the feature's worktree,
  `<data folder>/worktrees/<project id>/<slug>`, on branch `feature/<slug>`. It is made at
  the first start of any agent of the feature, from the project's HEAD, with `git worktree
  add`; a worktree or branch that exists is reused, so every channel of the feature shares
  one tree. A branch that is checked out elsewhere (in the main checkout, or in the worktree
  of another project on the same repo) cannot be added: the start fails with `branch
  feature/<slug> is checked out at <path>; switch that checkout to another branch`. A
  quick session runs in the project path. The feature is read at each agent
  start, so a promoted channel moves to the worktree at its next wake. A project that is not
  a git repo, or a repo with no commit, has no worktree: the start fails as a `failed` turn
  whose error is `features need the project folder to be a git repo with a commit: <path>:`
  and git's error, and the server goes on. A worktree whose folder was deleted is pruned and
  made again at the next wake: an agent running in the missing folder is closed, and the
  next one resumes the session in the new worktree.
- Before each turn of a role with `write` in a git checkout (a feature's worktree, or the
  project path of a quick session that is a git repo), the server stores a checkpoint: a commit
  of HEAD plus the whole working tree, untracked files included and ignored ones left out, at
  `refs/reef/checkpoints/<turn id>`. It is built in a temporary index that starts as a copy of
  the real one, so the index, HEAD and the files do not change. The ref is on the turn's
  `checkpoint`. If the checkpoint fails, the turn is stored `failed` with git's error and the
  agent gets no prompt. A repo with no commit has nothing to snapshot: its turns run with
  `checkpoint` null. A role without `write` gets none. The checkpoint commit has two parents:
  HEAD, then a commit of the index as it was, so a rollback can bring back what was staged.
  Its message records the directory and the branch it was taken in (`dir:` and `branch:`
  lines; no branch when HEAD was detached).
- One write turn at a time runs on a tree, across all channels and processes on the data
  folder: the feature's worktree, or the project path for quick sessions. A wake of a role with `write` waits for
  the write turn that runs on its tree, then runs. Its turn row is created when it starts, so
  `startedAt` is after the other turn's `endedAt`. A stop ends the wait, and the turn is
  stored `failed` as `interrupted`. A role without `write` is not held: it runs beside a
  write turn, and counts as a reader of the tree for its turn, which only a rollback
  respects. The first start
  of a feature's worktree takes a lock of its own, so two channels waking at once add it once.
  The locks are `flock` files in `<data folder>/locks/`, named from a hash of the tree path
  (`.lock` for the writer, `.read` for readers), so `reef serve` and `reef chat` share them.
  A waiting write turn polls its lock every 20 ms. A rollback takes the writer lock without
  waiting and checks the reader file, so a read-only turn of any process refuses it.
- `POST /turns/{id}/rollback` puts the channel's tree (the feature's worktree, or the project
  path of a quick session) back as the turn's checkpoint saw it: the branch the checkpoint was
  taken on is checked out at the HEAD it had, so commits made since, and a branch the turn
  switched to, are left behind (a detached checkpoint resets whatever is checked out); tracked
  and untracked files get the content they had, the same changes are staged, and files made
  since are removed. Ignored files are left alone. It claims the channel like a send. It is
  409, never a wait, when the channel's tree is not the directory the checkpoint was taken in
  (a quick session's turn after a promote), when a `reef chat` holds the channel, or when a
  turn of any role of any channel runs on the tree (the write lock is held, or a read-only
  turn is a reader). A reader that starts during the restore is not held back. A turn with
  no `checkpoint` is 400. Then `reef` posts `Rolled back
  to before turn N.` in the channel. The agent's session is not rewound. When a turn of
  a role that has run before in the channel is sent, its prompt gets the lines `reef wrote: ...`
  that `reef` posted since that role's last turn began, after the header line, so the role
  learns of the rollback (and of a restart failure note) even when its wake was held at a gate
  or is a retry; a role that never ran gets none. The wake kept as `wakeText` and shown in an
  approval does not have them.
- `DELETE /features/{id}` cleans what Reef made for the feature. It stops the agents of the
  feature's channels first (a running turn is stopped, as when a channel is deleted). A
  worktree with uncommitted changes (untracked files count) is 409 with `feature <slug> has
  uncommitted changes in <path>; commit or discard them first`, and nothing is removed.
  Otherwise the worktree is removed with `git worktree remove`, the feature rows are
  deleted, and last the `refs/reef/checkpoints/<turn id>` refs of every turn of its channels
  are deleted. The worktree goes first because it is the step that can refuse or fail, and a
  later wake of a feature makes it again; the refs go after the rows that name them, so a
  failure there leaves refs that no turn points to, and the delete is 500. The branch
  `feature/<slug>` is kept: it is the user's work. A feature with no worktree folder (no agent
  ever started) only has its rows deleted. Deleting a channel keeps worktrees and refs.
- `DELETE /projects/{id}` does the same for every feature of the project, and deletes the
  checkpoint refs of the turns of every channel of the project, quick sessions included (their
  refs are taken in the project folder). It checks every worktree first: one with uncommitted
  changes is 409 for that feature, and nothing is removed. Then it removes all worktrees,
  deletes the project rows, and last deletes the refs, in the same order and with the same
  failure rule as a feature delete. Branches are kept.
- The lead stays alive between turns, until `POST /channels/{id}/stop` or the server stops. A
  specialist's agent is stopped, and its token deleted, when its turn ends. Its next wake
  resumes its session.
- The server holds `chat-<id>.lock` while the channel has a live session, so `reef chat
  --channel ID` fails with `channel 1 is open elsewhere: conflict`. A send to a channel a
  `reef chat` holds is 409 too.
- `stop` interrupts a running turn (stored `failed`, `interrupted`), drops the wakes still
  waiting, then stops every agent of the channel, deletes their tokens and releases the lock.
  It also works while the send is still checking the caps or starting an agent: the send then
  answers 409 `channel 1 was stopped`, and no turn runs. Once the send has changed something
  (a promote's change) or stored the user message, it does not give up: it answers 202 as
  usual, and its turn runs with the stop already in effect, so it is stored `failed`,
  `interrupted` and the agent gets no prompt. A turn that fails for another reason also stops
  every agent of the channel; the next turn opens them again, each by its own session id.
- `DELETE /channels/{id}` does the same as `stop` before it deletes the channel.
  `DELETE /features/{id}` and `DELETE /projects/{id}` do it for each of their channels.
  From the start of that stop until the delete is done, a send to the channel is 404
  `channel 1 is being deleted`, so no agent starts for a channel that is going away.
- Server shutdown does the same for every channel before it stops listening.
- A turn keeps the wake it ran as `wakeText` and `wakeParent`. `POST /turns/{id}/retry` on a
  `failed` turn runs that wake again for the same role as a new turn in a new chain, the way an
  approve does, with no new user message: the new turn answers in the same thread, and what its
  reply wakes is routed as usual. The failed turn is marked `retried` before the turn starts, so a
  second retry is 409; if the turn cannot start, the mark is taken back. Nothing retries a
  turn by itself. A turn is retryable when it is `failed`, not `retried`, has a `wakeText`, and
  no later turn of its role ran in the channel: a turn that a later one supersedes is 409. A
  turn the user stopped (`interrupted`) is retryable, since the user may want its wake run
  again. A wrap-up or promote turn keeps no wake, so it is never retryable: the user wraps or
  promotes again. `GET /turns?retryable=1` lists the same turns, which is what the window shows
  as failed turns with a Retry button. The retry counts as a message of the user for the hop cap.

### Routing

When a turn ends, the server reads the messages stored since the last one it read and routes
those written by the role of the turn that ended: its reply, which is stored as a message from
the role, and any message it posted with the `post_to_channel` tool (see
[memory.md](memory.md#reef-mcp)). Other messages, such as a `POST /messages` from the user in
the middle of a chain, are skipped: they start no turn and do not reset the hop count. Each
routed message goes through the router, [`channel.Route`](../channel/route.go), plain code with
no model. In order:

1. A reply from the user in a thread goes to the thread's owner, the author of the root. When
   the user owns the thread, it goes to the lead. An agent's reply in a thread has no owner
   rule: it wakes only the roles it mentions.
2. Otherwise every `@name` in the body that is a role of the team goes to that role, each
   once.
3. Otherwise a message from the user goes to the lead.
4. Otherwise nobody: a message from an agent that names no one wakes no one, in a thread or not.

The lead is the channel's `role`: on a channel whose role is `developer`, `@lead` names no
role, so the message goes to `developer` and no `lead` session starts.

A role never wakes itself. The user's own message is routed by the same rules, so a send
with `parent` on an architect's thread goes to the architect, and `@architect ...` goes to the
architect with no lead turn. `POST /messages` stores a message and wakes nobody. The `/wrap`
and promote turns are not routed (see below).

A wake is a turn of the woken role. Its prompt starts with a line `[channel 3, message 12]`
for a top-level message, or `[channel 3, message 12, thread 9]` for a reply: the channel, the
waking message and, for a reply, its thread root. Then comes the user's text as it is, or
`<author> wrote: <body>` for an agent's message. For a message in a thread, the earlier messages of the thread (`Thread so
far:`) come between the line and the text; they are the conversation being answered, so they
stay in the prompt. One routing pass wakes a role once for each thread: when several messages
in the same thread, or several top-level messages, wake the same role, for example a
`post_to_channel` that names `@architect` and a reply that does too, their texts are joined in
id order into one prompt. A top-level message and a reply in a thread that name the same role
wake it twice. The woken role's reply is stored in the thread the header names (`parentId` is the
thread root). When a top-level message woke the role, the reply is top-level too: it is the root
of a thread the role owns, and the user's reply in it goes to the role.

The first turn and the wakes after it are one chain. They run one at a time, in the order
the messages were stored, and the messages each turn stores are routed in turn and queued at
the end. The channel has a turn running until the chain is done, so a `send`, a `wrap`, a
`promote`, a `continue` or an approve during the chain is 409. A turn that fails, or is capped, ends the
chain; the wakes still waiting are dropped. An example chain: the user asks the lead; the
lead's reply says `@architect ...`; the architect runs and posts a plan, a new thread it
owns; the chain ends because the plan names no one. The lead gets no turn after the plan.

### Gates

A wake from a role to another is checked against the team's gates (see [gate](#gate)). A
wake across a gate that is on does not run. The server stores an [approval](#approval) with
the prompt it would have run and goes on with the rest of the chain. `GET
/channels/{id}/approvals` lists the pending ones. `POST /channels/{id}/approvals/{aid}` with
`{"approve":true}` runs the wake as a new chain (it needs an idle channel that is not
paused, else 409 and the approval stays pending). An approve at 6 hops pauses the channel for
the hop cap, answers 409 and reopens the approval. With `{"approve":false}` it only marks the
approval `rejected`. A decided approval is 409. Pending approvals are stored, so they survive a
restart.

### Events

`GET /channels/{id}/events` is a Server-Sent Events stream. It works for a channel that
is not live: it waits. A `: keepalive` comment comes every 15 seconds. Five kinds of event:

- `turn`: `{"role":"architect","status":"running"}` when a turn of that role starts, then
  `{"role":"architect","status":"done"}` or `{"role":"architect","status":"failed","error":"interrupted"}`
  when it ends. A chain of wakes sends one pair for each turn, with no `idle` between them. A
  wrap or promote turn in which the agent made no proposal ends `{"role":"lead","status":"done","note":"no
  proposal made"}`. `{"status":"idle"}` has no role. It is sent only when a subscriber
  connects and no turn runs.
- `agent`: one [agent event](agents.md) as JSON, as it comes, from the turn that is running.
- `pause`: `{"reason":"..."}` when a cap pauses the channel: right after the `turn`
  event of a capped turn, when a send finds the token budget used, or when the hop cap holds
  a wake. `{"reason":""}` when `continue`, or a user message after a hop cap pause, clears the pause.
- `approval`: an [approval](#approval) as JSON, `pending` when a gate holds a wake, and again
  with `approved` or `rejected` when it is decided.
- `note`: `{}`, sent when a message is posted outside a turn, such as a [GitHub
  poll](#github-poll) note, so a subscriber knows to re-read `GET /messages`.

A subscriber is brought up to date when it connects, so a reconnect needs no other
request to know the state. The server keeps the events of the running chain in memory. A
subscriber that connects during a chain first gets `turn` running and every `agent` event
of the chain so far, then the live ones. One that connects between chains first gets
`turn` idle. Either gets `pause` with the reason when the channel is paused. It does not get
the pending approvals: read them with `GET /channels/{id}/approvals`. The stored messages
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

event: approval
data: {"id":1,"channelId":5,"from":"lead","to":"architect","messageId":28,"text":"[channel 5, message 28]\nlead wrote: @architect design the form","status":"pending","createdAt":"2026-10-13T09:00:10.123456Z"}

event: turn
data: {"role":"lead","status":"done"}
```

### Inbox

`GET /inbox` lists what waits on the user in every channel. It is read from the rows of
approvals, channels, turns and messages, so it has no table and a restart keeps it. Each
item has `kind`, `channelId`, `featureId` (null for a quick session), `text` and `at`, and the
id to act with:

| `kind` | Listed while | `text` | Also has | Cleared by |
| --- | --- | --- | --- | --- |
| `approval` | a gate holds a wake: a pending [approval](#approval) | `lead wants to wake architect` | `approvalId` | approve or reject |
| `paused` | the channel's `paused` is not empty | the reason: hop cap, tool cap, token budget or a routing or store failure | | `continue`, or a user message after a hop cap pause |
| `failed` | a turn is retryable (see [Channels live](#channels-live)) and its error is not `interrupted`: a turn the user stopped is a decision, not an item | `architect failed: server restarted` | `turnId` | `retry`, or any later turn of that role, so of several failed turns only the newest shows |
| `waiting` | the channel's newest message is from an agent (not `user` or `reef`) and woke nobody: no wake held at a gate for it, no turn running, no other item for the channel, and the channel's newest turn was not stopped (the part of a reply a stop kept is not waiting) | the first 200 characters of that message | | any new message, such as a send |

`at` is the time of the approval, of the failed turn's end, of the message, and for `paused`
the end of the channel's newest turn. A channel that has a chain running lists no `waiting`,
`paused` or `failed` item, since the chain is not over and `continue` or `retry` would be 409. Rejecting a gate is a decision, so the message it held is
not `waiting`. A hold of a pause is not an item of its own: its channel is `paused`.

`GET /inbox/events` is one Server-Sent Events stream for all channels, with a
`: keepalive` every 15 seconds. Its only event is `inbox`, whose data is the whole list as
`GET /inbox` returns it. The first one comes when the stream connects; after that one comes each
time the list differs from the last one sent. The list is read again after every event of every
channel (turn, agent, pause, approval), when a chain ends, and after every request that is not a
`GET`, since a reject, a delete or a `POST /messages` can change it with no channel event. A change
a `reef` process makes straight in the database shows at the next keepalive tick at the latest.
When the server stops, a stream sends the last list and ends.

```
event: inbox
data: [{"kind":"approval","channelId":5,"featureId":2,"text":"lead wants to wake architect","at":"2026-10-14T09:00:10.123456Z","approvalId":1}]
```

## Wrap-up and promotion

`POST /channels/{id}/wrap` does what a send of the text `/wrap` does: it runs the wrap-up
turn of [`Session.Wrap`](../channel/session.go) (see [memory.md](memory.md#wrap-up)). The
agent calls `propose_wrap`; when the turn is done, `GET /channels/{id}/proposal` returns the
pending proposal, or 404 when the agent made none. `POST /proposals/{id}/approve` and
`/reject` decide it with the same effect as `reef wrap approve` and `reject`. Approve
also returns `decisionsAdded`, the decisions it saved. A proposal that is already decided is
400, like `invalid: proposal 1 is already approved` in the CLI.

`POST /channels/{id}/promote` turns a quick session into a feature. It takes `{"slug":
"login","name":"Login"}` to create the feature in the channel's project, or `{"featureId":
3}` for a feature that exists in that project. Only a channel with no feature can be
promoted. The server first checks that the channel has no feature, that a `featureId` is
a feature of the channel's project and that a new slug is free, so a refused promote
changes nothing and never opens the agent. It then checks that no turn runs and the caps,
as for send, and once the agent is open, in one transaction creates the feature and sets
the channel's `featureId`. It answers 202 with the
channel and starts a turn: the stored user message is `/promote`, and the agent gets a
prompt that names the feature and asks it to call `propose_wrap` once with the first brief,
written from the session, its notes and the decisions so far. The user approves that
proposal like a wrap-up, and approval makes brief version 1. The promote turn runs in the
agent the quick session already has. When it ends, every agent of the channel is stopped, so
the next wake of each role resumes its session in the feature's worktree. The promote first
posts a message from `reef`: `This feature works in <worktree path> on feature/<slug>.`, and
` Uncommitted changes stay on the main checkout.` after it when the main checkout has
uncommitted changes. When the feature already has
a brief, the prompt tells the agent to call `get_brief` and propose that brief merged with
the session, and approval makes the next version. A promote that fails (a taken
slug, a feature of another project) changes nothing, and so does one that is stopped
before its change. A stop after the change does not undo it: the promote answers 202 and
its turn is stored `failed`, `interrupted`.

## Caps

Three caps pause a channel. All are constants in the code, not settings.

- Tool calls per wake: one turn (one wake, or the turn that answers one user message) may
  make 25 tool calls, subagent calls included. The 26th interrupts the agent, ends the turn
  `failed` with the error `capped: more than 25 tool calls` and pauses the channel with that
  text as `paused`. The agent stops like after a failed turn; the next send resumes it by
  session id. The wakes still waiting in the chain are dropped.
- Tokens per channel: the input, output and cache write tokens of all the channel's turns,
  of every role, cache reads not counted, against `tokenBudget` (1,000,000). A turn only
  counts once it has ended. A send, or a wake in a chain, that finds the sum at or over the
  budget pauses the channel with `token budget of 1000000 used: 1000312 tokens`.
- Agent hops: a hop is a turn that an agent's message woke. The turn that answers the user's
  message is not a hop. The server counts hops per channel since the last user message. The
  7th hop does not run: the channel is paused with `hop cap: 6 agent hops without a user
  message`, and the `turn` event of the turn before it is followed by a `pause` event. The wake
  that did not run, and the wakes behind it, are stored as holds.

A send to a paused channel is 409 with the reason in the error, except after the hop cap: that
pause is for agents, so a user message clears it, resets the hop count to 0, drops the held
wakes and runs. `POST /channels/{id}/continue` clears the pause. When the token sum is at or
over the budget it also sets `tokenBudget` to the sum plus 1,000,000; after a tool cap the next
turn simply gets a fresh 25. `continue` on a channel that is not paused, or has a turn running,
is 409.

Held wakes: when a wake cannot run because of a pause (the hop cap, or a token budget that ran
out in the middle of a chain), the server stores it and the wakes behind it as `hold`
rows of the `approval` table. `continue` claims the channel's pending holds once, resets the
hop count to 0 and runs them in order as a new chain, with `pause` `{"reason":""}` as the
chain starts. If the chain cannot start, the channel stays paused and keeps the holds. That
chain can reach the hop cap again, and then pauses again. Sending a user message resets the
hop count to 0 too, and drops the holds. Approving a gate does not. The holds are stored, so
they survive a restart; the hop count is not, so a chain that runs after a restart counts from 0.

The same pause holds work when the server cannot store an approval for a gated wake, or cannot
read the messages a turn stored to route them. The channel pauses with the error as the reason, and
nothing runs ungated. These wakes are held in memory only, since the database failed. `continue` stores the approval again, or does the routing read again from
the message before the turn's reply, and runs what that wakes. If it fails again the channel
stays paused.

The tool cap is counted in `channel.Runner`, so `reef chat` is capped too: it pauses the
channel, prints the reason and exits, because the interrupt ends the agent. Before each
line, `reef chat` runs the same pause and token budget check as send (`channel.CheckCaps`)
and prints the reason instead of running the line. The line `/continue` does what
`continue` does (`channel.Continue`). See [memory.md](memory.md#reef-chat). Hops do not
exist in `reef chat`: it wakes nobody.

## GitHub poll

`reef serve` polls GitHub for each feature that has a worktree, every `REEF_GH_INTERVAL`
(default 60s), once right away and then on that interval until the server closes: `gh pr
view feature/<slug> --json number,url,state,statusCheckRollup`, run in the project's
folder. `statusCheckRollup` items are either a `CheckRun` (`name`, `status`, `conclusion`,
`detailsUrl`, `completedAt`) or a `StatusContext` (`context`, `state`, `targetUrl`,
`startedAt`); both are read.

When the PR first appears, changes state (`open`, `merged`, `closed`) or a check run
finishes (a `CheckRun` whose `status` is `COMPLETED`, or a `StatusContext` whose `state` is
not `PENDING`/`EXPECTED`), the feature's most recent channel (the one with the newest
message across the feature, or, when it has none, its most recently created channel) gets
one message from author `reef`: `PR #12 open: <url>` or `<check name> succeeded`/`failed`.
A check run is identified by its name plus its `detailsUrl`/`targetUrl` (falling back to
`completedAt`/`startedAt`) as its run key; a rerun of the same check name, such as CI run
again on a later push, is a different run key and is noted again, and two runs of the same
name in one rollup (a matrix job, two workflows) are two rows, both noted. The state posted
is stored one row per `(feature, name, run key)` (migration 16
[`016_feature_github.sql`](../store/migrations/016_feature_github.sql), tables
`feature_github` and `feature_github_check`), so a restart does not repeat it and a second
poll with the same answer posts nothing. No `gh` on `PATH`: nothing is posted, logged once.
`gh` erroring (not logged in, no PR for the branch): nothing is posted, logged once per
feature for that cause; logged again once the cause changes.

A check run that finished failed also wakes the `developer` role in that channel, once per
run, also across restarts (`feature_github_check.woken`): the text names the PR and the
failed check, plus the last 50 lines of `gh run view <run id> --log-failed` when the run id
can be read from the `CheckRun`'s `detailsUrl` (`/actions/runs/<id>/`); no run id, no log. A
channel whose team has no `developer` role gets the note only. The wake runs as its own
chain, the way a retried turn does (decision 39), attributed to author `reef` rather than
`user`: no gate holds it, since no role sent it, but it counts as a hop like any
agent-originated wake, so a run of failing checks still pauses at the hop cap instead of
resetting the count every time; the channel's other caps (pause, token budget) also still
apply. A channel with a turn running, or paused (including by the hop cap), cannot take the
wake on that poll; the note still posts, the wake is retried on a later poll once the channel
is idle, and the busy cause is logged once, not every poll.

## Append-only

The API has no way to change or remove these rows.

- Messages and decisions: create and read only. `PUT`, `PATCH` and `DELETE` on
  `/messages/{id}` and `/decisions/{id}` return 405. The only way a message or decision
  goes away is a cascade from its channel or feature. Deleting a channel does not delete
  its decisions; they only lose `messageId`.
- Briefs: `PUT /features/{id}/brief` adds a version and never edits one. Old versions
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
- Each affected channel gets one message from author `reef`, naming the turns. For turns 2
  (developer) and 3 (tester) in one channel:

  `Turn 2 by developer, turn 3 by tester failed: server restarted. The worktree may be dirty; retry or roll back.`

- With one turn: `Turn 2 by developer failed: server restarted. The worktree may be dirty; retry or roll back.`
- The server logs `marked N running turns failed: server restarted`.
- A `running` turn in a channel whose `chat-<channel id>.lock` a live `reef chat` holds is
  left `running`, and that channel gets no message from `reef`. `reef chat` is a separate
  process, so a server restart does not kill its agent, and its turn can still finish.
  The count in the log does not include it. A chat that died leaves its lock free, so
  its turn is failed like any other.

An agent may have left half-done edits in the worktree, so the message tells the user to
check before retrying. A turn failed this way keeps its `wakeText`, so `POST /turns/{id}/retry`
can run it again (see [Channels live](#channels-live)).

A chain of wakes does not survive a restart: the wakes still waiting are lost, with the hop
count and the wakes held because the database failed, which are in memory. A pause is stored,
and so are the wakes it held, so the channel stays paused and `continue` runs them. A pending
approval is stored, so it is still pending after the restart and can be approved then.

## Try it

Start a fresh server, then run these in order. Ids and timestamps follow from an empty
data folder; your timestamps will differ.

```
reef serve --data /tmp/reef-demo
```

```
$ curl -s -X POST -H 'Content-Type: application/json' localhost:7700/projects -d '{"name":"shop","path":"/tmp"}'
{"id":1,"name":"shop","path":"/tmp","createdAt":"2026-10-13T09:00:00.123456Z"}

$ curl -s -X POST -H 'Content-Type: application/json' localhost:7700/features -d '{"projectId":1,"slug":"login","name":"Login"}'
{"id":1,"projectId":1,"slug":"login","name":"Login","goal":"","stage":"ideation","createdAt":"2026-10-13T09:00:01.123456Z"}

$ curl -s -X POST -H 'Content-Type: application/json' localhost:7700/features -d '{"projectId":1,"slug":"login","name":"Again"}'
{"error":"feature slug \"login\" is taken in its project: conflict"}

$ curl -s -X PUT -H 'Content-Type: application/json' localhost:7700/features/1/brief -d '{"body":"Email and password login","author":"user"}'
{"id":1,"featureId":1,"body":"Email and password login","author":"user","createdAt":"2026-10-13T09:00:02.123456Z"}

$ curl -s -X POST -H 'Content-Type: application/json' localhost:7700/channels -d '{"projectId":1,"featureId":1,"name":"build","backend":"claude"}'
{"id":1,"projectId":1,"featureId":1,"name":"build","role":"lead","backend":"claude","permissionMode":"","paused":"","tokenBudget":1000000,"team":"solo","createdAt":"2026-10-13T09:00:03.123456Z"}

$ curl -s -X POST -H 'Content-Type: application/json' localhost:7700/messages -d '{"channelId":1,"author":"user","body":"Start with the form"}'
{"id":1,"channelId":1,"parentId":null,"author":"user","body":"Start with the form","createdAt":"2026-10-13T09:00:04.123456Z"}

$ curl -s -X POST -H 'Content-Type: application/json' localhost:7700/decisions -d '{"featureId":1,"messageId":1,"body":"Use sessions, not JWT","why":"One server, no token refresh","author":"user"}'
{"id":1,"featureId":1,"messageId":1,"body":"Use sessions, not JWT","why":"One server, no token refresh","author":"user","createdAt":"2026-10-13T09:00:05.123456Z"}

$ curl -s -X POST -H 'Content-Type: application/json' localhost:7700/tasks -d '{"projectId":1,"featureId":1,"title":"Build the login form"}'
{"id":1,"projectId":1,"featureId":1,"title":"Build the login form","body":"","status":"todo","createdAt":"2026-10-13T09:00:06.123456Z","updatedAt":"2026-10-13T09:00:06.123456Z"}

$ curl -s -X POST -H 'Content-Type: application/json' localhost:7700/turns -d '{"channelId":1,"role":"developer","backend":"claude"}'
{"id":1,"channelId":1,"role":"developer","backend":"claude","sessionId":"","status":"running","error":"","usage":{"inputTokens":0,"outputTokens":0,"cacheReadTokens":0,"cacheWriteTokens":0},"startedAt":"2026-10-13T09:00:07.123456Z","endedAt":null,"wakeText":"","wakeParent":null,"retried":false,"checkpoint":null}

$ curl -s -X PATCH -H 'Content-Type: application/json' localhost:7700/turns/1 -d '{"status":"done","sessionId":"3c1296cc","usage":{"inputTokens":4,"outputTokens":123,"cacheReadTokens":22898,"cacheWriteTokens":0}}'
{"id":1,"channelId":1,"role":"developer","backend":"claude","sessionId":"3c1296cc","status":"done","error":"","usage":{"inputTokens":4,"outputTokens":123,"cacheReadTokens":22898,"cacheWriteTokens":0},"startedAt":"2026-10-13T09:00:07.123456Z","endedAt":"2026-10-13T09:00:08.123456Z","wakeText":"","wakeParent":null,"retried":false,"checkpoint":null}

$ curl -s -X PATCH -H 'Content-Type: application/json' localhost:7700/turns/1 -d '{"status":"failed"}'
{"error":"invalid: turn 1 already ended"}

$ curl -s "localhost:7700/messages?channel=1&after=1"
[]

$ curl -s -X DELETE -w ' %{http_code}\n' localhost:7700/messages/1
{"error":"method not allowed"}
 405
```

To see restart recovery: create a turn, stop the server with Ctrl-C, start it again. The
log shows `marked 1 running turns failed: server restarted`, `GET /turns/2` is `failed`, and
`GET /messages?channel=1` ends with the message from `reef`.
