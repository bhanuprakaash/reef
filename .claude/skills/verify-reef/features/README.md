# Reef verification map

This directory is the maintained source for verifying what a Reef user can do. So far that
is the `reef-agent` CLI over two backends, `claude` and `codex`, the `reef serve` REST
server, and the memory loop: the `reef` CLI, `reef mcp` and `reef chat`. Read this index,
then use the matching feature file as the recipe. Later slices add the UI; add a file here
for each feature they bring.

## Baseline preconditions

- Run from the repo root. `H=.claude/skills/verify-reef/scripts/verify-reef`.
- `$H build` printed a run id. Pass it as `REEF_VERIFY_RUN=<id>` on every later call.
- `REEF_VERIFY_RUN=<id> $H doctor` shows only `ok` lines: both CLIs logged in, binary built
  from HEAD, pre-run pids saved.
- Every `--backend claude` drive sets `ANTHROPIC_MODEL=sonnet`.
- Never drive a `reef-agent`, `reef serve` or `reef chat` that this run did not start.
- Server features also need `$H serve-start`: one server per run, on a fresh data folder
  (`$H serve-stop`, then `$H serve-start --data-name NAME`). Each server feature file names
  its folder and assumes ids start at 1. Drive it with `$H api`, `$H sql` and `$H serve-*`.

## Driving conventions

- Drive `reef-agent` only through `$H run` and `$H interrupt`. They run the binary in the scratch cwd
  `/private/tmp/reef-verify/<id>/cwd` and record evidence.
- Give each drive a new `--name` that says the feature and backend, e.g. `resume-claude`.
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
- Record the feature id and backend with each proof.
- A sub-feature that does not apply to a backend is reported as not applicable, with the
  reason from its file.
- A drive that could not run is reported as not run, with the command and the reason.
  Never report one backend's proof as proof for the other.

## Feature entry contract

Each feature file starts with an H1 title and one paragraph on the user-visible behavior.
Then exactly four H2 sections, in this order: `Sub-features`, `How to get to it (user POV)`,
`Driving it with verify-reef`, `Gotchas`. Keep implementation details out; name user paths,
commands, required state and observable proof.

## Features

- [Run a turn](./turn.md): one prompt, streamed events, tool calls, permission mode, errors and usage errors.
- [Follow-up prompts](./follow-ups.md): stdin lines become later turns of the same session.
- [Resume a session](./resume.md): `--resume ID` continues an earlier session; a bad id fails cleanly.
- [Interrupt](./interrupt.md): Ctrl-C prints one usage line, exits 130, leaves no processes.
- [MCP servers](./mcp.md): `--mcp FILE` gives the session extra tools, pre-approved.
- [Subagent tool calls](./subagents.md): claude subagent tool calls carry `parentToolId`.

### Server

Drive these with `$H serve-start`, `$H api`, `$H sql`; each assumes a fresh data folder.

- [Server](./server.md): start and stop, data folder and lock, localhost only, request guard, JSON errors, newer database, odd folder names.
- [Projects and features](./projects-features.md): project CRUD, features and slugs, brief versions, role notes.
- [Channels and messages](./channels-messages.md): quick-session and feature channels, filters, append-only messages, cascade.
- [Tasks](./tasks.md): global list, filters, status, `updatedAt`, feature delete keeps the task.
- [Turns and restart recovery](./turns-recovery.md): running, done, failed; end once; recovery after a crash or stop; an open chat's turn is skipped.
- [Decisions](./decisions.md): append-only, author and message rules, cascade.

### Memory loop

Drive these with `$H reef`, `$H mcp-call` and `$H sql`; no server needed. Each names its
data folder with `$H reef --data-name NAME` and assumes ids start at 1.

- [CLI](./cli.md): project, feature, decide, task, brief, context; a write with no server and while `reef serve` runs; `--` args; a slug in two projects; subfolders and `--name`.
- [reef mcp](./reef-mcp.md): tokens, the seven tools through a scripted MCP client, scope and token errors, plain Claude Code.
- [reef chat](./chat.md): a piped turn stored as messages and turns, `/wrap` approve, reject and left pending, resume, Ctrl-C, on claude and codex; Ctrl-C at the prompt, a decision proposed twice, errors, the chat lock.
