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
- The role's own notes on the feature.
- An index: one line per decision, channel and open task.

It never gets old message bodies. When it needs details, it fetches them on demand with
the MCP tools (`read_decision`, `read_thread`, `search_sessions`). So the first prompt
stays small after many sessions. At the end of a session, the agent proposes a new brief,
new notes and the decisions it did not record yet. The user approves, and the next
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
| `reef decide <feature> "<body>" [--why W]` | Records a decision with no source message | `1  Use sessions, not JWT` |
| `reef task add [<feature>] "<title>"` | Adds a `todo` task. With a feature, the task is in the feature's project and tagged with it. Without one, it is in the current folder's project, with no feature | `1  todo  Build the login form` |
| `reef task list [<feature>] [--status S]` | Lists the feature's tasks, or all tasks of the current folder's project. `S` is `todo`, `doing` or `done` | one task per line, as for `task add` |
| `reef memory [--project P] [show]` | Prints the current project's memory, or the memory of project `P` (an id); bare `reef memory` means `show`. `--project` may come before or after `show` | the memory body exactly as stored, or nothing when none is set |
| `reef memory [--project P] set FILE\|-` | Sets the project's memory from a file, or from stdin with `-`, replacing any before | `saved memory for project shop` |
| `reef team apply FILE.json` | Creates a team from a file, or replaces the roles and gates of the team with that name (see Teams) | `applied team duo: lead*(claude) architect(claude)` |
| `reef team list` | Lists the teams by name, each with its roles | `duo  lead*(claude) architect(claude)`: the lead has a `*` |
| `reef context <feature> --role R` | Prints the context prompt for role `R` (see below) | the prompt |
| `reef token <feature> --role R` | Makes a channel and a token for role `R` (see Tokens) | a `claude mcp add` command |
| `reef wrap list [<feature>]` | Lists pending wrap-up proposals (see Wrap-up): of the feature, or of the current folder's project | `1  login  3  architect  2026-10-13`: id, feature slug, channel id, role, created date |
| `reef wrap show <id>` | Prints a proposal and its status | `proposal 1 from architect`, status, then brief, notes and decisions |
| `reef wrap approve <id>` | Approves a pending proposal | `approved: brief, notes and 1 decisions saved` |
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
tester, tester to developer on a failure or else to lead) and says to @mention no one else,
because every `@name` wakes that teammate. The tester runs only `go test`, `npm test` and
read-only `git` commands. Apply it with `reef team apply teams/full.json`.

