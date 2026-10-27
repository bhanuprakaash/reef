# Memory

The memory loop gives every session on a feature the same starting point, and saves what
the session learned when it ends. It has four parts: the `reef` CLI
([`cmd/reef/cli.go`](../cmd/reef/cli.go)), the MCP server `reef mcp`
([`server/mcp.go`](../server/mcp.go)), the context prompt
([`channel/context.go`](../channel/context.go)) and `reef chat`
([`cmd/reef/chat.go`](../cmd/reef/chat.go), [`channel/runner.go`](../channel/runner.go)).
The code is the source of truth. This doc says what it does and why.

## Why

Without Reef, a big feature is split over many sessions. The agent writes a handoff file
so that the next session can continue, and the history of why things were done gets lost.
Reef has no handoff file. A new session starts with three things:

- The feature's brief: goal, status, key decisions, open questions.
- Memory items: every live decision of the feature and the newest findings, project, team and
  teammate items, each part with a count of how many exist.
- An index: one line per task in progress, and a count of the other open tasks and of the earlier
  channels.

It never gets old message bodies. When it needs details, it fetches them on demand with
the MCP tools (`recall`, `read_memory`, `read_thread`, `list_tasks`, `search_sessions`). So the first prompt
stays small after many sessions. At the end of a session, the agent proposes a new brief
and the memory items it did not save yet. The user approves, and the next
session starts from that.

## Processes and the database

