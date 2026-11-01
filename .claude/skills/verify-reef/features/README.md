# Reef verification map

This directory is the maintained source for verifying what a Reef user can do. So far that
is the `reef-agent` CLI over two backends, `claude` and `codex`, the `reef serve` REST
server, the memory loop: the `reef` CLI, `reef mcp` and `reef chat`, and the window (the web
app `reef serve` serves). Read this index, then use the matching file as the recipe.
Later slices add the Mac shell; add a file here for each thing they bring.

## Baseline preconditions

- Run from the repo root. `H=.claude/skills/verify-reef/scripts/verify-reef`.
- `$H build` printed a run id. Pass it as `REEF_VERIFY_RUN=<id>` on every later call.
- `REEF_VERIFY_RUN=<id> $H doctor` shows only `ok` lines: both CLIs logged in, binary built
  from HEAD, pre-run pids saved.
- Every `--backend claude` drive sets `ANTHROPIC_MODEL=sonnet`.
- Never drive a `reef-agent`, `reef serve` or `reef chat` that this run did not start.
- A Space on a work item runs in a git worktree of its project. Before any recipe
  that wakes an agent on a work item, make the scratch cwd a git repo with one commit:
  `git -C <scratch>/cwd init -q -b main && git -C <scratch>/cwd -c user.name=v -c user.email=v@example.com commit -q --allow-empty -m init`.
  A work item's branch `work-item/<id>` can be checked out in one worktree only, so two data
  folders of one run cannot both wake a work item with the same id on the scratch cwd: the
  second fails with `branch work-item/<id> is checked out at <first data folder>/worktrees/<project id>/<id>; switch that checkout to another branch`.
- Server files also need `$H serve-start`: one server per run, on a fresh data folder
  (`$H serve-stop`, then `$H serve-start --data-name NAME`). Each server file names
  its folder and assumes ids start at 1. Drive it with `$H api`, `$H sql` and `$H serve-*`.

## Driving conventions

- Drive `reef-agent` only through `$H run` and `$H interrupt`. They run the binary in the scratch cwd
  `<scratch>/cwd` and record evidence. `<scratch>` and `<key>` are defined in `SKILL.md` (Launch).
- Give each drive a new `--name` that says what it drives and the backend, e.g. `resume-claude`.
- Treat commands as literal. Prompts that name a shell command must keep it exact, so the
  `tool_call` input can be matched.
- Keep live runs few and short. One drive per sub-feature per backend.
- Run `$H cleanup` at the end, also after a failed drive.

## Proof and skip reporting

- Proof is the event lines from `~/.reef-verify/<id>/<name>.jsonl` (quote them with
  `grep`), the `exit:` line of `<name>.meta`, and any side effect (files in the scratch
  cwd, `mcp-calls.log`, `orphans_alive: none`).
- For the memory loop, proof is the lines of `reef.log` (each `reef` call), `reef-mcp.log`
  (each `mcp-call`) and `sql.log` (the rows written).
- Record the sub-feature id and backend with each proof.
- Claude's auto-memory (a user setting) can write a remembered word to a memory file in the scratch cwd, so a recall-only proof (`heliotrope`, `pineapple`) is weak: the same session id is the proof of a resume.
- A sub-feature that does not apply to a backend is reported as not applicable, with the
  reason from its file.
- A drive that could not run is reported as not run, with the command and the reason.
  Never report one backend's proof as proof for the other.

## Entry contract

Each file starts with an H1 title and one paragraph on the user-visible behavior.
Then exactly four H2 sections, in this order: `Sub-features`, `How to get to it (user POV)`,
`Driving it with verify-reef`, `Gotchas`. Keep implementation details out; name user paths,
commands, required state and observable proof.

## Files

- [Run a turn](./turn.md): one prompt, streamed events, tool calls, permission mode, errors and usage errors.
- [Follow-up prompts](./follow-ups.md): stdin lines become later turns of the same session.
- [Resume a session](./resume.md): `--resume ID` continues an earlier session; a bad id fails cleanly.
- [Interrupt](./interrupt.md): Ctrl-C prints one usage line, exits 130, leaves no processes.
- [MCP servers](./mcp.md): `--mcp FILE` gives the session extra tools, pre-approved.
- [Subagent tool calls](./subagents.md): claude subagent tool calls carry `parentToolId`.

### Server

Drive these with `$H serve-start`, `$H api`, `$H sql`; each assumes a fresh data folder.

