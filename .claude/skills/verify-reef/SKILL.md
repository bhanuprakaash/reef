---
name: verify-reef
description: Drive Reef's real surfaces and capture proof. reef-agent (the agent adapters over the claude and codex backends) gives event JSON lines, exit codes and process checks; reef serve (the data server) gives REST calls, SQLite rows, locks and crash recovery; the reef CLI, reef mcp and reef chat (the memory loop) give command output, MCP tool results, chat turns, wrap-up and SQLite rows. Use to verify a ticket or slice acceptance line, to re-check after a change in agent/, cmd/reef-agent, cmd/reef, channel/, server/ or store/, or when asked "does reef-agent / reef serve / reef chat still work". The window (the web UI) is driven in Chrome through `verify-reef ui`. Later slices add the Mac shell; this map grows with them.
---

# Verify Reef

Four user surfaces exist so far:

- `reef-agent` (`cmd/reef-agent`) runs one backend (`claude` or `codex`) and prints one JSON
  `Event` per line. Contract: `docs/agents.md`.
- `reef serve` (`cmd/reef`) keeps Reef's data in one SQLite file and serves it as a JSON
  REST API. It also runs a channel's team: it routes messages between teammates, holds
  gated wakes and caps hops. Contract: `docs/server.md`.
- The memory loop (`cmd/reef`): the CLI (`reef project`, `feature`, `brief`, `decide`,
  `task`, `team`, `context`, `token`), `reef mcp` (stdio MCP tools for one teammate) and `reef
  chat` (one role in one channel, in the terminal). Contract: `docs/memory.md`.
- The window: the React app `reef serve` serves (`web/`). Contract: `docs/server.md` (Web UI).

The Mac shell does not exist yet. When a later slice adds it, add its Launch, Drive and
feature files here. The feature map is in
[`features/README.md`](features/README.md). Read the matching feature file before a run.

All commands below run from the repo root. `H=.claude/skills/verify-reef/scripts/verify-reef`.
Each Bash call starts a new shell, so pass `REEF_VERIFY_RUN=<id>` on every call.

## Launch

Build the binaries once per run. `reef-agent` drives are each their own process in the run's
scratch cwd; the server is started separately (see Server).

```sh
.claude/skills/verify-reef/scripts/verify-reef build
# prints the run id, e.g. 20261013-134053-78591
```

`build` runs `go build` for `./cmd/reef-agent` and `./cmd/reef` into `<scratch>/bin/`, makes
the scratch cwd `<scratch>/cwd/`, and writes
`~/.reef-verify/<id>/build.txt` (HEAD sha, dirty file count, both binary paths). Ready = it
printed an id and exited 0.

`<scratch>` is `/tmp/reef-verify/<id>` on Linux and `/private/tmp/reef-verify/<id>` on macOS: the
helper resolves `/tmp` with `pwd -P`. This skill and its feature files write `<scratch>` for it.
`<key>` is `<scratch>/cwd` with every character outside A-Z a-z 0-9 turned into `-`: claude keeps
the scratch cwd's sessions in `~/.claude/projects/<key>/`.

## Doctor

Read-only. Run it after `build` and whenever anything looks off. Every line must be `ok`.

```sh
REEF_VERIFY_RUN=<id> .claude/skills/verify-reef/scripts/verify-reef doctor
```

It checks `go`, `claude` and `codex` versions, `claude auth status` (`loggedIn`),
`codex login status` (each limited to 20 s, so a hung CLI is a `FAIL` line), that `sqlite3`
and `curl` exist, and that the HEAD `build` recorded in `build.txt` equals `git rev-parse HEAD`
for both `reef-agent` and `reef` (rebuild after a commit). It also saves the pre-run snapshot
of `claude`, `codex` and `reef-agent` pids to `pre-pids.txt`. `run`, `interrupt`,
`serve-start` and `reef` refuse to start without it. Exit 1 if any line is `FAIL`.

## Drive

`run` starts `reef-agent` in the scratch cwd with the args after `--`, waits for it to
exit, and records the result. stdin is `/dev/null` unless `--stdin FILE` gives follow-up
prompts, so the process exits 0 after the last `turn_end`. `--timeout S` (default 240)
sends SIGINT if it runs too long, and the meta file says `timed_out`.

```sh
ANTHROPIC_MODEL=sonnet REEF_VERIFY_RUN=<id> .claude/skills/verify-reef/scripts/verify-reef run --name tool-claude \
  -- --backend claude "Use the Read tool on note.txt in the current directory and tell me its contents."
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
  -- --backend claude \
  "Write a 3000-word essay on the history of lighthouses. Do not use any tools. Output the full essay as plain text."
```

