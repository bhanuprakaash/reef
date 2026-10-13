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

`reef chat` also locks `chat-<channel id>.lock` in the data folder while it runs. A
second `reef chat` on the same channel exits with `channel 1 is open in another reef
chat`. When `reef serve` starts, it fails the turns that a dead server left running (see
[server.md](server.md#restart-recovery)). It skips a channel whose chat lock is held,
because that turn is still live in a `reef chat` process.

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
| `reef context <feature> --role R` | Prints the context prompt for role `R` (see below) | the prompt |
| `reef token <feature> --role R` | Makes a channel and a token for role `R` (see Tokens) | a `claude mcp add` command |
| `reef chat ...` | Runs a channel in the terminal (see below) | the conversation |
| `reef mcp` | Serves the MCP tools over stdio (see below) | MCP only |

## Tokens

A token says who a `reef mcp` process works for: one role in one channel. The channel
gives the feature and the project. The role is the author of the decisions the agent
records, the owner of the notes `get_brief` reads, and the role on a wrap-up proposal.
Roles are free text for now.

- A token is 64 hex characters (32 random bytes). It is stored in the `token` table (see
  [server.md](server.md#token)).
- `reef token` makes one, in a new channel named `<role> (claude code)`. Every start of
  `reef chat`, new or resumed, makes one for its channel, and deletes it when the chat
  ends. The tokens of `reef token` stay.
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

Every tool works on the token's feature. On a quick-session channel (no feature) every
tool fails with `this channel has no feature`. A result is one JSON object as text. A
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
| `read_thread` | `channelId` (required), `after` (only newer message ids) | `{"messages": [...]}`: the channel's messages, oldest first. `messages` is `null` when there are none | `channel 9: not found`, also for a channel of another feature or a quick session |
| `propose_wrap` | `brief`, `notes`, `decisions` (all required). Each decision is `{"body", "why"}`; `why` may be `""` | `{"id": 1, "message": "proposal saved; the user will approve it"}` | `invalid: proposal brief is required`, `invalid: decision body is required` |

`propose_wrap` saves a pending proposal and changes nothing else. See Wrap-up.

## The context prompt

`reef context <feature> --role R` prints it, and `reef chat` puts it before the first
message of a new channel. It holds the brief, the role's notes and an index. It never
holds a message body. Its shape:

1. One line: role, feature name and slug, project name. Then the goal (if set) and the
   stage.
2. `## Brief`: the latest brief, or `No brief yet.`
3. `## Your notes`: this role's notes, or `No notes yet.`
4. `## Index`, with `### Decisions` (one line each: id, the first line of the body cut
   at 100 characters with `…`, author, date), `### Channels` (one line each: id, name,
   date, message count) and `### Open tasks` (tasks not `done`). An empty group says
   `none`.
5. One closing line that names the tools for details.

Example, for a feature with one approved wrap-up:

```
You are the developer on feature "Email login" (login) in project "shop".
Goal: Users sign in with email and password
Stage: ideation

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
reef chat --channel ID [--permission-mode M] [--debug]
```

`reef chat` runs one role in one channel, in the terminal. It uses the agent adapters
from [agents.md](agents.md). The runner is in package `channel` and has no terminal code,
so the server can drive it later.

New channel (`<feature> --role R --backend B`):

- It checks the backend first, so a typo stores nothing. It builds the context prompt,
  then makes a channel named `N` (default `<role> chat`) on the feature. So the new
  channel is not in its own index. If the agent does not start, the channel is deleted.
- It starts the agent in the project's folder, with one MCP server named `reef`: this
  `reef` binary, `mcp --data <data folder>`, with a new token in `REEF_TOKEN`. Its tools
  need no approval (see [agents.md](agents.md#gotchas)).
- The context prompt goes before the first message, in the same prompt.

Resume (`--channel ID`):

- Role, backend and session id come from the channel's newest turn that has a session
  id. The agent resumes that session, so the context prompt is not sent again.
- First it fails any turn the channel left running (error `chat exited`). That turn
  belonged to a `reef chat` that died, because this one now holds the chat lock.
- A channel with no session fails: `channel 1 has no session to resume`.

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
  `reef: agent: <text>`.

Leaving:

- At the end of stdin (Ctrl-D, or the end of a piped file) it prints `channel 1: resume
  with reef chat --channel 1` and exits 0.
- Ctrl-C during a turn interrupts the agent and stores the turn `failed` with error
  `interrupted`. The part of the reply already printed is not stored. Then it prints the
  same resume line and exits 0.

Flags:

- `--permission-mode M` goes to the backend as is. See [agents.md](agents.md#gotchas) for
  the values. Without it, Claude denies tools that need permission. The `reef` tools are
  allowed anyway.
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
5. The next line decides. `y` or `yes` (any case) approves. Anything else rejects. Lines
   typed while the agent was still wrapping are not the answer: it prints `ignored while
   wrapping: <line>` for each, before `approve? [y/N]`. If stdin ends at the prompt, the
   proposal stays `pending` and it prints `proposal 1 left pending`.

Approve applies everything in one transaction:

- a new brief version, with the role as author,
- the role's notes, replacing the old ones,
- each proposed decision, with the role as author and no source message,
- the proposal marked `approved`, with `decidedAt`.

If any step fails, nothing is applied. It prints `approved: brief, notes and 1 decisions
saved`.

Reject marks the proposal `rejected` and changes nothing else: `rejected: nothing
changed`. To change a proposal, reject it, tell the agent what to change, and type
`/wrap` again. A proposal is decided once. A second approve or reject fails with
`invalid: proposal 1 is already approved`.

The chat goes on after `/wrap`. The next session, in any channel, starts from the
approved brief and notes.

Only `reef chat` approves proposals. A proposal made from plain Claude Code (below) stays
`pending`.

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
