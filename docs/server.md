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
logged in and on `PATH`. The new channel form picks a team from `GET /teams`. Each message
shows its author, replies are folded under their thread root with a count, and a thread
opens in a pane where you can reply. The running turn names its teammate. A wake held at a gate
shows Approve and Reject, and a pause from the hop cap shows in the cap banner.

Client routes such as `/features/3` and `/tasks` have the same paths as the REST API. In the Go server, `GET /` and `HEAD /` always get the app's `index.html`; other `GET` or `HEAD` requests get it when the `Accept` header includes `text/html`. Every other request goes to the API, so `curl /projects` and the app's own `fetch` calls (`Accept: application/json`) are unchanged. A file that exists in `web/dist`, such as `/assets/...`, is served as is. The Host and Content-Type checks guard the API only, not these files.

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
| 409 | A unique rule broke: a feature slug is taken in its project. A send to a channel with a turn running, to a channel paused by a cap other than the hop cap or to a channel open in a `reef chat`; `continue` on a channel that is not paused or has a turn running; an approval that is not pending, or one approved while a turn runs or the channel is paused |
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
migration 8 [`008_approval.sql`](../store/migrations/008_approval.sql) (`gate`, `approval`).
Migrations 4 and 5 add columns to `channel` (see [channel](#channel)).

### project

| Field | JSON | Notes |
| --- | --- | --- |
| `id` | number | |
| `name` | string | required |
| `path` | string | required; the repo folder, stored as an absolute path with symlinks resolved |
| `createdAt` | string | |

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

A named set of roles and the gates between them. A team has exactly one lead role. There
is no REST route to write a team: `reef team apply` does (see [memory.md](memory.md#teams)).
Migration 6 seeds the team `solo`, with one role, `lead` on `claude`, and points every
channel that exists at it.

| Field | JSON | Notes |
| --- | --- | --- |
| `name` | string | required; unique. Applying a name that exists replaces that team's roles and gates in one transaction |
| `roles` | array | [roles](#role), the lead first, then the others in the order of the file |
| `gates` | array | [gates](#gate); left out when the team lists none |

### role

| Field | JSON | Notes |
| --- | --- | --- |
| `name` | string | required; unique within the team; letters, digits, `_` and `-` only, and not `user`. It is the `author` of what the role posts, and the name an `@mention` uses |
| `lead` | boolean | exactly one role of a team is the lead. The lead answers a message that nobody else owns |
| `backend` | string | required; `claude` or `codex` |
| `permissionMode` | string | `""` is the backend default; claude: `default` or `acceptEdits`; codex: `read-only` or `workspace-write`. The same rule as a channel's `permissionMode` |
| `persona` | string | goes at the start of the role's first prompt |
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

An edge the team does not list uses the default: `architect` to `developer` and `developer`
to `tester` are on, every other edge is off.

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

Deleting the channel deletes its approvals.

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
a channel takes its messages, turns, tokens, proposals and approvals.

| Route | Filters and body | OK | Notable errors |
| --- | --- | --- | --- |
| `GET /health` | | 200 `{"status":"ok"}` | |
| `GET /projects` | | 200 | |
| `POST /projects` | `name`, `path` | 201 | 400 missing field, or a `path` that is not an absolute path to an existing folder |
| `GET /projects/{id}` | | 200 | 404 |
| `PUT /projects/{id}` | `name`, `path` | 200 | 400 (same `path` rule), 404 |
| `DELETE /projects/{id}` | | 204 | 404 |
| `GET /features` | `?project=ID` | 200 | 400 bad id |
| `POST /features` | `projectId`, `slug`, `name`, `goal`, `stage` | 201 | 400 unknown project, 409 slug taken |
| `GET /features/{id}` | | 200 | 404 |
| `PUT /features/{id}` | `name`, `goal`, `stage` | 200 | 400, 404 |
| `DELETE /features/{id}` | | 204 | 404 |
| `GET /features/{id}/brief` | | 200 latest version | 404 no feature, or no brief yet |
| `PUT /features/{id}/brief` | `body`, `author` | 201 new version | 400, 404 |
| `GET /features/{id}/brief/versions` | | 200 oldest first | 404 |
| `GET /features/{id}/notes/{role}` | | 200 | 404 no feature, or no notes for the role |
| `PUT /features/{id}/notes/{role}` | `body`; replaces the old notes | 200 | 404 no feature |
| `DELETE /features/{id}/notes/{role}` | | 204 | 404 no feature, or no notes for the role |
| `GET /teams` | | 200 all teams by name, each with its roles and gates; a role has no `mcpServers`, whose `args` and `url` may hold API keys | |
| `GET /teams/{name}` | | 200, without `mcpServers` as above | 404 |
| `GET /channels` | `?project=ID`, `?feature=ID` | 200 | 400 bad id |
| `POST /channels` | `projectId`, `featureId`, `name`, `team` (a team name; default `solo`), `backend` (`claude` or `codex`; default the team lead's), `permissionMode` (`""`; claude: `default` or `acceptEdits`; codex: `read-only` or `workspace-write`; default the lead's when `backend` is the lead's), `role` (empty or `lead`; the channel's `role` becomes the name of the team's lead role) | 201 | 400 unknown project, feature or team, feature of another project, bad backend or permission mode, a role other than `lead` |
| `GET /channels/{id}` | | 200 | 404 |
| `PUT /channels/{id}` | `name` only; project, feature, role, backend and permission mode stay as created | 200 | 400, 404 |
| `DELETE /channels/{id}` | stops the channel first if it is live | 204 | 404 |
| `POST /channels/{id}/send` | `text`, `parent` (optional: the id of a top-level message of this channel; the message is stored as a reply in its thread) | 202, no body | 400 empty text, an unknown field (`role` is not a field), a `parent` that is not a top-level message of this channel, or `/wrap` on a quick session, 404, 409 turn running, channel paused (a hop cap pause does not count), open elsewhere or stopped before its turn began, 503 |
| `GET /channels/{id}/events` | | 200 `text/event-stream` | 404 |
| `POST /channels/{id}/stop` | no body needed | 204, also when the channel is not live or does not exist | |
| `POST /channels/{id}/continue` | no body needed | 204, no body | 404, 409 not paused or a turn running |
| `GET /channels/{id}/approvals` | | 200 the channel's pending approvals, oldest first; `[]` when none | 404 |
| `POST /channels/{id}/approvals/{aid}` | `approve` (required: `true` runs the held wake, `false` drops it) | 204, no body | 400 no `approve`, 404 no such approval in this channel, 409 not pending, or approve while a turn runs, the channel is paused or 4 hops ran (it stays pending), 503 |
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
| `GET /turns` | `?channel=ID`, `?status=running\|done\|failed` | 200 | 400 bad id or status |
| `POST /turns` | `channelId`, `role`, `backend`, `sessionId` | 201 | 400 bad backend or unknown channel |
| `GET /turns/{id}` | | 200 | 404 |
| `PATCH /turns/{id}` | `status` (`done` or `failed`), `error`, `usage`, `sessionId`. With no `status`, only `sessionId` is set and the turn stays `running` | 200 | 400 bad status, negative usage, empty `sessionId` without a status, `error` or `usage` without a status, or already ended; 404 |
| `GET /decisions` | `?feature=ID` (required) | 200 | 400 no feature, 404 unknown feature |
| `POST /decisions` | `featureId`, `messageId`, `body`, `why`, `author` | 201 | 400 missing field, unknown feature, or a message of another feature |
| `GET /decisions/{id}` | | 200 | 404 |

A `POST` with no body (`stop`, `continue`, `wrap`, `approve`, `reject`) still needs
`Content-Type: application/json`, or the guard answers 415. These routes ignore the body,
so an empty one or `{}` both work. `send` and `promote` read a JSON body, and an empty one
is 400.

Create and update take the whole object from the table above. A field you leave out
takes its default. `PUT` replaces; there is no partial update except `PATCH /turns/{id}`.

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
  newest session id (see [memory.md](memory.md#reef-chat)).
- The lead stays alive between turns, until `POST /channels/{id}/stop` or the server stops. A
  specialist's agent is stopped, and its token deleted, when its turn ends. Its next wake
  resumes its session.
- The server holds `chat-<id>.lock` while the channel has a live session, so `reef chat
  --channel ID` fails with `channel 1 is open elsewhere: conflict`. A send to a channel a
  `reef chat` holds is 409 too.
- `stop` interrupts a running turn (stored `failed`, `interrupted`), drops the wakes still
  waiting, then stops every agent of the channel, deletes their tokens and releases the lock.
  It also works while an agent is still starting: the send that began that turn answers 409
  `channel 1 was stopped`, and no turn runs. A turn that fails for another reason also stops
  every agent of the channel; the next turn opens them again, each by its own session id.
- `DELETE /channels/{id}` does the same as `stop` before it deletes the channel.
- Server shutdown does the same for every channel before it stops listening.

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
paused, else 409 and the approval stays pending). An approve at 4 hops pauses the channel for
the hop cap, answers 409 and reopens the approval. With `{"approve":false}` it only marks the
approval `rejected`. A decided approval is 409. Pending approvals are stored, so they survive a
restart.

### Events

`GET /channels/{id}/events` is a Server-Sent Events stream. It works for a channel that
is not live: it waits. A `: keepalive` comment comes every 15 seconds. Four kinds of event:

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
proposal like a wrap-up, and approval makes brief version 1. A promote that fails (a taken
slug, a feature of another project) changes nothing.

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
  5th hop does not run: the channel is paused with `hop cap: 4 agent hops without a user
  message`, and the `turn` event of the turn before it is followed by a `pause` event. The wake
  that did not run, and the wakes behind it, are held in memory.

A send to a paused channel is 409 with the reason in the error, except after the hop cap: that
pause is for agents, so a user message clears it, resets the hop count to 0, drops the held
wakes and runs. `POST /channels/{id}/continue` clears the pause. When the token sum is at or over the budget it
also adds 1,000,000 to `tokenBudget`; after a tool cap the next turn simply gets a fresh
25. `continue` on a channel that is not paused, or has a turn running, is 409.

Held wakes: when a wake cannot run because of a pause (the hop cap, or a token budget that ran
out in the middle of a chain), the server keeps it and the wakes behind it. `continue` then
resets the hop count to 0 and runs them as a new chain, with `pause` `{"reason":""}` as the
chain starts. If the chain cannot start, the channel stays paused and keeps the wakes. That
chain can reach the hop cap again, and then pauses again. Sending a user message resets the
hop count to 0 too. Approving a gate does not. A held wake lives in
memory only: after a restart `continue` just clears the pause and runs nothing.

The tool cap is counted in `channel.Runner`, so `reef chat` is capped too: it pauses the
channel, prints the reason and exits, because the interrupt ends the agent. `reef chat
--channel ID` runs the same pause and token budget check as send (`channel.CheckCaps`): it
refuses a paused channel and pauses one over budget, exit 1 with the reason. Hops do not
exist in `reef chat`: it wakes nobody.

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
  and no other edit. Restart recovery keeps a session id set this way.

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
check before retrying.

A chain of wakes does not survive a restart: the wakes still waiting are lost, with the held
wakes of a pause and the hop count, which are in memory. A pause is stored, so the channel
stays paused; `continue` clears it and runs nothing. A pending approval is stored, so it
is still pending after the restart and can be approved then.

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
{"id":1,"channelId":1,"role":"developer","backend":"claude","sessionId":"","status":"running","error":"","usage":{"inputTokens":0,"outputTokens":0,"cacheReadTokens":0,"cacheWriteTokens":0},"startedAt":"2026-10-13T09:00:07.123456Z","endedAt":null}

$ curl -s -X PATCH -H 'Content-Type: application/json' localhost:7700/turns/1 -d '{"status":"done","sessionId":"3c1296cc","usage":{"inputTokens":4,"outputTokens":123,"cacheReadTokens":22898,"cacheWriteTokens":0}}'
{"id":1,"channelId":1,"role":"developer","backend":"claude","sessionId":"3c1296cc","status":"done","error":"","usage":{"inputTokens":4,"outputTokens":123,"cacheReadTokens":22898,"cacheWriteTokens":0},"startedAt":"2026-10-13T09:00:07.123456Z","endedAt":"2026-10-13T09:00:08.123456Z"}

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
