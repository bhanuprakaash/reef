---
name: verify-reef
description: Drive Reef's real reef-agent CLI (the agent adapters, Reef's only surface so far) over the claude and codex backends and capture proof - event JSON lines, exit codes, process checks. Use to verify a ticket or slice acceptance line, to re-check the adapters after a change in agent/ or cmd/reef-agent, or when asked "does reef-agent still work". Later slices add the server and UI; this map grows with them.
---

# Verify Reef

Today the only user surface is the `reef-agent` CLI (`cmd/reef-agent`). It runs one backend
(`claude` or `codex`) and prints one JSON `Event` per line. The server, the `reef` CLI, the
UI and the Mac shell do not exist yet. When a later slice adds one, add its Launch, Drive
and feature files here.

The contract and every backend detail are in `docs/agents.md`. The feature map is in
[`features/README.md`](features/README.md). Read the matching feature file before a run.

All commands below run from the repo root. `H=.claude/skills/verify-reef/scripts/verify-reef`.
Each Bash call starts a new shell, so pass `REEF_VERIFY_RUN=<id>` on every call.

## Launch

There is no server to keep alive. Launch means: build the binary once per run, then start
each drive as its own process in the run's scratch cwd.

```sh
.claude/skills/verify-reef/scripts/verify-reef build
# prints the run id, e.g. 20261013-134053-78591
```

`build` runs `go build ./cmd/reef-agent` into `/private/tmp/reef-verify/<id>/bin/`, makes
the scratch cwd `/private/tmp/reef-verify/<id>/cwd/`, and writes
`~/.reef-verify/<id>/build.txt` (HEAD sha, dirty file count). Ready = it printed an id and
exited 0.

## Doctor

Read-only. Run it after `build` and whenever anything looks off. Every line must be `ok`.

```sh
REEF_VERIFY_RUN=<id> .claude/skills/verify-reef/scripts/verify-reef doctor
```

It checks `go`, `claude` and `codex` versions, `claude auth status` (`loggedIn`),
`codex login status`, and that the HEAD `build` recorded in `build.txt` equals `git rev-parse HEAD`
(rebuild after a commit). It also saves the pre-run snapshot of `claude`, `codex` and
`reef-agent` pids to `pre-pids.txt`. `run` and `interrupt` refuse to start without it.
Exit 1 if any line is `FAIL`.

## Drive

`run` starts `reef-agent` in the scratch cwd with the args after `--`, waits for it to
exit, and records the result. stdin is `/dev/null` unless `--stdin FILE` gives follow-up
prompts, so the process exits 0 after the last `turn_end`. `--timeout S` (default 240)
sends SIGINT if it runs too long, and the meta file says `timed_out`.

```sh
ANTHROPIC_MODEL=sonnet REEF_VERIFY_RUN=<id> .claude/skills/verify-reef/scripts/verify-reef run --name turn-claude \
  -- --backend claude --permission-mode bypassPermissions "Run the shell command: echo reef-ok"
REEF_VERIFY_RUN=<id> .claude/skills/verify-reef/scripts/verify-reef run --name turn-codex \
  -- --backend codex --permission-mode workspace-write "Run the shell command: echo reef-ok"
REEF_VERIFY_RUN=<id> .claude/skills/verify-reef/scripts/verify-reef run --name followups --stdin /path/to/prompts.txt \
  -- --backend codex "Say one"
```

`interrupt` does the same, but holds stdin open (a live session, as at a terminal) and
sends SIGINT to `reef-agent` `--after S` seconds. That is what Ctrl-C does: `claude` and
`codex` run in their own process group, so the signal reaches only `reef-agent`.

```sh
ANTHROPIC_MODEL=sonnet REEF_VERIFY_RUN=<id> .claude/skills/verify-reef/scripts/verify-reef interrupt --after 12 --name int-claude \
  -- --backend claude --permission-mode bypassPermissions \
  "Run this exact shell command in the foreground, not in the background: python3 -c 'import time; time.sleep(60)'"
```

`mcp-server` writes a stdio MCP server (`vping`, one tool `secret_word` that returns
`kumquat-4417`) and prints the path of its `--mcp` file. The server appends each
`tools/call` to `~/.reef-verify/<id>/mcp-calls.log`.