`mcp-server` writes a stdio MCP server (`vping`: tool `secret_word` returns `kumquat-4417`,
tool `sleep_60` runs `sleep 60`) and prints the path of its `--mcp` file. The server appends each
`tools/call` to `~/.reef-verify/<id>/mcp-calls.log`.

```sh
REEF_VERIFY_RUN=<id> .claude/skills/verify-reef/scripts/verify-reef mcp-server
# <scratch>/mcp/mcp.json
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

## Server

`reef serve` is long-lived, so it is the one thing that stays up between Bash calls. One
server per run, driven serially. Read the matching feature file first.

Launch:

```sh
REEF_VERIFY_RUN=<id> .claude/skills/verify-reef/scripts/verify-reef serve-start [--data-name NAME]
# serve: ready pid=... addr=127.0.0.1:PORT data=<scratch>/NAME in 0.4 s
```

It picks a free 127.0.0.1 port, starts `reef serve --addr ... --data <scratch>/NAME` (default
`data`), and waits up to 10 s for `GET /health` to answer 200. Otherwise it fails and prints the
tail of `serve.log`. It refuses if this run's server is already up. While a server runs,
`pgrep -fl 'reef serve'` also lists a `bash ... verify-reef serve-start` line: that is the
wrapper that records the exit code, and it ends with the server.

Drive:

```sh
REEF_VERIFY_RUN=<id> $H api POST /projects '{"name":"shop","path":"/tmp/shop"}'
REEF_VERIFY_RUN=<id> $H api -H 'Host: evil.example' GET /projects
REEF_VERIFY_RUN=<id> $H sql "select id, status, error from turn"
REEF_VERIFY_RUN=<id> $H serve-second --same-addr
REEF_VERIFY_RUN=<id> $H serve-stop --kill
```

- `api [-H 'Name: value']... METHOD PATH [JSON]` calls the running server. It prints `HTTP <status>`
  then the body. POST, PUT and PATCH send `Content-Type: application/json` unless a `-H`
  sets one. Any status exits 0 (an error status is often the proof); a failed connection exits 1.
- `sql [--write] "SQL"` runs `sqlite3` on the run's `reef.db`. Without `--write` the session is
  `query_only`. Output starts with a header line of column names (`sqlite3 -header`); the
  recipes quote only the rows after it. Use `--write` only to set up a state the API cannot
  reach (for example `pragma user_version=99` on a stopped server).
- `serve-stop` sends SIGINT (what Ctrl-C does) and prints the exit code and seconds. With `--kill`
  it sends SIGKILL, a real crash: the server cannot clean up. Only the recorded pid is signaled.
- `serve-second [--same-addr]` starts another `reef serve` on the same data folder, on the same
  addr or a new free port. It must exit within 5 s with `data folder ... is in use by another
  reef`. If it is still running it kills it and exits 1.
- Restart = `serve-stop` (or `--kill`), then `serve-start` with the same `--data-name`. The
  folder stays, the port changes. A different `--data-name` is a fresh, empty server.

Rules:
- One long-lived server per run, driven serially. Never drive a `reef serve` this run did not start.
- Every write sends JSON. A body that is not JSON is only for the 415 check.
- Ids start at 1 on a fresh data folder and are never reused. Each feature file says which
  state it assumes.
- Quote lines of `api.log` (`> METHOD PATH body`, `< status body`) as proof, plus the
  `sql.log` rows for side effects.
- Run `cleanup` at the end. It stops the server if it is still up.

## Memory loop

The CLI, `reef mcp` and `reef chat` open the data folder themselves, so they need no
server. They share the run's data folder with `serve-start` and `sql`. Read the matching
feature file first.

```sh
REEF_VERIFY_RUN=<id> $H reef --data-name cli -- project add
REEF_VERIFY_RUN=<id> $H reef -- decide login "Use sessions, not JWT"
REEF_TOKEN=<token> REEF_VERIFY_RUN=<id> $H mcp-call record_decision '{"body":"Rate limit by IP"}'
ANTHROPIC_MODEL=sonnet REEF_VERIFY_RUN=<id> $H reef --stdin ~/.reef-verify/<id>/chat-in.txt --answer y \
  --timeout 300 -- chat login --role developer --backend claude
