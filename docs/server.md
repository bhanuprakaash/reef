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
- One server owns a data folder. `reef serve` locks `reef.lock` for as long as it runs. A
  second `reef serve` on the same `--data` exits with `data folder ... is in use by another
  reef`, before it touches the data, whatever its `--addr`. A second instance needs its own
  `--data` and its own `--addr`.
- `--addr` is where to listen. The default is `127.0.0.1:7700`. The API has no login, so
  keep it on localhost. It does refuse a request whose `Host` is not `127.0.0.1`,
  `localhost` or `::1` (403), and a `POST`, `PUT` or `PATCH` whose `Content-Type` is not
  `application/json` (415). This stops a web page on another origin from writing to it,
  including through DNS rebinding.
- The file opens in WAL mode with foreign keys on. Migrations in
  [`store/migrations`](../store/migrations) run at start, in order, each once. They only
  go forward. A file whose version is newer than the newest migration (made by a newer
  reef) is refused: `database is newer than this reef`.
- SIGINT or SIGTERM stops the server, with one second for requests in flight.

## Errors

Every error has a JSON body: `{"error": "feature slug \"login\" is taken in its project: conflict"}`.

| Status | When |
| --- | --- |
| 400 | Bad JSON or data after the JSON body, a bad or missing field, a non-number id, an unknown parent id inside a body (`unknown project 9`) |
| 403 | The `Host` is not `127.0.0.1`, `localhost` or `::1`. Body is `{"error":"forbidden host"}` |
| 404 | The id in the path (or a required query filter) matches nothing, or no route knows the path. Body for an unknown path is `{"error":"not found"}` |
| 405 | The path exists, the method does not. Body is `{"error":"method not allowed"}` |
| 409 | A unique rule broke: a feature slug is taken in its project |
| 415 | A `POST`, `PUT` or `PATCH` without `Content-Type: application/json`. Charset parameters are fine |
| 500 | Anything else. Body is `{"error":"internal error"}`; the detail goes to the log |

An id in the path that matches nothing is 404; an id in the body that matches nothing is
400. Unknown JSON fields in a request are ignored, and so are the server-set fields
(`id`, `createdAt`, `updatedAt`).

## Entities

Ids are integers. Instants (`createdAt`, `startedAt`, ...) are RFC 3339 UTC strings.
A type with `?` may be `null`. The headings below are the SQL tables in
[`001_init.sql`](../store/migrations/001_init.sql).

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
| `slug` | string | required; lowercase letters, digits and `-`, not starting with `-` (`^[a-z0-9][a-z0-9-]*$`), because it becomes a branch and folder name; unique within the project (409 if taken) |
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
| `sessionId` | string | default `""`; set on create or when the turn ends |
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

## REST

`{id}` is a number. Lists come back as `[]` when empty, ordered by id (oldest first).
Create returns 201 and the row. Update returns 200 and the row. Delete returns 204 and no
body. Deleting a row deletes what hangs off it by foreign key: a project takes its
features, channels and tasks; a feature takes its briefs, notes, decisions and channels;
a channel takes its messages and turns.

| Route | Filters and body | OK | Notable errors |
| --- | --- | --- | --- |
| `GET /health` | | 200 `{"status":"ok"}` | |
| `GET /projects` | | 200 | |
| `POST /projects` | `name`, `path` | 201 | 400 missing field |
| `GET /projects/{id}` | | 200 | 404 |
| `PUT /projects/{id}` | `name`, `path` | 200 | 400, 404 |
| `DELETE /projects/{id}` | | 204 | 404 |
| `GET /features` | `?project=ID` | 200 | 400 bad id |
| `POST /features` | `projectId`, `slug`, `name`, `goal`, `stage` | 201 | 400 unknown project, 409 slug taken |
| `GET /features/{id}` | | 200 | 404 |
| `PUT /features/{id}` | `slug`, `name`, `goal`, `stage` | 200 | 400, 404, 409 slug taken |
| `DELETE /features/{id}` | | 204 | 404 |
| `GET /features/{id}/brief` | | 200 latest version | 404 no feature, or no brief yet |
| `PUT /features/{id}/brief` | `body`, `author` | 201 new version | 400, 404 |
| `GET /features/{id}/brief/versions` | | 200 oldest first | 404 |
| `GET /features/{id}/notes/{role}` | | 200 | 404 no feature, or no notes for the role |
| `PUT /features/{id}/notes/{role}` | `body`; replaces the old notes | 200 | 404 no feature |
| `DELETE /features/{id}/notes/{role}` | | 204 | 404 no feature, or no notes for the role |
| `GET /channels` | `?project=ID`, `?feature=ID` | 200 | 400 bad id |
| `POST /channels` | `projectId`, `featureId`, `name` | 201 | 400 unknown project or feature, feature of another project |
| `GET /channels/{id}` | | 200 | 404 |
| `PUT /channels/{id}` | `name` only; project and feature stay as created | 200 | 400, 404 |
| `DELETE /channels/{id}` | | 204 | 404 |
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
| `PATCH /turns/{id}` | `status` (`done` or `failed`), `error`, `usage`, `sessionId` | 200 | 400 bad status or already ended, 404 |
| `GET /decisions` | `?feature=ID` (required) | 200 | 400 no feature, 404 unknown feature |
| `POST /decisions` | `featureId`, `messageId`, `body`, `why`, `author` | 201 | 400 missing field, unknown feature, or a message of another feature |
| `GET /decisions/{id}` | | 200 | 404 |