```json
{
  "name": "duo",
  "roles": [
    {"name": "lead", "lead": true, "backend": "claude",
     "persona": "You are the lead. Answer the user. Hand design work to @architect."},
    {"name": "architect", "backend": "claude", "permissionMode": "default",
     "persona": "You are the architect. Reply with a short plan.",
     "skills": "Writing plans.",
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
  A claude role without `write` cannot list plain `Bash` (or `Bash(*)`): `reef team apply`
  rejects it, so list patterns such as `Bash(go test:*)`.
  Codex ignores `tools`. Its limit is `permissionMode`. `write` decides which limit applies
  (see [server.md](server.md#role)).

## Tokens

A token says who a `reef mcp` process works for: one role in one channel. The channel
gives the feature and the project. The role is the author of the decisions the agent
records, the owner of the notes `get_brief` reads, the author of what `post_to_channel`
posts, and the role on a wrap-up proposal. A role of a channel in `reef serve` comes from its
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

Every tool except `post_to_channel` works on the token's feature. On a quick-session channel
(no feature) those tools fail with `this channel has no feature`, except `read_thread` of the
token's own channel. A result is one JSON object as text. A
tool error comes back as a result with `isError: true` and the message as text, so the
agent can read it. Arguments that do not match the tool's schema are a tool error too
(`validating "arguments": ... missing properties: ["query"]`). An unknown tool name is a
JSON-RPC error (`-32602`, `unknown tool "nope"`).

| Tool | Input | Output | Errors |
| --- | --- | --- | --- |
| `get_brief` | none | `{"brief": "...", "notes": "..."}`: the latest brief and this role's notes. Each is `""` when there is none yet | |
| `record_decision` | `body` (required), `why`, `messageId` | the new decision, as in [server.md](server.md#decision). The author is the token's role | `invalid: decision body is required`, `invalid: message 5 is not in a channel of feature 1` |
| `add_task` | `title` (required), `body` | the new task: `todo`, in the feature's project, tagged with the feature | `invalid: task title is required` |
| `search_sessions` | `query` (required), `limit` (default 20) | `{"hits": [{"channelId", "messageId", "author", "snippet"}]}`: messages in all of the feature's channels that contain `query`, newest first. Case is ignored for ASCII. The snippet is about 120 characters around the match. `hits` is `null` when nothing matches | `query is required` |
| `read_decision` | `id` (required) | the decision | `decision 99: not found`, also for a decision of another feature |
| `read_thread` | `channelId` (required), `after` (only newer message ids), `root` (the id of a top-level message) | `{"messages": [...]}`: the channel's messages, oldest first. With `root`: that message and its replies, oldest first, and `after` is ignored. Each message has `parentId`, `null` at the top level. `messages` is `null` when there are none | `channel 9: not found`, also for a channel of another feature or a quick session other than the token's own; `thread 9: not found` when `root` is not a top-level message of the channel |
| `post_to_channel` | `body` (required), `parent` (the id of a top-level message: reply in its thread; leave it out to post at the top level) | the stored message, with the token's role as `author`. It works on a quick session too | `invalid: message body is required`, `invalid: parent 9 is not a top-level message of channel 1` |
| `propose_wrap` | `brief`, `notes`, `decisions` (all required). Each decision is `{"body", "why"}`; `why` may be `""` | `{"id": 1, "message": "proposal saved; the user will approve it"}` | `invalid: proposal brief is required`, `invalid: decision body is required` |
| `browser_navigate` | `url` (required) | `{"text": "navigated to <url>"}` | `browser unavailable: the Reef app is not open`, `browser did not answer: <error>`, `the user has the browser`, `refused: the browser pane cannot open Reef itself` (or another `refused: ...` for a non-http(s) URL), the page's own error (e.g. `net::ERR_CONNECTION_REFUSED`) when it fails to load |
| `browser_screenshot` | none | a PNG image of the browser pane's current page | `browser unavailable: the Reef app is not open`, `browser did not answer: <error>`, `the user has the browser` |
| `browser_snapshot` | none | the page's accessibility tree as text, one line per element: `- role "name" [ref=e12]`. A ref is only valid until the next snapshot or a navigation | `browser unavailable: the Reef app is not open`, `browser did not answer: <error>`, `the user has the browser` |
| `browser_click` | `ref` (required, from `browser_snapshot`) | `{"text": "clicked <ref>"}` | `browser unavailable: the Reef app is not open`, `browser did not answer: <error>`, `the user has the browser`, `ref e12 is not on the page; take a new snapshot`, `cannot click e12: <error>` for any other failure |
| `browser_type` | `ref` (required, from `browser_snapshot`), `text` (required) | `{"text": "typed into <ref>"}` | `browser unavailable: the Reef app is not open`, `browser did not answer: <error>`, `the user has the browser`, `ref e12 is not on the page; take a new snapshot`, `cannot type into e12: <error>` for any other failure |
| `browser_console` | none | `{"text": "..."}`: the console messages and errors since the last navigation, one per line with their level, e.g. `error: boom` | `browser unavailable: the Reef app is not open`, `browser did not answer: <error>`, `the user has the browser` |
| `browser_network` | none | `{"text": "..."}`: the network requests since the last navigation, one per line with method, URL and status, e.g. `GET http://localhost/ 404` | `browser unavailable: the Reef app is not open`, `browser did not answer: <error>`, `the user has the browser` |

