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

## Web UI

`reef serve` also serves the React app in [`web/`](../web). `web/dist` is embedded in the
binary with `go:embed` ([`web/embed.go`](../web/embed.go)). A fresh clone builds without
npm: `web/dist` holds only `.keep`, and a page load answers 503 `UI not built: run npm run
build in web/` until the app is built.

```
cd web && npm ci && npm run build
cd .. && go build ./cmd/reef
```

Client routes such as `/features/3` and `/tasks` have the same paths as the REST API. One
rule splits them, in the Go server and in the Vite dev proxy: a `GET` or `HEAD` whose
`Accept` header includes `text/html` gets the app's `index.html`. Every other request goes
to the API, so `curl /projects` and the app's own `fetch` calls (`Accept: application/json`)
are unchanged. A file that exists in `web/dist`, such as `/assets/...`, is served as is. The Host and Content-Type checks guard the API only, not these files.

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
| 409 | A unique rule broke: a feature slug is taken in its project. A send to a channel with a turn running, to a paused channel or to a channel open in a `reef chat`; `continue` on a channel that is not paused |
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
[`002_token.sql`](../store/migrations/002_token.sql) (`token`) and migration 3
[`003_proposal.sql`](../store/migrations/003_proposal.sql) (`proposal`).

### project

| Field | JSON | Notes |
| --- | --- | --- |
| `id` | number | |
| `name` | string | required |
| `path` | string | required; the repo folder |
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
| `featureId` | number? | set on create and never changes. `null` is a quick session. Must be a feature of the same project. Deleting the feature deletes the channel |
| `name` | string | required |
| `role` | string | the role the channel's agent plays. Empty when not set. For a channel from before this field existed, the role of its newest turn |
| `backend` | string | `claude` or `codex`, the agent that runs the channel. Empty when not set. For an older channel, the backend of its newest turn |
| `permissionMode` | string | passed to the backend as is when the channel's agent starts or resumes. Empty means the backend default |
| `paused` | string | why the channel is paused (see [Caps](#caps)). Empty when it is not paused. Set by the server only; a client cannot set it |
| `tokenBudget` | number | tokens the channel may use before it pauses. Starts at 1,000,000. Set by the server only |
| `createdAt` | string | |

### message

| Field | JSON | Notes |
| --- | --- | --- |
| `id` | number | |
| `channelId` | number | required |
| `author` | string | required; free text, for example `user` or a role |
| `body` | string | required |
| `createdAt` | string | |

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

### token

Lets one teammate's `reef mcp` act as one role in one channel. No REST route: `reef
token` and `reef chat` make them, `reef mcp` reads them, and `reef chat` deletes its own
when it ends. See [memory.md](memory.md#tokens).

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
a channel takes its messages, turns, tokens and proposals.

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
| `GET /channels` | `?project=ID`, `?feature=ID` | 200 | 400 bad id |
| `POST /channels` | `projectId`, `featureId`, `name`, `backend` (required: `claude` or `codex`), `permissionMode` (`""`; claude: `default` or `acceptEdits`; codex: `read-only` or `workspace-write`), `role` (default `lead`) | 201 | 400 unknown project or feature, feature of another project, bad backend or permission mode |
| `GET /channels/{id}` | | 200 | 404 |
| `PUT /channels/{id}` | `name` only; project, feature, role, backend and permission mode stay as created | 200 | 400, 404 |
| `DELETE /channels/{id}` | | 204 | 404 |
| `POST /channels/{id}/send` | `text` | 202, no body | 400 empty text, 404, 409 turn running, channel paused or open elsewhere, 503 |
| `GET /channels/{id}/events` | | 200 `text/event-stream` | 404 |
| `POST /channels/{id}/stop` | | 204, also when the channel is not live | |
| `POST /channels/{id}/continue` | | 204, no body | 404, 409 not paused |
| `POST /channels/{id}/wrap` | no body needed; a body is ignored | 202, no body | 404, 409 and 503 as send |
| `POST /channels/{id}/promote` | `slug` and `name` for a new feature, or `featureId` for an existing one; exactly one form | 202 the channel | 400 bad body, channel already has a feature, feature of another project; 404; 409 slug taken, or as send; 503 |
| `GET /channels/{id}/proposal` | | 200 the channel's pending proposal | 404 none pending |
| `POST /proposals/{id}/approve` | | 200 the proposal and `decisionsAdded` | 400 already decided, 404 |
| `POST /proposals/{id}/reject` | | 200 the proposal | 400 already decided, 404 |
| `GET /messages` | `?channel=ID` (required), `?after=ID` (only newer ids) | 200 | 400 no channel, 404 unknown channel |
| `POST /messages` | `channelId`, `author`, `body` | 201 | 400 unknown channel or missing field |
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

Create and update take the whole object from the table above. A field you leave out
takes its default. `PUT` replaces; there is no partial update except `PATCH /turns/{id}`.

## Channels live

`POST /channels/{id}/send` runs one turn of the channel's agent through
[`channel.Open`](../channel/session.go), the code `reef chat` uses.

- The agent starts at the first send and stays alive between turns, until `POST
  /channels/{id}/stop` or the server stops. A send after that resumes the agent by the
  newest session id (see [memory.md](memory.md#reef-chat)).
- The server holds `chat-<id>.lock` while the agent is alive, so `reef chat --channel ID`
  fails with `channel 1 is open elsewhere: conflict`. A send to a channel a `reef chat`
  holds is 409 too.
- The turn runs in the background, not in the request, and send answers 202 at once. A
  send while a turn runs is 409. The text `/wrap` runs the wrap-up turn.
- `stop` interrupts a running turn (stored `failed`, `interrupted`), then stops the
  agent, deletes its token and releases the lock. A turn that fails for another reason
  also stops the agent; the next send starts it again.
- Server shutdown does the same for every channel before it stops listening.

`GET /channels/{id}/events` is a Server-Sent Events stream. It works for a channel that
is not live: it waits. A `: keepalive` comment comes every 15 seconds. Three kinds of event:

- `turn`: `{"status":"running"}` when a turn starts, then `{"status":"done"}` or
  `{"status":"failed","error":"interrupted"}` when it ends.
- `agent`: one [agent event](agents.md) as JSON, as it comes.
- `pause`: `{"reason":"..."}` when a cap pauses the channel: right after the `turn`
  event of a capped turn, or when a send finds the token budget used. `{"reason":""}`
  when `continue` clears the pause. These come between turns, so only a connected
  subscriber sees them. For the state on load, read `paused` from `GET /channels/{id}`.

The server keeps the events of the running turn in memory. A subscriber that connects
during a turn first gets `turn` running and every `agent` event of that turn so far, then
the live ones. One that connects between turns gets nothing until the next turn starts:
the stored messages (`GET /messages`) are the history. A slow subscriber only lags; it
never holds up the agent.

```
event: turn
data: {"status":"running"}

event: agent
data: {"kind":"tool_call","toolId":"t1","toolName":"Bash","input":{"command":"ls"}}

event: agent
data: {"kind":"text","text":"done"}

event: turn
data: {"status":"done"}
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
promoted. The server checks that no turn runs and the caps, as for send, then in one
transaction creates the feature and sets the channel's `featureId`. It answers 202 with the
channel and starts a turn: the stored user message is `/promote`, and the agent gets a
prompt that names the feature and asks it to call `propose_wrap` once with the first brief,
written from the session, its notes and the decisions so far. The user approves that
proposal like a wrap-up, and approval makes brief version 1. A promote that fails (a taken
slug, a feature of another project) changes nothing.

## Caps

Two caps pause a channel. Both are constants in the code, not settings.

- Tool calls per wake: one turn (one user message) may make 25 tool calls, subagent calls
  included. The 26th interrupts the agent, ends the turn `failed` with the error `capped:
  more than 25 tool calls` and pauses the channel with that text as `paused`. The agent
  stops like after a failed turn; the next send resumes it by session id.
- Tokens per channel: the input plus output tokens of all the channel's turns, cache reads
  and writes not counted, against `tokenBudget` (1,000,000). A turn only counts once it
  has ended. A send that finds the sum at or over the budget pauses the channel with
  `token budget of 1000000 used: 1000312 tokens`.

A send to a paused channel is 409 with the reason in the error. `POST
/channels/{id}/continue` clears the pause. When the token sum is at or over the budget it
also adds 1,000,000 to `tokenBudget`; after a tool cap the next turn simply gets a fresh
25. `continue` on a channel that is not paused is 409.

The tool cap is counted in `channel.Runner`, so `reef chat` is capped too: it pauses the
channel, prints the reason and exits, because the interrupt ends the agent. Resume with
`reef chat --channel ID`. `reef chat` does not check the pause or the token budget.

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
{"id":1,"projectId":1,"featureId":1,"name":"build","role":"lead","backend":"claude","permissionMode":"","createdAt":"2026-10-13T09:00:03.123456Z"}

$ curl -s -X POST -H 'Content-Type: application/json' localhost:7700/messages -d '{"channelId":1,"author":"user","body":"Start with the form"}'
{"id":1,"channelId":1,"author":"user","body":"Start with the form","createdAt":"2026-10-13T09:00:04.123456Z"}

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
