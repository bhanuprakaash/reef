# Memory

The memory loop gives every session on a work item the same starting point, and saves what
the session learned when it ends. It has four parts: the `reef` CLI
([`cmd/reef/cli.go`](../cmd/reef/cli.go)), the MCP server `reef mcp`
([`server/mcp.go`](../server/mcp.go)), the context prompt
([`space/context.go`](../space/context.go)) and `reef chat`
([`cmd/reef/chat.go`](../cmd/reef/chat.go), [`space/runner.go`](../space/runner.go)).
The code is the source of truth. This doc says what it does and why.

## Why

Without Reef, a big work item is split over many sessions. The agent writes a handoff file
so that the next session can continue, and the history of why things were done gets lost.
Reef has no handoff file. A new session starts with three things:

- The work item's brief: goal, status, key decisions, open questions.
- Memory items: every live decision of the work item and the newest findings, project, team and
  teammate items, each part with a count of how many exist.
- An index: the sub-items in progress, and a count of the open sub-items and the earlier Spaces.

It never gets old message bodies. When it needs details, it fetches them on demand with
the MCP tools (`recall`, `read_memory`, `read_thread`, `search_sessions`). So the first prompt
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
- `reef chat`: one process per open Space. It starts the agent, and the agent starts
  its own `reef mcp`.

Only `reef serve` takes `reef.lock`. The CLI, `reef mcp` and `reef chat` open `reef.db`
directly, with or without a running server. WAL mode and a 5 second busy timeout let the
processes share the file.