```

- `reef [--data-name NAME] [--cwd DIR] [--stdin FILE [--answer A | --sigint-at-prompt]]
  [--timeout S] -- ARGS` runs the `reef` binary in the scratch cwd with `--data <folder>`
  right after the command words (`task add --data <folder> 1 -- "--why not"`), so a `--` in
  ARGS still ends the flags.
  `--data-name` picks `<scratch>/NAME`. It refuses
  while this run's server uses another folder. Without it, the call uses the last folder that
  `reef --data-name` or `serve-start` used, or `<scratch>/data`. stdin is `/dev/null` unless
  `--stdin` gives a file. `--answer A` answers a wrap-up: FILE's lines go through a FIFO held
  open until the live output shows `approve? [y/N]`, then `A` is written and stdin closes
  (see `features/chat.md`). `--sigint-at-prompt` feeds FILE the same way, but at `approve?
  [y/N]` it sends SIGINT to `reef` instead of an answer and keeps stdin open until `reef`
  exits: Ctrl-C at the prompt. `--cwd DIR` runs in `<scratch>/DIR` instead, made if
  missing (`--cwd cwd/sub`, `--cwd cwd2`), for a subfolder or a second project; `reef.log`
  shows that cwd. `--timeout S` (default 240) sends SIGINT, which is Ctrl-C. It prints
  the command's stdout and stderr, then `exit: N elapsed_s: ... children: N orphans_alive:
  ...`, and exits with the command's code.
- `mcp-call TOOL [JSON]` starts `reef mcp` on the run's folder with the caller's `REEF_TOKEN`,
  does the MCP handshake, lists the tools and calls `TOOL` with `JSON` (default `{}`). It
  prints `tools: ...`, `< isError=false|true <text>` (or `< rpc error ...`), `reef mcp exit:
  N` and stderr. It exits 0 when a tool result came back.
- The project folder is the scratch cwd, so `reef chat` runs its agent there and `cleanup`
  removes claude's session state for it.

Rules:
- Every `--backend claude` `reef chat` drive sets `ANTHROPIC_MODEL=sonnet`. Keep chats to the
  lines of the feature file.
- Quote `reef.log` and `reef-mcp.log` lines as proof, plus the `sql.log` rows for side effects.

## Window

The window is driven in Chrome with `playwright-core`, by `scripts/ui/drive.mjs`. On macOS that
is the system Chrome (`/Applications/Google Chrome.app`), no browser download. It needs a running
server, so launch as in Server. Read `features/window.md` first.

On Linux there is no system Chrome (the launch fails with `Chromium distribution 'chrome' is not
found`). Install Playwright's Chromium once, with its system libraries, and set
`REEF_VERIFY_CHROME` to its binary on every `ui` call (each Bash call is a new shell). `ui` then
launches that binary with `--no-sandbox`:

```sh
PLAYWRIGHT_DOWNLOAD_HOST=https://cdn.playwright.dev npx playwright@1.64.0 install chromium
npx playwright@1.64.0 install-deps chromium   # or the same libraries with apt
export REEF_VERIFY_CHROME=$HOME/.cache/ms-playwright/chromium-<rev>/chrome-linux/chrome
```

`1.64.0` is the `playwright-core` version in `scripts/ui/package.json`. The default download host
may be blocked, hence `PLAYWRIGHT_DOWNLOAD_HOST`. `<rev>` is the folder `install` made; `ls` it
for the binary's exact path.

Launch: build the web app first, because `build` embeds `web/dist` in `reef` (a missing build
shows 503 `UI not built`). Start the server with the model set: it starts the `claude` agents.

```sh
(cd web && npm ci && npm run build)
id=$($H build)
REEF_VERIFY_RUN=$id $H doctor
ANTHROPIC_MODEL=sonnet REEF_VERIFY_RUN=$id $H serve-start --data-name ui
```

Drive: one scenario per call. The first call runs `npm ci` in `scripts/ui`.

```sh
REEF_VERIFY_RUN=<id> $H ui live      # claude channel on a feature, tool rows mid-turn, stored reply
REEF_VERIFY_RUN=<id> $H ui brief     # new channel on a feature with a brief, ask the goal
REEF_VERIFY_RUN=<id> $H ui promote   # quick session, two exchanges, promote, approve, brief version 1
REEF_VERIFY_RUN=<id> $H ui cap       # token_budget=1, banner with reason, Continue, next message runs
REEF_VERIFY_RUN=<id> $H ui pages     # bad project path error, stage survives reload, task marked done and filtered
REEF_VERIFY_RUN=<id> $H ui stop      # Stop mid-turn shows Failed: interrupted, next message resumes
REEF_VERIFY_RUN=<id> $H ui wrap      # Wrap up, Reject, /wrap, Approve, brief version 1
REEF_VERIFY_RUN=<id> $H ui teams     # team picker, lead then architect, Thread pane reply, gate Approve
```

`teams` needs the teams `duo` and `gated` applied to the `ui` folder first: run
`$H reef --data-name ui -- team apply FILE.json` for each before `serve-start --data-name ui` (the
files are in `features/teams.md`).

`ui` runs `node scripts/ui/drive.mjs SCENARIO http://<serve.addr>` with the run's data folder, the
scratch cwd as the project folder and `~/.reef-verify/<id>/ui` for screenshots. Each scenario adds
its own project through the form, so any order works on one server. Without the helper, set
`REEF_UI_DATA` (data folder, for `sqlite3`), `REEF_UI_PROJECT` and `REEF_UI_PROOF`, then
`node .claude/skills/verify-reef/scripts/ui/drive.mjs SCENARIO BASE_URL`.