- [Server](./server.md): start and stop, data folder and lock, localhost only, request guard, JSON errors, newer database, odd folder names.
- [Projects and work items](./work-items.md): project CRUD, work items with acceptance lines and sub-items, brief versions, the role notes and decision routes (nothing reads them since memory items), a lead's sub-items and a Bot's proposed work item.
- [Global chats and Spaces](./global-chats.md): a chat or Space with no Project, created, restarted and reopened with the same history and session; it runs in `<data>/chats/<id>`, no git repo and no project path in its prompt; New chat keeps the older chat; `?project=` filters by association and changes nothing; association routes; the window's All projects filter (`$H ui global-chats`).
- [Spaces and messages](./spaces.md): free, work item and conversation Spaces, filters, append-only messages, cascade; send, the event stream, stop and resume, `/wrap` and `/attach` text, wrap-up, attach, approve and reject, caps and continue, delete a live Space. Needs `ANTHROPIC_MODEL=sonnet` on `serve-start`.
- [Turns and restart recovery](./turns-recovery.md): running, done, failed; end once; recovery after a crash or stop; an open chat's turn is skipped.
- [Bots and Teams](./bots.md): Bots and their REST routes, Team members and delete, `reef team apply` and `list`, a Space on a team, threads, one agent session per Bot, routing by thread, `@mention` and `post_to_space`, direct requests, wrap wakes nobody, restart, a codex Bot on the test fixture, providers. Needs `ANTHROPIC_MODEL=sonnet` on `serve-start`.
- [Caps](./caps.md): the hop cap counts talk-only turns (Edit and Bash ping-pong runs, a talk-only loop pauses with the work item in Needs you), one token budget per work item.
- [Direct conversations](./conversations.md): start a conversation with a Bot, add a Bot with `@name` or the action to move it to a Space, two conversations with their own sessions, the list newest first, continue an old one by resume, the Bot's profile, the sidebar. Needs `ANTHROPIC_MODEL=sonnet` on `serve-start`.
- [Team queue and review](./queue-and-review.md): approve work items; each starts at once with its Space and first prompt, with no Team or Bot limit, the lead's sub-items, submit for review with proof, the next item starting live, send back and Done. Needs `ANTHROPIC_MODEL=sonnet` on `serve-start` and a git repo as the scratch cwd. `$H feature queue-and-review` runs the queue lines, `$H feature work-item-flow` the whole flow.
- [Full team](./full-team.md): `write` per Bot on claude and codex, `teams/full.json`, one worktree per work item, one writer per tree, checkpoints, rollback, retry, the inbox and its stream, the window's Retry, Rollback and Inbox, and the Google login run end to end with no approvals. Needs `ANTHROPIC_MODEL=sonnet` on `serve-start` and a git repo as the scratch cwd.
- [GitHub poll](./github.md): a fake `gh` on `PATH`, a PR note and its dedupe, a finished check's note, a failed check's note and developer wake keyed by run, a restart, a solo-team Space (note only) and no `gh` on `PATH`. Needs `ANTHROPIC_MODEL=sonnet` on `serve-start` and a git repo as the scratch cwd.

### Work item flow

`$H feature work-item-flow` drives the `flow-*` lines of the files above (queue, sub-items, developer and tester, submit for review, send back, Done, a conversation with a Bot added, a proposed work item) and the `hop-*` lines of `caps.md` on the scripted `claude`, in three sections that run at the same time. One command, one PASS/FAIL line per sub-feature.

### Memory loop

Drive these with `$H reef`, `$H mcp-call` and `$H sql`; no server needed. Each names its
data folder with `$H reef --data-name NAME` and assumes ids start at 1.

- [CLI](./cli.md): project, work-item, decide, brief, context; a write with no server and while `reef serve` runs; `--` args; subfolders and `--name`.
- [reef mcp](./reef-mcp.md): tokens, the eight tools through a scripted MCP client, scope and token errors, plain Claude Code.
- [reef chat](./chat.md): a piped turn stored as messages and turns, `/wrap` approve, reject and left pending, `reef wrap` list/show/approve/reject, resume, Ctrl-C, on claude and codex; Ctrl-C at the prompt, a decision proposed twice, errors, the chat lock.

### Window

Drive this with `$H ui SCENARIO` against a server started with `ANTHROPIC_MODEL=sonnet $H serve-start`, after building the web app.

- [Window](./window.md): live Space with tool rows, a new Space that answers from the brief, attach a free Space to a work item, a cap banner and Continue, teams (team picker, Thread pane, Approve and Reject: `$H ui teams`, setup in [Teams](./teams.md)).

### Desktop app

Drive this with `$H desktop` after `build` and `doctor`; it builds, launches and quits the app itself.

- [Desktop app](./desktop.md): the app starts its own server, window screenshot, every browser tool through `reef mcp`, take over, stale refs, the terminal pane on a work item's worktree, quit with no process left, the browser unavailable after quit.