`propose_wrap` saves a pending proposal and changes nothing else. See Wrap-up.

The browser tools go straight from `reef mcp` to the Reef app, over `browser.sock` in the data
folder, not through `reef serve` (see [desktop.md](desktop.md#browser-pane)). With no socket, or
a stale one left by a crash, all seven tools fail with `browser unavailable: the Reef app is not
open`. Once connected, a write, read or deadline failure instead fails with `browser did not
answer: <error>`.

`post_to_channel` is how a teammate speaks in the channel besides its reply. `reef serve`
routes the message when the poster's turn ends: every `@role` in the body wakes that teammate
(once, even when the reply names the same role again), and a post that names nobody wakes no
one (see [server.md](server.md#routing)). Plain Claude
Code and `reef chat` have no router, so a post there is only stored.

## The context prompt

`reef context <feature> --role R` prints it, and `reef chat` puts it before the first
message of a new channel. It holds the brief, the role's notes and an index. It never
holds a message body. Its shape:

0. When the role has a persona or skills (a team role, see Teams): the persona, then
   `Your skills:` and the skills, each followed by a blank line. `reef serve` and `reef chat`
   add this before part 1. `reef context` does not print it. A role that wakes with a
   persona or skills that differ from what its session last got, whether it is starting or
   resuming, gets this part again, marked `Your persona or skills changed:`.
1. One line: role, feature name and slug, project name. Then, when a profile exists,
   `You work with Ada, a developer. They like: short answers, ask before big changes.` (occupation
   and style only when set, lower-cased; `Other` is left out). Then the goal (if set) and the stage. A quick session
   adds the same line after its second line.
2. `## Project memory`: the project's memory (see [server.md](server.md#project_memory)), only
   when it is not empty.
3. `## Brief`: the latest brief, or `No brief yet.`
4. `## Your notes`: this role's notes, or `No notes yet.`
5. `## Index`, with `### Decisions` (one line each: id, the first line of the body cut
   at 100 characters with `…`, author, date), `### Channels` (channels with messages, one line each: id, name,
   date, message count) and `### Open tasks` (tasks not `done`). An empty group says
   `none`.
6. One closing line that names the tools for details.
7. When the team has two or more roles, a `## Team` section: `Your team: @lead, @architect.`
   It says that to hand work over you @mention a teammate in a message, which wakes them, and
   that your reply, and anything you send with `post_to_channel`, is posted to the channel
   and nobody sees anything else. A message that wakes you starts with `[channel N, message
   N]` or `[channel N, message N, thread N]`; your plain reply goes where the header says (a
   top-level reply starts your own thread), and `post_to_channel` takes the header's `thread` as
   its `parent` when there is one.
   After the header, a wake may carry `reef wrote: ...` lines: what `reef` posted in the
   channel since the role's last turn began, such as a rollback note (see
   [server.md](server.md#channels-live)). The prompt does not explain them.
   The lead gets one more line: do not summarize or repeat a
   specialist's post back to the user, because they see it. `reef context` does not print it.

A quick session gets the same parts 0, 2 and 7 around its two lines.

Example, for a feature with one approved wrap-up:

```
You are the developer on feature "Email login" (login) in project "shop".
Goal: Users sign in with email and password
Stage: ideation

## Project memory
Sessions are stored server-side, not JWT.

## Brief
Goal: Users sign in with email and password.
Status: ideation. No implementation yet.
Key decisions: Lock account after 5 failed logins (decision 1).
Open questions: lockout duration and unlock method.

## Your notes
Only the lockout threshold is decided so far. Nothing built.

## Index

### Decisions
decision 1: Lock an account after 5 failed logins. (developer, 2026-10-13)

### Channels
channel 1: developer chat, 2026-10-13, 4 messages

### Open tasks
none

Details are on demand through the reef MCP tools read_decision, read_thread and search_sessions. Record decisions with record_decision.
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
one now holds the chat lock. It starts no agent: a teammate's agent starts when that role has
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
  changed since that turn: then part 0 goes before the wake text, marked as changed.
- Without one, the agent starts a new session and the context prompt goes before the
  first message, in the same prompt. A channel on a feature gets the prompt below. A
  quick session (no feature) gets two lines: the role, the project's name and folder, and
  that there is no feature yet.

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
  `reef: agent: <text>`. A turn that ends `failed` for any reason (an error, a stop, the
  tool call cap) still stores the reply streamed so far, if it has text.

Caps in chat: before each line it sends, `reef chat` runs the same check as the server
(`channel.CheckCaps`). A paused channel, or one at or over its token budget (paused first),
does not run the line and does not store it. It prints `channel 1 is paused: <reason>` and
`/continue to go on`, then `> ` again. The line `/continue` clears the pause the way
`POST /channels/{id}/continue` does (`channel.Continue`): a channel at or over its token budget
gets a budget of the tokens it used plus 1,000,000, and it prints `continued`. On a channel that is not paused
it prints `the channel is not paused`. The next line then runs. A tool call cap still
ends the chat, as below; resume the channel and type `/continue`.

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
   is still `/wrap`. The prompt asks the agent to call `propose_wrap` once, with the full
   rewritten brief, its own notes on the feature, and the decisions made in this channel
   that are not recorded yet. Decisions the agent already recorded with `record_decision`
   are not proposed again.
2. `propose_wrap` stores a `pending` proposal (see [server.md](server.md#proposal)).
3. After the turn, `reef chat` looks for a pending proposal of this channel that is newer
   than the one pending before `/wrap`. With none, it prints `no proposal made`.
4. Otherwise it prints the proposal: brief, notes and decisions, then `approve? [y/N]`.
5. The next line typed after the prompt decides. `y` or `yes` (any case) approves.
   Anything else, a blank line too, rejects. `reef chat` reads stdin all the time, so it
   drops lines already queued after first printing `approve? [y/N]`. It prints
   `ignored while wrapping: <line>` for each non-blank dropped line, then prints the prompt
   again. Those lines are not answers.
   If stdin ends at the prompt, or the user presses Ctrl-C there, the proposal stays
   `pending` and it prints `proposal 1 left pending`. After Ctrl-C it then prints the resume line and exits 0.

Approve applies everything in one transaction:

- a new brief version, with the role as author,
- the role's notes, replacing the old ones,
- each proposed decision, with the role as author and no source message. A decision whose
  body is exactly the body of one the feature already has is skipped, so a decision the
  agent recorded live is not saved twice. A repeat inside the proposal is skipped too,
- the proposal marked `approved`, with `decidedAt`.

If any step fails, nothing is applied. It prints `approved: brief, notes and 1 decisions
saved`, counting only the decisions it added.

Reject marks the proposal `rejected` and changes nothing else: `rejected: nothing
changed`. To change a proposal, reject it, tell the agent what to change, and type
`/wrap` again. A proposal is decided once. A second approve or reject fails with
`invalid: proposal 1 is already approved`.

`reef serve` offers the same over REST, and promotes a quick session with the same
approval: see [server.md](server.md#wrap-up-and-promotion).

The chat goes on after `/wrap`. The next session, in any channel, starts from the
approved brief and notes.

A proposal stays `pending` until someone decides it. `reef chat` only asks about the one
`/wrap` just made. Every other pending proposal is decided later with `reef wrap`: one
left pending by end of input or Ctrl-C, one the agent proposed outside `/wrap`, the older
of two from one turn, and every proposal from plain Claude Code (below). `reef wrap list`
shows them, `reef wrap show <id>` prints one, and `reef wrap approve <id>` or `reef wrap
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

Claude Code's own messages are not stored in Reef. Only what the tools write is: decisions,
tasks and proposals. The channel that `reef token` made stays empty, so `search_sessions`
and `read_thread` see only `reef chat` channels. Each `reef token` call makes a new
channel and token.
