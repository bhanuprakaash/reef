# Reef verification map

This directory is the maintained source for verifying what a Reef user can do. So far that
is the `reef-agent` CLI over two backends, `claude` and `codex`. Read this index, then
use the matching feature file as the recipe. Later slices add the server, the `reef` CLI and
the UI; add a file here for each feature they bring.

## Baseline preconditions

- Run from the repo root. `H=.claude/skills/verify-reef/scripts/verify-reef`.
- `$H build` printed a run id. Pass it as `REEF_VERIFY_RUN=<id>` on every later call.
- `REEF_VERIFY_RUN=<id> $H doctor` shows only `ok` lines: both CLIs logged in, binary built
  from HEAD, pre-run pids saved.
- Every `--backend claude` drive sets `ANTHROPIC_MODEL=sonnet`.
- Never drive a `reef-agent` that this run did not start.

## Driving conventions

- Drive only through `$H run` and `$H interrupt`. They run the binary in the scratch cwd
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