Create and update take the whole object from the table above. A field you leave out
takes its default. `PUT` replaces; there is no partial update except `PATCH /turns/{id}`.

## Append-only

The API has no way to change or remove these rows.

- Messages and decisions: create and read only. `PUT`, `PATCH` and `DELETE` on
  `/messages/{id}` and `/decisions/{id}` return 405. The only way a message or decision
  goes away is a cascade from its channel or feature. Deleting a channel does not delete
  its decisions; they only lose `messageId`.
- Briefs: `PUT /features/{id}/brief` adds a version and never edits one. Old versions
  stay in `/brief/versions`. Role notes are not versioned; `PUT` replaces them.
- Turns: created `running`, ended once with `PATCH` to `done` or `failed`. A second
  `PATCH` returns 400. A turn has no delete route and no other edit.

## Restart recovery

A turn is `running` only while a server owns it. If the server stops, the agent process
dies with it, so a `running` turn in the file can no longer finish. At start, once it
holds the data folder and has bound `--addr` but before it serves requests, `reef serve`
fails them all. A second `reef serve` never gets this far on a folder in use, so it cannot
fail the turns of the server that owns it:

- Each `running` turn becomes `failed`, with `error` `server restarted` and `endedAt` set.
- Each affected channel gets one message from author `reef`, naming the turns. For turns 2
  (developer) and 3 (tester) in one channel:

  `Turn 2 by developer, turn 3 by tester failed: server restarted. The worktree may be dirty; retry or roll back.`

- With one turn: `Turn 2 by developer failed: server restarted. The worktree may be dirty; retry or roll back.`
- The server logs `marked N running turns failed: server restarted`.

An agent may have left half-done edits in the worktree, so the message tells the user to
check before retrying.

## Try it

Start a fresh server, then run these in order. Ids and timestamps follow from an empty
data folder; your timestamps will differ.

```
reef serve --data /tmp/reef-demo
```

```
$ curl -s -X POST -H 'Content-Type: application/json' localhost:7700/projects -d '{"name":"shop","path":"/tmp/shop"}'
{"id":1,"name":"shop","path":"/tmp/shop","createdAt":"2026-10-13T09:00:00.123456Z"}

$ curl -s -X POST -H 'Content-Type: application/json' localhost:7700/features -d '{"projectId":1,"slug":"login","name":"Login"}'
{"id":1,"projectId":1,"slug":"login","name":"Login","goal":"","stage":"ideation","createdAt":"2026-10-13T09:00:01.123456Z"}

$ curl -s -X POST -H 'Content-Type: application/json' localhost:7700/features -d '{"projectId":1,"slug":"login","name":"Again"}'
{"error":"feature slug \"login\" is taken in its project: conflict"}

$ curl -s -X PUT -H 'Content-Type: application/json' localhost:7700/features/1/brief -d '{"body":"Email and password login","author":"user"}'
{"id":1,"featureId":1,"body":"Email and password login","author":"user","createdAt":"2026-10-13T09:00:02.123456Z"}

$ curl -s -X POST -H 'Content-Type: application/json' localhost:7700/channels -d '{"projectId":1,"featureId":1,"name":"build"}'
{"id":1,"projectId":1,"featureId":1,"name":"build","createdAt":"2026-10-13T09:00:03.123456Z"}

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