All parts share one SQLite file, `reef.db`, in the data folder (see
[server.md](server.md#run)). Several processes open it at the same time:

- `reef serve`: the REST server. It locks `reef.lock` for as long as it runs, so only one
  server uses a data folder.
- CLI commands: each one opens the file, does one thing and exits.
- `reef mcp`: one process per teammate. The agent (Claude Code or Codex) starts it as a
  stdio MCP server.
- `reef chat`: one process per open channel. It starts the agent, and the agent starts
  its own `reef mcp`.

Only `reef serve` takes `reef.lock`. The CLI, `reef mcp` and `reef chat` open `reef.db`
directly, with or without a running server. WAL mode and a 5 second busy timeout let the
processes share the file.

`reef chat` also locks `chat-<channel id>.lock` in the data folder while it runs, and so
does `reef serve` for a channel it runs. A second `reef chat` on the same channel exits
with `channel 1 is open elsewhere: conflict`. When `reef serve` starts, it fails the
turns that a dead server left running (see [server.md](server.md#restart-recovery)). It
skips a channel whose chat lock is held, because that turn is still live in a `reef chat`
process.

## CLI

```
reef <command> [args] [--data DIR]
```

Rules for every command:

- `--data DIR` is the data folder, the same as for `reef serve`. Flags can come before or
  after the other arguments.
- `--` ends the flags: every argument after it is positional, even if it starts with `-`.
- `<feature>` is a feature id (all digits) or a slug. A slug cannot be all digits, so a
  ref is never both. If two projects have a feature with that slug, the command fails with
  `invalid: feature slug "login" is in projects 1, 3; use the feature id`.
- Commands that need a project and get no feature use the project of the current folder:
  the project whose path is the current folder or its nearest parent. If there is none:
  `no reef project for /tmp; run reef project add`.
- Output is plain text, one row per line, id first, fields split by two spaces.
- An error goes to stderr as `reef: <message>`, with exit code 1. Bad arguments print the
  command's usage line.
- The CLI writes as author `user`.

| Command | Does | Prints |
| --- | --- | --- |
| `reef serve [--addr 127.0.0.1:7700]` | Runs the REST server. See [server.md](server.md) | log lines on stderr |
| `reef project add [--name N]` | Makes the current folder a project. The name is the folder name if you give none. Fails if the folder is already a project: `/tmp/shop is already project 1` | `1  shop  /tmp/shop` |
| `reef feature add <slug> <name> [--goal G]` | Adds a feature to the current folder's project. Slug rules are in [server.md](server.md#feature) | `1  login  Email login` |
| `reef brief <feature>` | Prints the latest brief. With none: `feature login has no brief yet` | the brief body |
| `reef decide <feature> "<body>" [--why W]` | Writes a `decision` item in the feature's `work_item` scope as author and role `user`, so it is `accepted`. After the item, the lookalike and vector lines of `reef memory add` | `1  Use sessions, not JWT`: id, body; then `3  similar  <body>` |
| `reef task add [<feature>] "<title>"` | Adds a `todo` task. With a feature, the task is in the feature's project and tagged with it. Without one, it is in the current folder's project, with no feature | `1  todo  Build the login form` |
| `reef task list [<feature>] [--status S]` | Lists the feature's tasks, or all tasks of the current folder's project. `S` is `todo`, `doing` or `done` | one task per line, as for `task add` |
| `reef memory add <kind> "<body>" --scope S [--as ROLE] [--about X]... [--replaces ID]... [--why W] [--needs-approval]` | Writes a memory item as author `user`, so it is `accepted`; `--needs-approval` makes it a `candidate` that waits in `pending`. `S` and each `X` are `kind:id`, such as `project:1` or `work_item:12`; `--about` and `--replaces` repeat. Each `--replaces` item must be accepted (a candidate is approved or rejected first) and in the same scope as the new one; it is retired and points at the new one, or, with `--needs-approval`, when the new one is approved. `--as` sets the role the item carries. After the item, one line per stored item that looks like it. When an embedding provider is set and the vector could not be stored, stderr gets `vector not stored: <reason>; run reef memory embed <provider> to retry` and the exit code stays 0; the reason names the status and the vendor's error code, never the key | `12  accepted  decision  project:1  Use sessions, not JWT`: id, status, kind, scope, body; then `3  similar  <body>` |
| `reef memory recall "<q>" [--scope S] [--limit N]` | Searches the items of every scope, retired ones included, plus the items one link away from the best hits. `--scope` narrows to one shelf; `--limit` defaults to 10 | one hit per line: `12  accepted  decision  project:1  direct  Use sessions, not JWT`: id, status, kind, scope, `via`, body. `via` is `direct` or the link it was reached through, such as `replaces memory:4` |
| `reef memory show <id>` | Prints one item with its details | the item row as for `add`, then these lines when they apply: `why  <why>`, `author  user  <role>` (the role is left out when empty), one `link  <rel>  <kind>:<id>` per link, `replaced_by  <id>`, `retired_at  <RFC 3339 time>` |
| `reef memory retire <id>` | Retires an accepted item with nothing replacing it. Refuses a candidate (`invalid: memory 3 is a candidate`; use `reject`), a rejected item and a retired one (`invalid: memory 3 is already retired`) | `12  retired` |
| `reef memory pending` | Lists the candidates waiting for approval | one item row per line, as for `add`; nothing when none |
| `reef memory reject <id>` | Turns down a candidate: it is kept with status `rejected` and `retired_at` set, and no search or recall finds it. An item the candidate would have replaced stays live. Any other item fails with `invalid: memory 3 is not a candidate` | `12  rejected` |
| `reef memory approve <id>` | Moves a candidate to `accepted` and retires each item it replaces (one already retired is skipped). Any other item fails with `invalid: memory 3 is not a candidate` | `12  accepted` |
| `reef memory embed` | Prints the embedding provider: `none`, `openai`, `voyage` or `gemini`. The key is stored in `reef.db` and never printed | `openai`; `none` when unset |
| `reef memory embed <provider> [--key K]` | Sets the embedding provider and stores its key in `reef.db`; the key is never printed. Storing a key makes `reef.db` and its `-wal` and `-shm` files readable only by the owner (mode 0600). `openai`, `voyage` and `gemini` need `--key` unless the stored provider is the same one, else `invalid: embedding provider voyage needs --key`; `none` clears the key. Changing the provider clears every stored vector. Each `openai`, `voyage` or `gemini` call then embeds every item that has no vector; a failure there exits 1 after the setting is saved, and running the command again retries | `embedding provider openai; embedded 12 items` (`embedded 0 items` when none was missing), or `embedding provider none` |
| `reef team apply FILE.json` | Creates a team from a file, or replaces the roles and gates of the team with that name (see Teams) | `applied team duo: lead*(claude) architect(claude)` |
| `reef team list` | Lists the teams by name, each with its roles | `duo  lead*(claude) architect(claude)`: the lead has a `*` |
| `reef context <feature> --role R` | Prints the context prompt for role `R` (see below) | the prompt |
| `reef token <feature> --role R` | Makes a channel and a token for role `R` (see Tokens) | a `claude mcp add` command |
| `reef wrap list [<feature>]` | Lists pending wrap-up proposals (see Wrap-up): of the feature, or of the current folder's project | `1  login  3  architect  wrap  2026-10-13`: id, feature slug, channel id, role, kind (`wrap` or `brief`), created date |
| `reef wrap show <id>` | Prints a proposal and its status | `proposal 1 from architect` (`brief update 2 from lead` for a brief proposal), status, then brief, items (kind and scope each) and retire ids |
| `reef wrap approve <id>` | Approves a pending proposal | `approved: brief and 2 items saved`, or `approved: brief saved` for a brief proposal |
| `reef wrap reject <id>` | Rejects a pending proposal | `rejected: nothing changed` |
| `reef chat ...` | Runs a channel in the terminal (see below) | the conversation |
| `reef mcp` | Serves the MCP tools over stdio (see below) | MCP only |

## Teams

A team is a set of roles, one of them the lead, and the gates between them. It is a JSON
file. `reef team apply FILE.json` stores it, and a channel picks it by name (`team` in `POST
/channels`, see [server.md](server.md#team)). Migration 6 seeds a team `solo`, with one role,
`lead` on `claude`, and every channel without a team is in it. A new channel created by
`reef chat` is in `solo`. When resuming, `reef chat` allows teams with one role and refuses
teams with multiple roles; use the Reef window (`reef serve`) for those teams.

`teams/full.json` in the repo is a four-role team: `lead`, `architect`, `developer` (the only
role with `write`) and `tester`, all on `claude`, with the default gates. Each persona names
the one teammate it hands work to (lead to architect, architect to developer, developer to
tester, tester to developer on a failure or else to lead) and says to hand off by starting a
line with `@name` and to write no other `@name`, because only an `@name` at the start of a
line wakes a teammate (decision 80). The tester runs `go test`, `npm test` and
read-only `git` commands. Under decision 83 `go test` and `npm test` make it a writer, with
the single-writer lock and a checkpoint. Apply it with `reef team apply teams/full.json`.

```json
{
  "name": "duo",
  "roles": [
    {"name": "lead", "lead": true, "backend": "claude",
     "persona": "You are the lead. Answer the user. Hand design work over: start a line with @architect."},
    {"name": "architect", "backend": "claude", "permissionMode": "default",
     "persona": "You are the architect. Reply with a short plan.",
     "skills": ["writing-plans"],
     "tools": ["Read", "Grep"],
     "mcpServers": [{"command": "my-mcp", "args": ["--stdio"]}]}
  ],
  "gates": [{"from": "lead", "to": "architect", "on": true}]
}
```

- `name` and `roles` are required. A role needs `name` and `backend` (`claude` or `codex`). The
  other role fields are optional: `lead` (default `false`), `write` (default `false`), `permissionMode`, `persona`,
  `skills`, `face`, `tools` and `mcpServers`. What each does is in [server.md](server.md#role).
  Exactly one role has `"lead": true`. A role name can appear once, uses only letters, digits,
  `_` and `-` (so an `@mention` can name it), and is not `user`.
- `gates` is optional. A gate `{"from","to","on"}` names two roles of the team. An edge the
  file does not list uses the default: on for `architect` to `developer` and `developer` to
  `tester`, off for the rest (see [server.md](server.md#gate)).
- The file is read strictly: an unknown field is an error.
- Applying a name that exists replaces its roles and gates in one transaction, and the
  team keeps its channels. One failed check changes nothing. A new persona or skills
  reaches the role's next wake, whether that starts a session or resumes one (see below).
  A role applied with a new `backend` starts a new session; it does not resume the old one.
- Errors print `reef: <message>` and exit 1: `reef: invalid: team bad needs exactly one
  lead role, has 0` (or `has 2`), `reef: role a: invalid: backend must be claude or codex`,
  `reef: invalid: role name "two words" must be letters, digits, _ or -, and not user or reef`,
  `reef: invalid: gate a to ghost names a role team bad does not have`,
  `reef: invalid: role tester has no write, so it cannot have unrestricted Bash; list command
  patterns such as Bash(go test:*)`, `reef: parse
  team.json: json: unknown field "color"`.
- `tools` are the backend's allowed tools: Claude runs them without asking (`--allowedTools`),
  and so it does the reef tools (`mcp__reef__*`), but not the tools of the role's own `mcpServers`
  unless `tools` lists them (`mcp__mcp1__*`).
  A claude role that picks skills (`skills`) may use only those: each is allowed as `Skill(name)` and every
  other claude skill on disk is denied. A role that picks none keeps every skill. A codex role gets the
  names in its prompt only.
  A claude role without `write` cannot list plain `Bash` (or `Bash(*)`): `reef team apply`
  rejects it, so list patterns such as `Bash(go test:*)`.
  Codex ignores `tools`. Its limit is `permissionMode`. `write` decides which limit applies
  (see [server.md](server.md#role)).

## Tokens

A token says who a `reef mcp` process works for: one role in one channel. The channel
gives the feature and the project. The role is the author and role of the items the
agent saves with `remember`, the owner of the `teammate` scope it reads, the author of what
`post_to_channel` posts, and the role on a wrap-up proposal. A role of a channel in `reef serve` comes from its
team. For `reef token` and `reef chat` it is free text.

- A token is 64 hex characters (32 random bytes). It is stored in the `token` table (see
  [server.md](server.md#token)).
- `reef token` makes one, in a new channel named `<role> (claude code)`. Every start of
  `reef chat`, new or resumed, makes one for its channel, and deletes it when the chat
  ends. The tokens of `reef token` stay.
- `reef serve` makes one for each teammate session, when that teammate's agent starts, and
  deletes it when the agent stops. A specialist's agent stops after each wake, so its
  token lives for that wake. The lead's lives until the channel stops. A channel with a lead
  and an architect therefore has one token while only the lead runs, and two during an
  architect wake.
- `reef token` stores the channel role as `lead` for now, even when `--role` names another role.
- A token never expires. It goes away when its channel is deleted, or, for a `reef chat`
  token, when the chat ends.
- `reef mcp` reads the token from the `REEF_TOKEN` environment variable, not from a flag,
  so it does not show in the process list.

## reef mcp

```
REEF_TOKEN=<token> reef mcp [--data DIR]
```

It serves Reef's MCP tools over stdio, with the official Go SDK
(`github.com/modelcontextprotocol/go-sdk`). Only MCP goes to stdout. It refuses to start,
with exit code 1, when `REEF_TOKEN` is empty (`REEF_TOKEN is not set`) or matches no
token (`unknown REEF_TOKEN`). It stops when stdin closes.

Every tool except `post_to_channel`, `send_to_role`, `read_direct_messages`, `recall` and `read_memory` works on the token's feature, and so does `remember` unless its scope is `project` or `teammate`. On a quick-session channel
(no feature) those tools fail with `this channel has no feature`, except `read_thread` of the
token's own channel. A result is one JSON object as text. A
tool error comes back as a result with `isError: true` and the message as text, so the
agent can read it. Arguments that do not match the tool's schema are a tool error too
(`validating "arguments": ... missing properties: ["query"]`). An unknown tool name is a
JSON-RPC error (`-32602`, `unknown tool "nope"`).

| Tool | Input | Output | Errors |
| --- | --- | --- | --- |
| `get_brief` | none | `{"brief": "..."}`: the latest brief, `""` when there is none yet. The role's own items are in scope `teammate`: read them with `recall` | |
| `remember` | `kind` (`decision`, `convention`, `finding`, `fact` or `lesson`) and `body` (required), `about` (strings `kind:id`), `replaces` (ids of accepted items it amends or replaces), `why`, `scope` (`work_item`, the default, `project` or `teammate`), `needsApproval` | `{"item": {...}, "similar": [{"id", "body"}], "embedError": "..."}`: the stored item with `id`, `kind`, `body`, `why`, `scope` (`work_item:1`), `author` and `role` (both the token's role), `status`, `retiredAt`, `replacedBy`, `createdAt` and `links` (`{"rel", "to"}`: the item's `source` is `channel:<id>`); `similar` appears only when stored items look like it, and `embedError` only when no vector could be stored for it. `work_item` is the token's feature, `project` its project, `teammate` the token's role on every feature. The item is `accepted`, or a `candidate` that waits for the user when `needsApproval` is set or the kind is `convention` from a role other than `architect` or `lead`. Each `replaces` item must be an accepted item of the same scope as the new one; it is retired and points at the new one, or when the new one is a candidate, after the user approves it. `team` is never written from here: team items come only from the lead's wrap-up | `invalid: scope "team" is not work_item, project or teammate`, `this channel has no feature` (`work_item` on a quick session), `invalid: unknown kind "x"`, `invalid: body is required`, `memory 9: not found` (a `replaces` id that is not readable here), `invalid: memory 3 is in scope project:1, not work_item:1` (a `replaces` id on another shelf) (nothing is written) |
| `recall` | `query` (required), `limit` (default 10), `scope` (`work_item`, `project`, `team` or `teammate`: only that one) | `{"hits": [{"id", "kind", "body", "scope", "status", "via"}]}`: matches in every scope the token can read, retired items included and marked, plus items one link away; `via` is `direct` or the link it came through. A feature's `work_item` items are read only on that feature; `team` only on a channel with a team. `hits` is `[]` when nothing matches | `query is required`, `invalid: you have no "team" scope here` |
| `read_memory` | `id` (required) | the item as `remember` returns it, with its `links` | `memory 99: not found`, also for an item in a scope the token cannot read |
| `add_task` | `title` (required), `body` | the new task: `todo`, in the feature's project, tagged with the feature | `invalid: task title is required` |
| `list_tasks` | `status` (`todo`, `doing` or `done`; leave it out for all) | `{"tasks": [...]}`: the feature's tasks by id, as in [server.md](server.md#task). `tasks` is `[]` when there are none | `invalid: unknown status "later"` |
| `read_task` | `id` (required) | the task, with its full body | `task 99: not found`, also for a task of another feature or of no feature |
| `update_task` | `id` (required), `title`, `body`, `status` (`todo`, `doing` or `done`) | the updated task. Only the fields given change; `{"id": 3, "status": "done"}` marks task 3 done. There is no tool to delete a task | `task 99: not found` as for `read_task`, `invalid: task title is required`, `invalid: unknown status "later"` |
| `search_sessions` | `query` (required), `limit` (default 20) | `{"hits": [{"channelId", "messageId", "kind", "author", "snippet"}]}`: messages in all of the feature's channels that contain `query`, newest first. `kind` is `public` or `direct`. A direct hit appears only when the token's role is its sender or recipient, in any channel of the feature, and also has `from`, `to` and `chainId`; its `messageId` is the direct message id, which `read_thread` does not take. Case is ignored for ASCII. The snippet is about 120 characters around the match. `hits` is `null` when nothing matches | `query is required` |
| `read_thread` | `channelId` (required), `after` (only newer message ids), `root` (the id of a top-level message) | `{"messages": [...]}`: the channel's messages, oldest first. With `root`: that message and its replies, oldest first, and `after` is ignored. Each message has `parentId`, `null` at the top level. `messages` is `null` when there are none | `channel 9: not found`, also for a channel of another feature or a quick session other than the token's own; `thread 9: not found` when `root` is not a top-level message of the channel |
| `post_to_channel` | `body` (required), `parent` (the id of a top-level message: reply in its thread; leave it out to post at the top level) | the stored message, with the token's role as `author`. It works on a quick session too | `invalid: message body is required`, `invalid: parent 9 is not a top-level message of channel 1` |
| `send_to_role` | `to` (exactly one teammate role), `body` (required) | the stored direct request, with `channelId`, `chainId`, `from`, `to`, `replyToId: null` and `status: sent` | empty body, self, unknown or out-of-team role, a recipient list, a closed channel, a starting channel, or a wrapping or promoting channel are errors; nothing is stored |
| `read_direct_messages` | `after` (only newer direct-message ids) | `{"messages": [...]}` containing only exchanges in this channel where the token's role is sender or recipient | |
| `set_stage` | `stage` (required): `ideation`, `grill`, `plan`, `tickets`, `implement`, `testing`, `live-testing` or `post-release` | the updated feature, as in [server.md](server.md#feature). It also stores a `reef` message in the token's channel: `<role> moved the stage to <stage>`, unless the feature is already in that stage. No approval: the stage is a label | `invalid: unknown stage "x"`, `invalid: feature login is a list of fixes and has no stage` |
| `propose_wrap` | `brief` (required); `items` (memory items not saved with `remember` yet), `retire` (ids of live memory items that are stale or duplicate). Each item is `{"kind", "body", "why", "scope", "replaces"}`: `kind` and `body` required, `scope` is `work_item` (the default), `project`, `teammate`, or `team` when the token's role leads the channel's team, `replaces` lists live item ids it replaces | `{"id": 1, "message": "proposal saved; the user will approve it"}` | `invalid: proposal brief is required`, `invalid: item body is required`, `invalid: unknown item kind "x"`, `invalid: scope "x" is not one of work_item, project, teammate` (a solo channel has no `team` scope; a channel of a real team adds `team`), `invalid: only the lead of team "core" can propose team items`, `invalid: memory 9 is not a live item in work_item:1, project:1, teammate:architect` (a `retire` id that is not an accepted item of the feature, project, team or role here; a `replaces` id must be an accepted item of the new item's own scope) (nothing is stored) |
| `propose_brief` | `brief` (required) | `{"id": 1, "message": "proposal saved; the user will approve it"}` | `invalid: proposal brief is required`, `invalid: channel 1 has a wrap-up waiting for approval` |
| `browser_navigate` | `url` (required) | `{"text": "navigated to <url>"}` | `browser unavailable: the Reef app is not open`, `browser did not answer: <error>`, `the user has the browser`, `refused: the browser pane cannot open Reef itself` (or another `refused: ...` for a non-http(s) URL), the page's own error (e.g. `net::ERR_CONNECTION_REFUSED`) when it fails to load |
| `browser_screenshot` | none | a PNG image of the browser pane's current page | `browser unavailable: the Reef app is not open`, `browser did not answer: <error>`, `the user has the browser` |
| `browser_snapshot` | none | the page's accessibility tree as text, one line per element: `- role "name" [ref=e12]`. A ref is only valid until the next snapshot or a navigation | `browser unavailable: the Reef app is not open`, `browser did not answer: <error>`, `the user has the browser` |
| `browser_click` | `ref` (required, from `browser_snapshot`) | `{"text": "clicked <ref>"}` | `browser unavailable: the Reef app is not open`, `browser did not answer: <error>`, `the user has the browser`, `ref e12 is not on the page; take a new snapshot`, `cannot click e12: <error>` for any other failure |
| `browser_type` | `ref` (required, from `browser_snapshot`), `text` (required) | `{"text": "typed into <ref>"}` | `browser unavailable: the Reef app is not open`, `browser did not answer: <error>`, `the user has the browser`, `ref e12 is not on the page; take a new snapshot`, `cannot type into e12: <error>` for any other failure |
| `browser_console` | none | `{"text": "..."}`: the console messages and errors since the last navigation, one per line with their level, e.g. `error: boom` | `browser unavailable: the Reef app is not open`, `browser did not answer: <error>`, `the user has the browser` |
| `browser_network` | none | `{"text": "..."}`: the network requests since the last navigation, one per line with method, URL and status, e.g. `GET http://localhost/ 404` | `browser unavailable: the Reef app is not open`, `browser did not answer: <error>`, `the user has the browser` |

`propose_wrap` saves a pending proposal and changes nothing else. See Wrap-up. A channel has one
pending proposal: a new wrap rejects the older pending proposals of the channel, whatever their
kind, and `propose_wrap` refuses a `retire` id that is not an accepted item in a scope the role can
read, and a `replaces` id that is not an accepted item of the new item's own scope, so the agent
sees the error and nothing is saved.
`propose_brief` saves a pending proposal of kind `brief`: the agent calls it when the user and
the team agree on the goal, scope or plan, and the channel goes on. Approving it adds one
brief version and nothing else: no memory item is saved or retired and the channel
stays open. A new brief proposal rejects the channel's older pending brief proposals, and is
refused while a wrap is pending. Approving a brief proposal of a closed channel fails with
`invalid: channel 1 is closed`. `reef wrap approve` prints `approved: brief saved`.

The browser tools go straight from `reef mcp` to the Reef app, over `browser.sock` in the data
folder, not through `reef serve` (see [desktop.md](desktop.md#browser-pane)). With no socket, or
a stale one left by a crash, all seven tools fail with `browser unavailable: the Reef app is not
open`. Once connected, a write, read or deadline failure instead fails with `browser did not
answer: <error>`.

`post_to_channel` is how a teammate speaks in the channel besides its reply. When a turn
posts, its plain reply stays on the turn and is not a second message. `reef serve`
routes the message when the poster's turn ends: every `@role` at the start of a line of the body
(and each `@role` of an unbroken run there) wakes that teammate, and a post that hands off to
nobody wakes no one (see [server.md](server.md#routing)). Plain Claude
Code and `reef chat` have no router, so a post there is only stored.

`send_to_role` starts a private request/response chain under the current channel. The named
role wakes with the sender, channel, direct message, chain and `source: direct request` in its
prompt. Its ordinary reply is stored directly for the sender and wakes the sender with
`source: direct response`. That turn's ordinary reply is not stored or routed; continuing
requires another `send_to_role`. Direct messages never appear in `read_thread` or the public
channel message list. Partial output from a failed direct turn is kept privately with status
`failed` and does not wake the sender. A retry appends the one `done` response, which wakes the
sender once; SQLite permits only one `done` response per request.

Direct work reaches the rest of the team through public posts, not through search. Example:
the lead posts `@developer add the export button`. The developer and the tester trade
`send_to_role` messages about a failing fixture and its fix; no other role can read or find
them. When the chain is done, the developer posts one public message, `Export button done;
tester confirmed it on the sqlite fixture`. Raw logs and routine acknowledgements stay
private. A chain that ends without a public post stores no channel message; wrap-up (below)
carries a private decision into the feature's memory.

## The context prompt

`reef context <feature> --role R` prints it, and `reef chat` and `reef serve` put it before the
first message of a new session. One assembler (`channel/context.go`) builds it for a channel on
a feature and for a quick session. It has three sections in this order, each with its own
`##` header. It never holds a message body. A part with nothing to say is left out: there is
no `No brief yet.` and no `none`. A section with no part left is left out.

1. `## You`
   - When the role has a persona or skills (a team role, see Teams): the persona, then
     `Your skills:` and the skills, each followed by a blank line. `reef context` has no role
     to read them from and does not print them. A role that wakes with a persona or skills that
     differ from what its session last got, whether it is starting or resuming, gets them again
     before the wake text, marked `Your persona or skills changed:`, without the header.
   - One line: role, feature name and slug, project name. A quick session says instead:
     role, project name and folder, and that the session has no feature yet, so there is no brief or
     feature items.
   - When a profile exists, `You work with Ada, a developer. They like: short answers, ask before
     big changes.` (occupation and style only when set, lower-cased; `Other` is left out).
   - The goal (if set) and the stage, then one line that lists the stages in order and asks the
     agent to call `set_stage` when the work moves to the next. A `fixes` feature says it is a
     list of fixes tracked as tasks instead, with no stage lines. A quick session has none of these.
   - When the project has a base branch, `Base branch: dev. A pull request you open targets it.`
2. `## How to work`
   - When the team has two or more roles: `Your team: @lead, @architect.` It says that
     `post_to_channel` and @mentions are shared (decisions, blockers, user questions and outcomes
     belong there), while `send_to_role` coordinates in detail with one teammate and gets one
     automatic direct response, and that an `Asked together:` line names every role one message
     woke. A public message that wakes you starts with `[channel N, message N]` or `[channel N,
     message N, thread N]`; your plain reply goes where the header says (a top-level reply starts
     your own thread), and `post_to_channel` takes the header's `thread` as its `parent` when there
     is one. After the header, a wake may carry `reef wrote: ...` lines: what `reef` posted in the
     channel since the role's last turn began, such as a rollback note (see
     [server.md](server.md#channels-live)). The prompt does not explain them. It also asks a role
     to publish one concise outcome with `post_to_channel` when a direct chain completes with
     information the team or the user needs; raw logs and routine acknowledgements stay private.
     A new channel never gets direct messages in its context: the brief and the memory items are
     the memory that carries over. The lead gets one more line: do not summarize or repeat a
     specialist's post back to the user, because they see it. `reef context` has no team and
     prints none of this.
   - On a feature, one line that names the tools for details (`recall`, `read_memory`, `read_thread`,
     `list_tasks`, `search_sessions`), `remember` and `propose_brief`. Those tools need a
     feature, so a quick session gets no such line, and a quick session with a team of one has no `## How to work` at all.
3. `## What we know`, each part under a `###` label and only when it has content:
   - `### Brief`: the latest brief, in full. Features only.
   - `### Decisions`: every live decision item in the feature's `work_item` scope, newest first,
     with no limit. Features only.
   - `### Findings`: the 5 newest accepted findings, facts, conventions and lessons in the
     feature's `work_item` scope. Features only.
   - `### Project`: the 10 newest accepted items in the project's scope.
   - `### Team`: the 5 newest accepted items in the team's scope, only when the channel's team
     is a real team: the `solo` team is no team and has none. `reef context` prints none.
   - `### Your items`: the 5 newest accepted items in the role's `teammate` scope.
   - `### Doing`: one line `task N: title` for each task in status `doing`.
   - `### On demand`: one line that counts what the role can look up:
     `23 open tasks (list_tasks). 3 earlier channels (search_sessions).` Open tasks are those not
     `done`; earlier channels are the feature's channels that have messages, not counting the
     channel the prompt is for. A count of one reads `1 open task` and `1 earlier channel`. A zero
     count is left out of the line, and the part is left out when both are zero.

   Only accepted items are read: a candidate, a rejected item and a retired item never appear.
   Newest means accepted last: a candidate the user approves is newer than an item accepted
   before it.
   Each line reads `kind id: first line of the body (author, date)`,
   the line cut at 100 characters with `…`. Findings, Project, Team and Your items end with
   `N of M; recall(..., scope=<kind>)`, where N is the lines shown and M the accepted items in
   that scope, so the agent knows to recall the rest. A quick session has only Project, Team
   and Your items. The old decision, notes and project memory tables are not read.

Example, for a feature with one approved wrap-up, asked by the developer of a team of two:

```
## You
You are the developer on feature "Email login" (login) in project "shop".
Goal: Users sign in with email and password
Stage: ideation
Stages in order: ideation, grill, plan, tickets, implement, testing, live-testing, post-release. Call set_stage when the work moves to the next one.

## How to work
Your team: @lead, @developer. Use post_to_channel and @mentions for work the whole team needs: ...
Details are on demand through the reef MCP tools recall, read_memory, read_thread, list_tasks and search_sessions. Record decisions with remember. When the user and the team agree on the goal, scope or plan, call propose_brief.

## What we know
### Brief
Goal: Users sign in with email and password.
Status: ideation. No implementation yet.
Key decisions: Lock account after 5 failed logins (decision 1).
Open questions: lockout duration and unlock method.

### Decisions
decision 1: Lock an account after 5 failed logins. (developer, 2026-10-13)

### Findings
finding 4: The mail provider rate-limits to 10 sends a minute. (developer, 2026-10-14)
1 of 1; recall(..., scope=work_item)

### Project
convention 2: Sessions are stored server-side, not JWT. (architect, 2026-10-12)
1 of 1; recall(..., scope=project)

### Your items
lesson 3: Ask before adding a dependency. (developer, 2026-10-12)
1 of 1; recall(..., scope=teammate)

### Doing
task 2: Build the login form

### On demand
3 open tasks (list_tasks). 1 earlier channel (search_sessions).
```

## reef chat

```
reef chat <feature> --role R --backend claude|codex [--name N] [--permission-mode M] [--debug]
reef chat --channel ID [--debug]
```

`reef chat` runs one role in one channel, in the terminal. It uses the agent adapters
from [agents.md](agents.md). A new channel is in the `solo` team, and `--role` is the name of
its lead, so only the lead takes part: `reef chat` routes nothing, and an `@mention` in a
reply wakes no one. A resumed channel with multiple roles is refused; use the Reef window
(`reef serve`) for those teams. The code that opens a channel is in
package `channel` (`channel.Open`) and has no terminal code. `reef serve` is a second runner
of it: the window sends messages, wraps up and promotes through the same code, and it also
routes messages between teammates (see [server.md](server.md#channels-live)).

A channel stores its role, its backend and its permission mode (see
[server.md](server.md#channel)). `channel.Open` locks the channel and fails any turn it left
running (error `chat exited`). That turn belonged to a `reef chat` that died, because this
one now holds the chat lock. The turn keeps the prompt and the tokens stored while it ran,
so the next turn of that role does not send its intro again. It starts no agent: a teammate's agent starts when that role has
its first turn, in the project's folder (a channel on a feature: the feature's worktree, see
[server.md](server.md#channels-live)), with one MCP server named `reef` (this `reef`
binary, `mcp --data <data folder>`, with that teammate's own new token in `REEF_TOKEN`), plus
the role's own `mcpServers`. Claude runs the reef tools without approval; the role's own
servers need their tools in `tools` (see [agents.md](agents.md#gotchas)).
So a `reef chat` on a feature needs the project to be a git repo with a commit. Like `reef
serve`, it takes a checkpoint before each turn of a role with `write` (the `solo` lead has
it). The one-writer lock of a tree is a file lock in the data folder, so a `reef chat` and
`reef serve` on the same tree wait for each other.
`reef chat` starts the lead at once (`Session.Start`), before the first line is read, so an
agent that does not start fails the command at the start. In `reef serve` the lead starts at
its first turn: a first message that goes to a specialist starts no lead.

- With a turn of the role that has a session id, the agent resumes the session of the newest
  such turn, and the context prompt is not sent again, unless the role's persona or skills
  changed since that turn: then the persona and skills go before the wake text, marked as changed.
  On a channel with a feature, the wake text then holds what changed since the session's
  previous turn: the newest turn of that role, channel and session that sent its prompt, by its
  start time. A turn that failed before it sent told the agent nothing, so it does not count.
  The text goes right after the wake's `[channel N, message N]` header line, before any
  `reef wrote:` lines, or at the top when the turn has no header (wrap-up, promote). It has two
  parts.
  Under `### Changed since your last turn`: one index line, as in the first prompt
  (`decision 3: Use WAL (lead, 2026-10-27)`), for each memory item accepted since (a candidate counts from the moment the user approves it) in the
  scopes the role reads (`work_item`, `project`, the channel's `team` unless it is `solo`, and the role's own
  `teammate`), whatever its author (the author is only a role name, which every channel's lead
  shares), and `decision N is retired, replaced by decision M.` (`decision N is retired.` when
  nothing replaced it) for each one retired since. An item added and retired since is named as
  retired only: the role may have seen it in its previous turn. A candidate, a rejected item
  and the `teammate` items of another role never appear. Under `### New brief`: the latest brief,
  in full, when a version is newer; the next turn does not repeat it. A wake with no change has
  neither part, tasks are never in it, and a change alone starts no turn. Nothing is stored for
  it: the turn's `prompt` holds the text. It is built after the turn's start is stamped, so what
  is written while the turn waits for its tree is in it or in the next one, never in neither.
- Without one, the agent starts a new session and the context prompt goes before the
  first message, in the same prompt: the three sections above, for a channel on a feature
  and for a quick session alike. It is built after the turn's start is stamped, from the team
  and the role as they are then, so a retry of a failed first send has any edit made since.
- When the backend compacts the role's session (a `compacted` event, see
  [agents.md](agents.md)), by itself as its context fills or on `/compact`, the session no longer
  holds the first prompt it was sent. A compaction inside a turn is stored on that turn
  (`turn.compacted`); one between turns, which only the lead's live agent can report, marks the
  role's newest sent turn of that session. The role's next turn then gets the full first prompt
  again, built as for a new session, instead of the changes since its last turn; the turns after
  it get the changes again. A role whose agent starts again after a stop or a restart reads the
  mark from its newest sent turn, so it gets the full first prompt too.

New channel (`<feature> --role R --backend B`): it makes a channel named `N` (default
`<role> chat`) on the feature, with the role, backend and `--permission-mode`, then opens
it. A backend typo, or an agent that
does not start, deletes the channel again.

Resume (`--channel ID`): role, backend and permission mode come from the channel. The
flags `--role`, `--backend`, `--name` and `--permission-mode` are refused. A paused
channel still opens; see "Caps in chat" below.

Each turn:

- Each line on stdin is one message. Blank lines are skipped. `reef chat` prints `> `,
  then the agent's text as it streams. Tool calls are not printed.
- It stores the line as a message from `user`, and a turn for the role and backend. The
  turn starts `running`, gets the session id, and ends `done` with its token usage. The
  agent's whole reply is stored as one message from the role. A reply with no text stores
  no message.
- The reply is stored before the turn is marked `done`. If a store write fails during a
  turn, `reef chat` interrupts the agent and marks the turn `failed` with that error.
- An agent error ends the turn `failed` with the error text. `reef chat` then exits 1 with
  `reef: agent: <text>`. A turn that ends `failed` for any reason (an error, a stop) still
  stores the reply streamed so far, if it has text.

Caps in chat: before each line it sends, `reef chat` runs the same check as the server
(`channel.CheckCaps`). A paused channel, or one at or over its token budget (paused first),
does not run the line and does not store it. It prints `channel 1 is paused: <reason>` and
`/continue to go on`, then `> ` again. The line `/continue` clears the pause the way
`POST /channels/{id}/continue` does (`channel.Continue`): a channel at or over its token budget
gets a budget of the tokens it used plus 1,000,000, and it prints `continued`. On a channel that is not paused
it prints `the channel is not paused`. The next line then runs.

Leaving:

- At the end of stdin (Ctrl-D, or the end of a piped file) it prints `channel 1: resume
  with reef chat --channel 1` and exits 0.
- Ctrl-C during a turn interrupts the agent and stores the turn `failed` with error
  `interrupted`. The part of the reply already printed is stored. Then it prints the
  same resume line and exits 0. Closing the terminal (SIGHUP) and SIGTERM do the same.
  A turn is stored even when the signal comes right after the line is sent.

Flags:

- `--permission-mode M` is stored on the channel and goes to the backend as is, on a new
  channel and on every resume. See [agents.md](agents.md#gotchas) for the values. Without
  it, Claude denies tools that need permission. The `reef` tools are allowed anyway.
- `--debug` prints each prompt it sends to stderr, between `--- prompt ---` and `---`.
  Use it to see the context prompt.
- `--name` is only for a new channel.

## Wrap-up

A channel ends explicitly, never on a timer. The user types `/wrap` in `reef chat`:

1. `reef chat` sends the agent a fixed prompt instead of `/wrap`. The stored user message
   is still `/wrap`. The prompt asks the agent to call `get_brief` first, then `propose_wrap`
   once, with the full brief written as the latest brief plus what this channel achieved
   (keep what still holds, status is where the feature is now and not a history, drop what
   is done or no longer true, stay short), and as `items` what the channel made that the
   agent has not saved with `remember` yet: decisions, findings, facts, lessons and
   conventions, each with a scope (`work_item` by default, `project`, `teammate`). Items it
   already saved are not proposed again. A lead may also propose `team` lessons. The prompt
   also asks it to use `recall` for stale or duplicate items: `retire` lists their ids, and
   `replaces` on a new item lists older ones it amends. When the channel has direct messages, the
   prompt starts with all of them as plain text, one line each, `[direct 1] developer ->
   tester: use the sqlite fixture`, with ` (failed)` after the recipient for a failed attempt (a body is cut at 1000 characters; only the newest 100 are
   listed; none are listed when there are none), and asks the agent to propose as items
   what they decided or found that is not saved yet and a brief that reflects the direct work
   that is complete. A promote turn gets the same list.
2. `propose_wrap` stores a `pending` proposal (see [server.md](server.md#proposal)).
3. After the turn, `reef chat` looks for a pending wrap proposal of this channel that is newer
   than the one pending before `/wrap`. With none, it prints `no proposal made`: an agent that
   answers with `propose_brief` has made none. A pending brief proposal does not block `/wrap`;
   the new wrap rejects it.
4. Otherwise it prints the proposal: the brief, the items (`- decision (work_item): body
   (why) [replaces [3]]`, kind and scope first), the memory items to retire, then `approve? [y/N]`.
5. The next line typed after the prompt decides. `y` or `yes` (any case) approves.
   Anything else, a blank line too, rejects. `reef chat` reads stdin all the time, so it
   drops lines already queued after first printing `approve? [y/N]`. It prints
   `ignored while wrapping: <line>` for each non-blank dropped line, then prints the prompt
   again. Those lines are not answers.
   If stdin ends at the prompt, or the user presses Ctrl-C there, the proposal stays
   `pending` and it prints `proposal 1 left pending`. After Ctrl-C it then prints the resume line and exits 0.

Approve is not one transaction: saving an item calls the memory store, which may embed over
the network, and that must not hold the database write lock. It runs in three steps, and the
same code serves `reef chat`, `reef wrap approve` and the server:

1. Each item is saved with `remember`, the role as author and role, in the scope it names
   (`work_item` is the channel's feature, `project` its project, `teammate` the role, `team`
   the channel's team). The usual write rules apply, so a `team` item, and a convention from a
   role other than `architect` or `lead`, is a `candidate` that waits in `reef memory pending`.
   An item whose exact body is already an accepted or candidate item of that scope is
   skipped, so an item the agent saved live is not saved twice. An id in `replaces` that is
   no longer accepted is dropped.
2. Each id in `retire` is retired with no replacement. One that is already retired is
   skipped.
3. One store transaction: a new brief version with the role as author, the proposal's channel
   closed (`closedAt`) and the proposal marked `approved` with `decidedAt`. The server then
   stops its agent and refuses new turns on it until `POST /channels/{id}/reopen`.

A failure in step 1 or 2 leaves the proposal `pending` and the channel open. Items saved
before it stay saved; approving again skips them and finishes. It prints `approved: brief and 2 items
saved`, counting only the items this approve saved. Each `retire` and `replaces` id was checked when the proposal was
made; a `retire` id that has since become unknown, a candidate or rejected fails step 2.

Reject marks the proposal `rejected` and changes nothing else: `rejected: nothing
changed`. To change a proposal, reject it, tell the agent what to change, and type
`/wrap` again. A proposal is decided once. A second approve or reject fails with
`invalid: proposal 1 is already approved`.

`reef serve` offers the same over REST, and promotes a quick session with the same
approval: see [server.md](server.md#wrap-up-and-promotion).

The chat goes on after `/wrap`. The next session, in any channel, starts from the
approved brief and memory items.

A proposal stays `pending` until someone decides it. `reef chat` only asks about the one
`/wrap` just made. Every other pending proposal is decided later with `reef wrap`: one
left pending by end of input or Ctrl-C, one the agent proposed outside `/wrap`, and every proposal from plain Claude Code (below).
`reef wrap list` shows them, `reef wrap show <id>` prints one, and `reef wrap approve <id>` or `reef wrap
reject <id>` decides it, with the same effect and output as in the chat. A proposal
that is already decided fails with `invalid: proposal 1 is already approved`, exit 1.

## Using it from plain Claude Code

The CLI and `reef mcp` work without `reef chat` and without `reef serve`. In the project
folder:

```
$ reef project add
1  shop  /Users/me/code/shop
$ reef feature add login "Email login"
1  login  Email login
$ reef token login --role developer
claude mcp add reef --env REEF_TOKEN=572b4e83...abda4 -- /usr/local/bin/reef mcp --data '/Users/me/Library/Application Support/Reef'
```

Run the printed `claude mcp add` line once. Claude Code then starts `reef mcp` with the
token and gets the tools above as `mcp__reef__<tool>`. Start the session with the context
prompt:

```
claude "$(reef context login --role developer)"
```

Claude Code's own messages are not stored in Reef. Only what the tools write is: memory items,
tasks and proposals. The channel that `reef token` made stays empty, so `search_sessions`
and `read_thread` see only `reef chat` channels. Each `reef token` call makes a new
channel and token.