Proof: `PASS <scenario>: ...` and `FAIL ...` lines (exit 1 on any `FAIL`), screenshots
`ui/<scenario>-<step>.png` that you open and look at, and `$H sql` rows (`message`, `turn`, `brief`,
`proposal`, `channel`).

Rules:
- Real `claude` on sonnet only; every scenario costs a few turns. Run each once.
- Never drive a server this run did not start.
- A `FAIL` is a finding only after you have opened its screenshot.

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
| `serve.log` | `reef serve` stderr (`listening on ...`, `marked N running turns failed`), plus one line per `serve-start` and `serve-stop` (signal, pid, exit code, seconds) |
| `api.log` | every `api` call: `> METHOD PATH [headers] [body]`, then `< status body` |
| `sql.log` | every `sql` call: `> SQL`, then its output |
| `serve-second.log` | each `serve-second`: addr, data, exit code, stderr, verdict |
| `reef.log` | every `reef` call: `--- reef` and the exact args with `--data DIR`, cwd and stdin, `out:` and `err:` lines, the `feed:` line with `--answer` or `--sigint-at-prompt`, then `exit:`, `elapsed_s`, `sigint_after_s`, `children`, `orphans_alive` |
| `reef-mcp.log` | every `mcp-call`: `> TOOL JSON`, `tools:`, `< isError=...` result, `reef mcp exit:`, stderr |
| `ui-<scenario>.log`, `ui/<scenario>-<step>.png` | the `PASS`/`FAIL` lines of each `ui` run, and its screenshots |

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

If this run's server is still up it gets SIGINT first. Then it sends SIGTERM, then SIGKILL, only to
pids this run recorded (`reef-agent`, `reef serve`, `reef` commands and the children seen under them), and only
while the pid still runs the recorded command. It never
kills by name: other claude sessions and a managed `codex app-server --listen` daemon may
run on this machine. It removes `<scratch>/` and claude's state for
the scratch cwd (`~/.claude/projects/<key>` and its task
folder under `claude-<uid>/` beside `reef-verify/`), then
lists the evidence dir it kept. The server's data folder goes with the scratch dir; `sql.log`
keeps what you read from it. Run it after every run, failed ones too. Codex threads stay
in `~/.codex/sessions`; cleanup leaves them.

## Helper

`.claude/skills/verify-reef/scripts/verify-reef` (bash; uses go, python3, perl, curl, sqlite3, pgrep, ps, awk).

| Subcommand | Does |
| --- | --- |
| `build` | new run id (or `$REEF_VERIFY_RUN`), builds the binary, writes `build.txt`, prints the id |
| `doctor` | read-only health check, saves `pre-pids.txt` |
| `run [--name N] [--stdin FILE] [--timeout S] -- ARGS` | one `reef-agent` drive, records evidence |
| `interrupt --after S [--name N] -- ARGS` | drive with stdin held open, SIGINT after S seconds |
| `mcp-server` | writes the test MCP server and `mcp.json`, prints its path |
| `serve-start [--data-name NAME]` | starts `reef serve` on a free port, waits for `/health` |
| `serve-stop [--kill]` | SIGINT (or SIGKILL) to the recorded server, prints exit code and seconds |
| `serve-second [--same-addr]` | second `reef serve` on the same data folder; must be refused |
| `api [-H 'N: v']... METHOD PATH [JSON]` | one HTTP call, prints status and body, logs to `api.log` |
| `sql [--write] "SQL"` | `sqlite3` on the run's `reef.db`, logs to `sql.log` |
| `reef [--data-name NAME] [--cwd DIR] [--stdin FILE [--answer A \| --sigint-at-prompt]] [--timeout S] -- ARGS` | one `reef` command in the scratch cwd (or `<scratch>/DIR`) on the run's data folder, logs to `reef.log`; `--answer` answers the wrap-up prompt, `--sigint-at-prompt` sends SIGINT there |
| `mcp-call TOOL [JSON]` | `reef mcp` with `$REEF_TOKEN`: handshake, `tools/list`, one `tools/call`, logs to `reef-mcp.log` |
| `ui live\|brief\|promote\|cap\|pages\|stop\|wrap\|teams` | one browser scenario against this run's server (needs `serve-start` and a built web app), logs to `ui-<scenario>.log`, screenshots in `ui/` |
| `cleanup` | stops the server, kills only this run's pids, removes scratch, keeps evidence |