`reef chat` also locks `chat-<space id>.lock` in the data folder while it runs, and so
does `reef serve` for a Space it runs. A second `reef chat` on the same Space exits
with `space 1 is open elsewhere: conflict`. When `reef serve` starts, it fails the
turns that a dead server left running (see [server.md](server.md#restart-recovery)). It
skips a Space whose chat lock is held, because that turn is still live in a `reef chat`
process.

## CLI

```
reef <command> [args] [--data DIR]
```

Rules for every command:

- `--data DIR` is the data folder, the same as for `reef serve`. Flags can come before or
  after the other arguments.
- `--` ends the flags: every argument after it is positional, even if it starts with `-`.
- `<work-item>` is a Work item id (all digits), else `work item "login" is not an id`.
- Commands that need a project and get no work item use the project of the current folder:
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
| `reef work-item add "<title>" [--type T] [--description D]` | Adds a Work item to the current folder's project. `T` is one of the types in [server.md](server.md#work_item); the default is `work item` | `1  work item  Email login`: id, type, title |
| `reef brief <work-item>` | Prints the latest brief. With none: `work item 1 has no brief yet` | the brief body |
| `reef decide <work-item> "<body>" [--why W]` | Writes a `decision` item in the work item's `work_item` scope as author and role `user`, so it is `accepted`. After the item, the lookalike and vector lines of `reef memory add` | `1  Use sessions, not JWT`: id, body; then `3  similar  <body>` |
| `reef memory add <kind> "<body>" --scope S [--as ROLE] [--about X]... [--replaces ID]... [--why W] [--needs-approval]` | Writes a memory item as author `user`, so it is `accepted`; `--needs-approval` makes it a `candidate` that waits in `pending`. `S` and each `X` are `kind:id`, such as `project:1` or `work_item:12`; `--about` and `--replaces` repeat. Each `--replaces` item must be accepted (a candidate is approved or rejected first) and in the same scope as the new one; it is retired and points at the new one, or, with `--needs-approval`, when the new one is approved. `--as` sets the role the item carries. After the item, one line per stored item that looks like it. When an embedding provider is set and the vector could not be stored, stderr gets `vector not stored: <reason>; run reef memory embed <provider> to retry` and the exit code stays 0; the reason names the status and the vendor's error code, never the key | `12  accepted  decision  project:1  Use sessions, not JWT`: id, status, kind, scope, body; then `3  similar  <body>` |
| `reef memory recall "<q>" [--scope S] [--limit N]` | Searches the items of every scope, retired ones included, plus the items one link away from the best hits. `--scope` narrows to one shelf; `--limit` defaults to 10 | one hit per line: `12  accepted  decision  project:1  direct  Use sessions, not JWT`: id, status, kind, scope, `via`, body. `via` is `direct` or the link it was reached through, such as `replaces memory:4` |
| `reef memory show <id>` | Prints one item with its details | the item row as for `add`, then these lines when they apply: `why  <why>`, `author  user  <role>` (the role is left out when empty), one `link  <rel>  <kind>:<id>` per link, `replaced_by  <id>`, `retired_at  <RFC 3339 time>` |
| `reef memory retire <id>` | Retires an accepted item with nothing replacing it. Refuses a candidate (`invalid: memory 3 is a candidate`; use `reject`), a rejected item and a retired one (`invalid: memory 3 is already retired`) | `12  retired` |
| `reef memory pending` | Lists the candidates waiting for approval | one item row per line, as for `add`; nothing when none |
| `reef memory reject <id>` | Turns down a candidate: it is kept with status `rejected` and `retired_at` set, and no search or recall finds it. An item the candidate would have replaced stays live. Any other item fails with `invalid: memory 3 is not a candidate` | `12  rejected` |
| `reef memory approve <id>` | Moves a candidate to `accepted` and retires each item it replaces (one already retired is skipped). Any other item fails with `invalid: memory 3 is not a candidate` | `12  accepted` |
| `reef memory embed` | Prints the embedding provider: `none`, `openai`, `voyage` or `gemini`. The key is stored in `reef.db` and never printed | `openai`; `none` when unset |
| `reef memory embed <provider> [--key K]` | Sets the embedding provider and stores its key in `reef.db`; the key is never printed. Storing a key makes `reef.db` and its `-wal` and `-shm` files readable only by the owner (mode 0600). `openai`, `voyage` and `gemini` need `--key` unless the stored provider is the same one, else `invalid: embedding provider voyage needs --key`; `none` clears the key. Changing the provider clears every stored vector. Each `openai`, `voyage` or `gemini` call then embeds every item that has no vector; a failure there exits 1 after the setting is saved, and running the command again retries | `embedding provider openai; embedded 12 items` (`embedded 0 items` when none was missing), or `embedding provider none` |
| `reef team apply FILE.json` | Creates a team from a file, or replaces the roles of the team with that name (see Teams) | `applied team duo: lead*(claude) architect(claude)` |
| `reef team list` | Lists the teams by name, each with its roles | `duo  lead*(claude) architect(claude)`: the lead has a `*` |
| `reef context <work-item> --role R` | Prints the context prompt for role `R` (see below) | the prompt |
| `reef token <work-item> --role R` | Makes a Space and a token for role `R` (see Tokens) | a `claude mcp add` command |
| `reef wrap list [<work-item>]` | Lists pending wrap-up proposals (see Wrap-up): of the work item, or of the current folder's project | `1  1  3  architect  wrap  2026-10-13`: id, work item id, Space id, role, kind (`wrap` or `brief`), created date |
| `reef wrap show <id>` | Prints a proposal and its status | `proposal 1 from architect` (`brief update 2 from lead` for a brief proposal), status, then brief, items (kind and scope each) and retire ids |
| `reef wrap approve <id>` | Approves a pending proposal | `approved: brief and 2 items saved`, or `approved: brief saved` for a brief proposal |
| `reef wrap reject <id>` | Rejects a pending proposal | `rejected: nothing changed` |
| `reef chat ...` | Runs a Space in the terminal (see below) | the conversation |
| `reef mcp` | Serves the MCP tools over stdio (see below) | MCP only |

## Teams

A team is a set of roles, one of them the lead. It is a JSON
file. `reef team apply FILE.json` stores it, and a Space picks it by name (`team` in `POST
/spaces`, see [server.md](server.md#team)). Migration 6 seeds a team `solo`, with one role,
`lead` on `claude`, and every Space without a team is in it. A new Space created by
`reef chat` is in `solo`. When resuming, `reef chat` allows teams with one role and refuses
teams with multiple roles; use the Reef window (`reef serve`) for those teams.

`teams/full.json` in the repo is a four-role team: `lead`, `architect`, `developer` (the only
role with `write`) and `tester`, all on `claude`. Each persona names
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
  ]
}
```

- `name` and `roles` are required. A role needs `name` and `backend` (`claude` or `codex`). The
  other role fields are optional: `lead` (default `false`), `write` (default `false`), `permissionMode`, `persona`,
  `skills`, `face`, `tools` and `mcpServers`. What each does is in [server.md](server.md#role).
  Exactly one role has `"lead": true`. A role name can appear once, uses only letters, digits,
  `_` and `-` (so an `@mention` can name it), and is not `user`.
- The file is read strictly: an unknown field is an error.
- Applying a name that exists replaces its roles in one transaction, and the
  team keeps its Spaces. One failed check changes nothing. A new persona or skills
  reaches the role's next wake, whether that starts a session or resumes one (see below).
  A role applied with a new `backend` starts a new session; it does not resume the old one.
- Errors print `reef: <message>` and exit 1: `reef: invalid: team bad needs exactly one
  lead role, has 0` (or `has 2`), `reef: role a: invalid: backend must be claude or codex`,
  `reef: invalid: role name "two words" must be letters, digits, _ or -, and not user or reef`,
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

A token says who a `reef mcp` process works for: one role in one Space. The Space
gives the work item and the project. The role is the author and role of the items the
agent saves with `remember`, the author of what
`post_to_space` posts, and the role on a wrap-up proposal. A role of a Space in `reef serve` comes from its
team. For `reef token` and `reef chat` it is free text.
The `teammate` scope the agent reads is named by the id of the Teammate that holds the role in the
Space's team, so renaming the Teammate keeps its items. A free-text role has no Teammate: its scope
is named by the role.

- A token is 64 hex characters (32 random bytes). It is stored in the `token` table (see
  [server.md](server.md#token)).
- `reef token` makes one, in a new Space named `<role> (claude code)`. Every start of
  `reef chat`, new or resumed, makes one for its Space, and deletes it when the chat
  ends. The tokens of `reef token` stay.
- `reef serve` makes one for each teammate session, when that teammate's agent starts, and
  deletes it when the agent stops. A specialist's agent stops after each wake, so its
  token lives for that wake. The lead's lives until the Space stops. A Space with a lead
  and an architect therefore has one token while only the lead runs, and two during an
  architect wake.
- `reef token` stores the Space role as `lead` for now, even when `--role` names another role.
- A token never expires. It goes away when its Space is deleted, or, for a `reef chat`
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

Every tool except `post_to_space`, `send_to_role`, `read_direct_messages`, `recall` and `read_memory` works on the token's work item, and so does `remember` unless its scope is `project` or `teammate`. On a quick-session Space
(no work item) those tools fail with `this space has no work item`, except `read_thread` of the
token's own Space. A result is one JSON object as text. A
tool error comes back as a result with `isError: true` and the message as text, so the
agent can read it. Arguments that do not match the tool's schema are a tool error too
(`validating "arguments": ... missing properties: ["query"]`). An unknown tool name is a
JSON-RPC error (`-32602`, `unknown tool "nope"`).

| Tool | Input | Output | Errors |
| --- | --- | --- | --- |
| `get_brief` | none | `{"brief": "..."}`: the latest brief, `""` when there is none yet. The role's own items are in scope `teammate`: read them with `recall` | |
| `remember` | `kind` (`decision`, `convention`, `finding`, `fact` or `lesson`) and `body` (required), `about` (strings `kind:id`), `replaces` (ids of accepted items it amends or replaces), `why`, `scope` (`work_item`, the default, `project` or `teammate`), `needsApproval` | `{"item": {...}, "similar": [{"id", "body"}], "embedError": "..."}`: the stored item with `id`, `kind`, `body`, `why`, `scope` (`work_item:1`), `author` and `role` (both the token's role), `status`, `retiredAt`, `replacedBy`, `createdAt` and `links` (`{"rel", "to"}`: the item's `source` is `space:<id>`); `similar` appears only when stored items look like it, and `embedError` only when no vector could be stored for it. `work_item` is the token's work item, `project` its project, `teammate` the token's role on every work item. The item is `accepted`, or a `candidate` that waits for the user when `needsApproval` is set or the kind is `convention` from a role other than `architect` or `lead`. Each `replaces` item must be an accepted item of the same scope as the new one; it is retired and points at the new one, or when the new one is a candidate, after the user approves it. `team` is never written from here: team items come only from the lead's wrap-up | `invalid: scope "team" is not work_item, project or teammate`, `this space has no work item` (`work_item` on a quick session), `invalid: unknown kind "x"`, `invalid: body is required`, `memory 9: not found` (a `replaces` id that is not readable here), `invalid: memory 3 is in scope project:1, not work_item:1` (a `replaces` id on another shelf) (nothing is written) |
| `recall` | `query` (required), `limit` (default 10), `scope` (`work_item`, `project`, `team` or `teammate`: only that one) | `{"hits": [{"id", "kind", "body", "scope", "status", "via"}]}`: matches in every scope the token can read, retired items included and marked, plus items one link away; `via` is `direct` or the link it came through. A work item's `work_item` items are read only on that work item; `team` only on a Space with a team. `hits` is `[]` when nothing matches | `query is required`, `invalid: you have no "team" scope here` |
| `read_memory` | `id` (required) | the item as `remember` returns it, with its `links` | `memory 99: not found`, also for an item in a scope the token cannot read |
| `search_sessions` | `query` (required), `limit` (default 20) | `{"hits": [{"spaceId", "messageId", "kind", "author", "snippet"}]}`: messages in all of the work item's Spaces that contain `query`, newest first. `kind` is `public` or `direct`. A direct hit appears only when the token's role is its sender or recipient, in any Space of the work item, and also has `from`, `to` and `chainId`; its `messageId` is the direct message id, which `read_thread` does not take. Case is ignored for ASCII. The snippet is about 120 characters around the match. `hits` is `null` when nothing matches | `query is required` |
| `read_thread` | `spaceId` (required), `after` (only newer message ids), `root` (the id of a top-level message) | `{"messages": [...]}`: the Space's messages, oldest first. With `root`: that message and its replies, oldest first, and `after` is ignored. Each message has `parentId`, `null` at the top level. `messages` is `null` when there are none | `space 9: not found`, also for a Space of another work item or a quick session other than the token's own; `thread 9: not found` when `root` is not a top-level message of the Space |
| `post_to_space` | `body` (required), `parent` (the id of a top-level message: reply in its thread; leave it out to post at the top level) | the stored message, with the token's role as `author`. It works on a quick session too | `invalid: message body is required`, `invalid: parent 9 is not a top-level message of space 1` |
| `send_to_role` | `to` (exactly one teammate role), `body` (required) | the stored direct request, with `spaceId`, `chainId`, `from`, `to`, `replyToId: null` and `status: sent` | empty body, self, unknown or out-of-team role, a recipient list, a sender with no running turn, a closed Space, a starting Space, or a wrapping or promoting Space are errors; nothing is stored |
| `read_direct_messages` | `after` (only newer direct-message ids) | `{"messages": [...]}` containing only exchanges in this Space where the token's role is sender or recipient | |
| `propose_wrap` | `brief` (required); `items` (memory items not saved with `remember` yet), `retire` (ids of live memory items that are stale or duplicate). Each item is `{"kind", "body", "why", "scope", "replaces"}`: `kind` and `body` required, `scope` is `work_item` (the default), `project`, `teammate`, or `team` when the token's role leads the Space's team, `replaces` lists live item ids it replaces | `{"id": 1, "message": "proposal saved; the user will approve it"}` | `invalid: proposal brief is required`, `invalid: item body is required`, `invalid: unknown item kind "x"`, `invalid: scope "x" is not one of work_item, project, teammate` (a solo Space has no `team` scope; a Space of a real team adds `team`), `invalid: only the lead of team "core" can propose team items`, `invalid: memory 9 is not a live item in work_item:1, project:1, teammate:architect` (a `retire` id that is not an accepted item of the work item, project, team or role here; a `replaces` id must be an accepted item of the new item's own scope) (nothing is stored) |
| `propose_brief` | `brief` (required) | `{"id": 1, "message": "proposal saved; the user will approve it"}` | `invalid: proposal brief is required`, `invalid: space 1 has a wrap-up waiting for approval` |
| `create_sub_item` | `title` (required), `description`, `type` (a label: `feature`, `bug`, `incident`, `investigation`, `task` or `improvement`; default `task`) | the new sub-item as the work item REST returns it, in status `backlog`, under the token's work item, which it shares the worktree with. Any teammate of the Space may add one | `invalid: work item title is required`, `invalid: unknown type "epic"`, `this Space has no work item` |
| `list_sub_items` | none | `{"subItems": [{"id", "type", "title", "status"}]}`: the work item's sub-items, oldest first, every status | `this Space has no work item` |
| `read_sub_item` | `id` (required) | one sub-item in full: `description`, `status` and `acceptanceLines` | `sub-item 9: not found`, also for a work item that is not a sub-item of the token's work item |
| `set_sub_item_status` | `id`, `status` (required: `backlog`, `queued`, `in_progress`, `needs_you`, `review`, `done` or `dropped`) | the sub-item with its new status. Only the lead of the Space's team may call it; it is the one that sets a sub-item `done`. The move goes through the same function as every status change, so a sub-item that is `done` or `dropped` does not move again. A top-level work item goes to `review` only when every sub-item is `done` or `dropped` | `invalid: only the lead sets a sub-item's status`, `sub-item 9: not found`, `invalid: work item 3: lead cannot move an item from done to in_progress`, `invalid: work item 4: unknown status "x"` |
| `browser_navigate` | `url` (required) | `{"text": "navigated to <url>"}` | `browser unavailable: the Reef app is not open`, `browser did not answer: <error>`, `the user has the browser`, `refused: the browser pane cannot open Reef itself` (or another `refused: ...` for a non-http(s) URL), the page's own error (e.g. `net::ERR_CONNECTION_REFUSED`) when it fails to load |
| `browser_screenshot` | none | a PNG image of the browser pane's current page | `browser unavailable: the Reef app is not open`, `browser did not answer: <error>`, `the user has the browser` |
| `browser_snapshot` | none | the page's accessibility tree as text, one line per element: `- role "name" [ref=e12]`. A ref is only valid until the next snapshot or a navigation | `browser unavailable: the Reef app is not open`, `browser did not answer: <error>`, `the user has the browser` |
| `browser_click` | `ref` (required, from `browser_snapshot`) | `{"text": "clicked <ref>"}` | `browser unavailable: the Reef app is not open`, `browser did not answer: <error>`, `the user has the browser`, `ref e12 is not on the page; take a new snapshot`, `cannot click e12: <error>` for any other failure |
| `browser_type` | `ref` (required, from `browser_snapshot`), `text` (required) | `{"text": "typed into <ref>"}` | `browser unavailable: the Reef app is not open`, `browser did not answer: <error>`, `the user has the browser`, `ref e12 is not on the page; take a new snapshot`, `cannot type into e12: <error>` for any other failure |
| `browser_console` | none | `{"text": "..."}`: the console messages and errors since the last navigation, one per line with their level, e.g. `error: boom` | `browser unavailable: the Reef app is not open`, `browser did not answer: <error>`, `the user has the browser` |
| `browser_network` | none | `{"text": "..."}`: the network requests since the last navigation, one per line with method, URL and status, e.g. `GET http://localhost/ 404` | `browser unavailable: the Reef app is not open`, `browser did not answer: <error>`, `the user has the browser` |

`propose_wrap` saves a pending proposal and changes nothing else. See Wrap-up. A Space has one
pending proposal: a new wrap rejects the older pending proposals of the Space, whatever their
kind, and `propose_wrap` refuses a `retire` id that is not an accepted item in a scope the role can
read, and a `replaces` id that is not an accepted item of the new item's own scope, so the agent
sees the error and nothing is saved.
`propose_brief` saves a pending proposal of kind `brief`: the agent calls it when the user and
the team agree on the goal, scope or plan, and the Space goes on. Approving it adds one
brief version and nothing else: no memory item is saved or retired and the Space
stays open. A new brief proposal rejects the Space's older pending brief proposals, and is
refused while a wrap is pending. Approving a brief proposal of a closed Space fails with
`invalid: space 1 is closed`. `reef wrap approve` prints `approved: brief saved`.

The browser tools go straight from `reef mcp` to the Reef app, over `browser.sock` in the data
folder, not through `reef serve` (see [desktop.md](desktop.md#browser-pane)). With no socket, or
a stale one left by a crash, all seven tools fail with `browser unavailable: the Reef app is not
open`. Once connected, a write, read or deadline failure instead fails with `browser did not
answer: <error>`.

`post_to_space` is how a teammate speaks in the Space besides its reply. When a turn
posts, its plain reply stays on the turn and is not a second message. `reef serve`
routes the message when the poster's turn ends: every `@role` at the start of a line of the body
(and each `@role` of an unbroken run there) wakes that teammate, and a post that hands off to
nobody wakes no one (see [server.md](server.md#routing)). Plain Claude
Code and `reef chat` have no router, so a post there is only stored.

`send_to_role` starts a private request/response chain under the current Space. The named
role wakes with the sender, Space, direct message, chain and `source: direct request` in its
prompt. Its ordinary reply is stored directly for the sender and wakes the sender with
`source: direct response`. That turn's ordinary reply is not stored or routed; continuing
requires another `send_to_role`. Direct messages never appear in `read_thread` or the public
Space message list. Partial output from a failed direct turn is kept privately with status
`failed` and does not wake the sender. A retry appends the one `done` response, which wakes the
sender once; SQLite permits only one `done` response per request.

Direct work reaches the rest of the team through public posts, not through search. Example:
the lead posts `@developer add the export button`. The developer and the tester trade
`send_to_role` messages about a failing fixture and its fix; no other role can read or find
them. When the chain is done, the developer posts one public message, `Export button done;
tester confirmed it on the sqlite fixture`. Raw logs and routine acknowledgements stay
private. A chain that ends without a public post stores no Space message; wrap-up (below)
carries a private decision into the work item's memory.

## The context prompt

`reef context <work-item> --role R` prints it, and `reef chat` and `reef serve` put it before the
first message of a new session. One assembler (`space/context.go`) builds it for a Space on
a work item and for a quick session. It has three sections in this order, each with its own
`##` header. It never holds a message body. A part with nothing to say is left out: there is
no `No brief yet.` and no `none`. A section with no part left is left out.

1. `## You`
   - When the role has a persona or skills (a team role, see Teams): the persona, then
     `Your skills:` and the skills, each followed by a blank line. `reef context` has no role
     to read them from and does not print them. A role that wakes with a persona or skills that
     differ from what its session last got, whether it is starting or resuming, gets them again
     before the wake text, marked `Your persona or skills changed:`, without the header.
   - One line: role, work item title and id, project name. A quick session says instead:
     role, project name and folder, and that the session has no work item yet, so there is no brief or
     work item items.
   - When a profile exists, `You work with Ada, a developer. They like: short answers, ask before
     big changes.` (occupation and style only when set, lower-cased; `Other` is left out).
   - The description (if set), as `Description: ...`. A quick session has none.
   - When the project has a base branch, `Base branch: dev. A pull request you open targets it.`
2. `## How to work`
   - When the team has two or more roles: `Your team: @lead, @architect.` It says that
     `post_to_space` and @mentions are shared (decisions, blockers, user questions and outcomes
     belong there), while `send_to_role` coordinates in detail with one teammate and gets one
     automatic direct response, and that an `Asked together:` line names every role one message
     woke. A public message that wakes you starts with `[Space N, message N]` or `[Space N,
     message N, thread N]`; your plain reply goes where the header says (a top-level reply starts
     your own thread), and `post_to_space` takes the header's `thread` as its `parent` when there
     is one. After the header, a wake may carry `reef wrote: ...` lines: what `reef` posted in the
     Space since the role's last turn began, such as a rollback note (see
     [server.md](server.md#spaces-live)). The prompt does not explain them. It also asks a role
     to publish one concise outcome with `post_to_space` when a direct chain completes with
     information the team or the user needs; raw logs and routine acknowledgements stay private.
     A new Space never gets direct messages in its context: the brief and the memory items are
     the memory that carries over. The lead gets one more line: do not summarize or repeat a
     specialist's post back to the user, because they see it. `reef context` has no team and
     prints none of this.
   - On a work item, one line that names the tools for details (`recall`, `read_memory`, `read_thread`,
     `search_sessions`), `remember` and `propose_brief`. A quick session gets a
     shorter one: details are on demand through `recall` and `read_memory`, and `remember` saves
     what holds across the project or for the role, with scope `project` or `teammate`. So a
     quick session always has a `## How to work`.
3. `## What we know`, each part under a `###` label and only when it has content:
   - `### Brief`: the latest brief, in full. Work items only.
   - `### In progress`: one line `sub-item N: title (type)` for each sub-item of the work item
     whose status is `in_progress`. Work items only.
   - `### Decisions`: every live decision item in the work item's `work_item` scope, newest first,
     with no limit. Work items only.
   - `### Findings`: the 5 newest accepted findings, facts, conventions and lessons in the
     work item's `work_item` scope. Work items only.
   - `### Project`: the 10 newest accepted items in the project's scope.
   - `### Team`: the 5 newest accepted items in the team's scope, only when the Space's team
     is a real team: the `solo` team is no team and has none. `reef context` prints none.
   - `### Your items`: the 5 newest accepted items in the role's `teammate` scope.
   - `### On demand`: one line that counts what the role can look up:
     `4 open sub-items (list_sub_items). 3 earlier Spaces (search_sessions).` Open sub-items
     are the work item's sub-items that are not `done` or `dropped`, the ones in progress
     included. Earlier Spaces are the work item's Spaces that have messages, not counting the
     Space the prompt is for. A count of one reads `1 open sub-item` or `1 earlier Space`.
     A zero count is left out, and the part is left out with it.

   Only accepted items are read: a candidate, a rejected item and a retired item never appear.
   Newest means accepted last: a candidate the user approves is newer than an item accepted
   before it.
   Each line reads `kind id: first line of the body (author, date)`,
   the line cut at 100 characters with `…`. Findings, Project, Team and Your items end with
   `N of M; recall(..., scope=<kind>)`, where N is the lines shown and M the accepted items in
   that scope, so the agent knows to recall the rest. A quick session has only Project, Team
   and Your items. The old decision, notes and project memory tables are not read.

Example, for a work item with one approved wrap-up, asked by the developer of a team of two:

```
## You
You are the developer on work item "Email login" (1) in project "shop".
Description: Users sign in with email and password

## How to work
Your team: @lead, @developer. Use post_to_space and @mentions for work the whole team needs: ...
Details are on demand through the reef MCP tools recall, read_memory, read_thread and search_sessions. Record decisions with remember. When the user and the team agree on the goal, scope or plan, call propose_brief.

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

### On demand
1 earlier Space (search_sessions).
```

## reef chat

```
reef chat <work-item> --role R --backend claude|codex [--name N] [--permission-mode M] [--debug]
reef chat --space ID [--debug]
```

`reef chat` runs one role in one Space, in the terminal. It uses the agent adapters
from [agents.md](agents.md). A new Space is in the `solo` team, and `--role` is the name of
its lead, so only the lead takes part: `reef chat` routes nothing, and an `@mention` in a
reply wakes no one. A resumed Space with multiple roles is refused; use the Reef window
(`reef serve`) for those teams. The code that opens a Space is in
package `space` (`space.Open`) and has no terminal code. `reef serve` is a second runner
of it: the window sends messages and wraps up through the same code, and it also
routes messages between teammates (see [server.md](server.md#spaces-live)).

A Space stores its role, its backend and its permission mode (see
[server.md](server.md#space)). `space.Open` locks the Space and fails any turn it left
running (error `chat exited`). That turn belonged to a `reef chat` that died, because this
one now holds the chat lock. The turn keeps the prompt and the tokens stored while it ran,
so the next turn of that role does not send its intro again. It starts no agent: a teammate's agent starts when that role has
its first turn, in the project's folder (a Space on a work item: the work item's worktree, see
[server.md](server.md#spaces-live)), with one MCP server named `reef` (this `reef`
binary, `mcp --data <data folder>`, with that teammate's own new token in `REEF_TOKEN`), plus
the role's own `mcpServers`. Claude runs the reef tools without approval; the role's own
servers need their tools in `tools` (see [agents.md](agents.md#gotchas)).
So a `reef chat` on a work item needs the project to be a git repo with a commit. Like `reef
serve`, it takes a checkpoint before each turn of a role with `write` (the `solo` lead has
it). The one-writer lock of a tree is a file lock in the data folder, so a `reef chat` and
`reef serve` on the same tree wait for each other.
`reef chat` starts the lead at once (`Session.Start`), before the first line is read, so an
agent that does not start fails the command at the start. In `reef serve` the lead starts at
its first turn: a first message that goes to a specialist starts no lead.

- With a turn of the role that has a session id, the agent resumes the session of the newest
  such turn, and the context prompt is not sent again, unless the role's persona or skills
  changed since that turn: then the persona and skills go before the wake text, marked as changed.
  The wake text then holds what changed since the session's
  previous turn: the newest turn of that role, Space and session that sent its prompt, by its
  start time. A turn that failed before it sent told the agent nothing, so it does not count.
  The text goes right after the wake's `[Space N, message N]` header line, before any
  `reef wrote:` lines, or at the top when the turn has no header (wrap-up). It has two
  parts.
  Under `### Changed since your last turn`: one index line, as in the first prompt
  (`decision 3: Use WAL (lead, 2026-10-27)`), for each memory item accepted since (a candidate counts from the moment the user approves it) in the
  scopes the role reads (`work_item`, `project`, the Space's `team` unless it is `solo`, and the role's own
  `teammate`), whatever its author (the author is only a role name, which every Space's lead
  shares), and `decision N is retired, replaced by decision M.` (`decision N is retired.` when
  nothing replaced it) for each one retired since. An item added and retired since is named as
  retired only: the role may have seen it in its previous turn. A candidate, a rejected item
  and the `teammate` items of another role never appear. Under `### New brief`: the latest brief,
  in full, when a version is newer (a work item's Space only); the next turn does not repeat it. A wake with no change has
  neither part, and a change alone starts no turn. Nothing is stored for
  it: the turn's `prompt` holds the text. It is built after the turn's start is stamped, so what
  is written while the turn waits for its tree is in it or in the next one, never in neither.
- Without one, the agent starts a new session and the context prompt goes before the
  first message, in the same prompt: the three sections above, for a Space on a work item
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

New Space (`<work-item> --role R --backend B`): it makes a Space named `N` (default
`<role> chat`) on the work item, with the role, backend and `--permission-mode`, then opens
it. A backend typo, or an agent that
does not start, deletes the Space again.

Resume (`--space ID`): role, backend and permission mode come from the Space. The
flags `--role`, `--backend`, `--name` and `--permission-mode` are refused. A paused
Space still opens; see "Caps in chat" below.

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
(`space.CheckCaps`). A paused Space, or one at or over its token budget (paused first),
does not run the line and does not store it. It prints `space 1 is paused: <reason>` and
`/continue to go on`, then `> ` again. The line `/continue` clears the pause the way
`POST /spaces/{id}/continue` does (`space.Continue`): a Space at or over its token budget
gets a budget of the tokens it used plus 1,000,000, and it prints `continued`. On a Space that is not paused
it prints `the space is not paused`. The next line then runs.

Leaving:

- At the end of stdin (Ctrl-D, or the end of a piped file) it prints `space 1: resume
  with reef chat --space 1` and exits 0.
- Ctrl-C during a turn interrupts the agent and stores the turn `failed` with error
  `interrupted`. The part of the reply already printed is stored. Then it prints the
  same resume line and exits 0. Closing the terminal (SIGHUP) and SIGTERM do the same.
  A turn is stored even when the signal comes right after the line is sent.

Flags:

- `--permission-mode M` is stored on the Space and goes to the backend as is, on a new
  Space and on every resume. See [agents.md](agents.md#gotchas) for the values. Without
  it, Claude denies tools that need permission. The `reef` tools are allowed anyway.
- `--debug` prints each prompt it sends to stderr, between `--- prompt ---` and `---`.
  Use it to see the context prompt.
- `--name` is only for a new Space.

## Wrap-up

A Space ends explicitly, never on a timer. The user types `/wrap` in `reef chat`:

1. `reef chat` sends the agent a fixed prompt instead of `/wrap`. The stored user message
   is still `/wrap`. The prompt asks the agent to call `get_brief` first, then `propose_wrap`
   once, with the full brief written as the latest brief plus what this Space achieved
   (keep what still holds, status is where the work item is now and not a history, drop what
   is done or no longer true, stay short), and as `items` what the Space made that the
   agent has not saved with `remember` yet: decisions, findings, facts, lessons and
   conventions, each with a scope (`work_item` by default, `project`, `teammate`). Items it
   already saved are not proposed again. A lead may also propose `team` lessons. The prompt
   also asks it to use `recall` for stale or duplicate items: `retire` lists their ids, and
   `replaces` on a new item lists older ones it amends. When the Space has direct messages, the
   prompt starts with all of them as plain text, one line each, `[direct 1] developer ->
   tester: use the sqlite fixture`, with ` (failed)` after the recipient for a failed attempt (a body is cut at 1000 characters; only the newest 100 are
   listed; none are listed when there are none), and asks the agent to propose as items
   what they decided or found that is not saved yet and a brief that reflects the direct work
   that is complete.
2. `propose_wrap` stores a `pending` proposal (see [server.md](server.md#proposal)).
3. After the turn, `reef chat` looks for a pending wrap proposal of this Space that is newer
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
   (`work_item` is the Space's work item, `project` its project, `teammate` the role, `team`
   the Space's team). The usual write rules apply, so a `team` item, and a convention from a
   role other than `architect` or `lead`, is a `candidate` that waits in `reef memory pending`.
   An item whose exact body is already an accepted or candidate item of that scope is
   skipped, so an item the agent saved live is not saved twice. An id in `replaces` that is
   no longer accepted is dropped.
2. Each id in `retire` is retired with no replacement. One that is already retired is
   skipped.
3. One store transaction: a new brief version with the role as author, the proposal's Space
   closed (`closedAt`) and the proposal marked `approved` with `decidedAt`. The server then
   stops its agent and refuses new turns on it until `POST /spaces/{id}/reopen`.

A failure in step 1 or 2 leaves the proposal `pending` and the Space open. Items saved
before it stay saved; approving again skips them and finishes. It prints `approved: brief and 2 items
saved`, counting only the items this approve saved. Each `retire` and `replaces` id was checked when the proposal was
made; a `retire` id that has since become unknown, a candidate or rejected fails step 2.

Reject marks the proposal `rejected` and changes nothing else: `rejected: nothing
changed`. To change a proposal, reject it, tell the agent what to change, and type
`/wrap` again. A proposal is decided once. A second approve or reject fails with
`invalid: proposal 1 is already approved`.

`reef serve` offers the same over REST, and attaches a free Space to a work item, moving its
`space` items to the work item: see [server.md](server.md#wrap-up-and-attach).

The chat goes on after `/wrap`. The next session, in any Space, starts from the
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
$ reef work-item add "Email login"
1  work item  Email login
$ reef token 1 --role developer
claude mcp add reef --env REEF_TOKEN=572b4e83...abda4 -- /usr/local/bin/reef mcp --data '/Users/me/Library/Application Support/Reef'
```

Run the printed `claude mcp add` line once. Claude Code then starts `reef mcp` with the
token and gets the tools above as `mcp__reef__<tool>`. Start the session with the context
prompt:

```
claude "$(reef context 1 --role developer)"
```

Claude Code's own messages are not stored in Reef. Only what the tools write is: memory items
and proposals. The Space that `reef token` made stays empty, so `search_sessions`
and `read_thread` see only `reef chat` Spaces. Each `reef token` call makes a new
Space and token.