```sh
REEF_VERIFY_RUN=<id> .claude/skills/verify-reef/scripts/verify-reef mcp-server
# /private/tmp/reef-verify/<id>/mcp/mcp.json
```

Both `run` and `interrupt` print the meta and exit with `reef-agent`'s exit code.
`--name` must be new within the run.

Rules for live runs:
- Every `--backend claude` run sets `ANTHROPIC_MODEL=sonnet`. The env passes through
  `reef-agent` to `claude`.
- Keep prompts short and runs few. One prompt per acceptance line.
- To resume, drive in the same run: claude stores sessions per cwd, and every drive of a
  run shares one scratch cwd.
- `claude -p` loads the user's hooks and settings. A hook may block a command (a standalone
  `sleep` is blocked) and hook processes show up as children. Read the `tool_result` before
  calling a drive wrong.

## Evidence

Evidence goes to `~/.reef-verify/<id>/`, outside scratch. Cleanup never touches it.

| File | What |
| --- | --- |
| `build.txt` | repo, HEAD sha, dirty file count, binary path |
| `doctor.txt` | the doctor lines |
| `pre-pids.txt` | `claude`/`codex`/`reef-agent` pids before any drive |
| `<name>.jsonl` | `reef-agent` stdout, one Event per line |
| `<name>.stderr` | `reef-agent` stderr (`reef-agent: agent error: ...`, usage text) |
| `<name>.meta` | args, cwd, `ANTHROPIC_MODEL`, exit, elapsed, SIGINT time, event kind counts, last line, child count, orphans, new pids since doctor |
| `<name>.children` | every child process of `reef-agent` seen during the drive, `pid command` |
| `mcp.json`, `mcp-calls.log` | the `--mcp` file used, and one line per MCP tool call the server got |

Proof standards:
- Drive the real binary against the real `claude` and `codex`. The fake binaries in
  `agent/*/testdata` are for `go test`, not for proof.
- Quote the event lines that prove the line (`grep` the jsonl), plus `exit:` from the meta.
- Check side effects, not only output: files the agent wrote in the scratch cwd (list them
  before cleanup and copy the listing into evidence), `mcp-calls.log`, `orphans_alive`.
- Orphans: `orphans_alive` lists child processes of `reef-agent` (sampled every 0.2 s
  while it ran) that are still alive after it exits. It must be `none`.
  `new_pids_since_doctor` diffs against `pre-pids.txt`. Other claude sessions on this
  machine can show up there, so read it as a hint, never as proof alone.
- A line you could not drive is reported as not run, with the command and the reason.

## Cleanup

```sh
REEF_VERIFY_RUN=<id> .claude/skills/verify-reef/scripts/verify-reef cleanup
```

It sends SIGTERM, then SIGKILL, only to pids this run recorded (`reef-agent` and the
children seen under it), and only while the pid still runs the recorded command. It never
kills by name: other claude sessions and a managed `codex app-server --listen` daemon may
run on this machine. It removes `/private/tmp/reef-verify/<id>/` and claude's state for
the scratch cwd (`~/.claude/projects/-private-tmp-reef-verify-<id>-cwd` and its task
folder under `/private/tmp/claude-<uid>/`), then
lists the evidence dir it kept. Run it after every run, failed ones too. Codex threads stay
in `~/.codex/sessions`; cleanup leaves them.

## Helper

`.claude/skills/verify-reef/scripts/verify-reef` (bash; uses go, python3, pgrep, ps, awk).

| Subcommand | Does |
| --- | --- |
| `build` | new run id (or `$REEF_VERIFY_RUN`), builds the binary, writes `build.txt`, prints the id |
| `doctor` | read-only health check, saves `pre-pids.txt` |
| `run [--name N] [--stdin FILE] [--timeout S] -- ARGS` | one `reef-agent` drive, records evidence |
| `interrupt --after S [--name N] -- ARGS` | drive with stdin held open, SIGINT after S seconds |
| `mcp-server` | writes the test MCP server and `mcp.json`, prints its path |
| `cleanup` | kills only this run's pids, removes scratch, keeps evidence |
