# Run a turn

A user runs `reef-agent --backend claude|codex "prompt"`. It starts the backend in the cwd,
sends the prompt, and prints one JSON event per line as the turn streams: the session id,
text deltas, tool calls and their results, token usage (one `usage` line per model message), then `turn_end`. With
stdin closed it exits 0 after the turn. A failed turn ends with one `error` line (usually right after a `usage` line) and exits 1.

## Sub-features

- `turn-stream` prints `session`, `text`, `usage` and `turn_end` lines for one prompt; exit 0.
- `turn-tool` prints a `tool_call` and a `tool_result` with the same `toolId`.
- `turn-permission` passes `--permission-mode` through: allowed tools run, denied tools give a `tool_result` with `isError`.
- `turn-error` a failed backend ends with one `error` line (or gives only a stderr message) and exit 1.
- `turn-usage-error` a bad command line prints the usage text on stderr and exits 1.

## How to get to it (user POV)

- `reef-agent --backend claude [--cwd DIR] [--permission-mode MODE] "prompt"` in a terminal.
- `reef-agent --backend codex [--cwd DIR] [--permission-mode MODE] "prompt"` in a terminal.

## Driving it with verify-reef

Preconditions:

- Baseline from the README: `build` and `doctor` done, `ANTHROPIC_MODEL=sonnet` on claude drives.

- **Stream, claude.** Write `reef-read-ok` to `/private/tmp/reef-verify/<id>/cwd/note.txt`. Run `ANTHROPIC_MODEL=sonnet REEF_VERIFY_RUN=<id> $H run --name tool-claude -- --backend claude "Use the Read tool on note.txt in the current directory and tell me its contents."`. Meta shows `exit: 0` and `kinds:` with `session`, `text`, `tool_call`, `tool_result`, `usage`, `turn_end`. The last line is `{"kind":"turn_end"}`.
- **Tool pair, claude.** `grep -E '"tool_(call|result)"' ~/.reef-verify/<id>/tool-claude.jsonl`. A `tool_call` with `"toolName":"Read"` and a `file_path` ending in `/cwd/note.txt`, and a `tool_result` with the same `toolId`, no `isError`, and output containing `reef-read-ok`. In the jsonl the `tool_call` comes right after a `usage` line: `grep -n -E '"kind":"(usage|tool_call)"' ~/.reef-verify/<id>/tool-claude.jsonl` shows `usage` before each `tool_call` (claude sends a message's tool calls after its usage, so a turn stopped at a call has counted that model call).
- **Stream, codex.** Run `REEF_VERIFY_RUN=<id> $H run --name turn-codex -- --backend codex --permission-mode workspace-write "Run the shell command: echo reef-ok"`. `exit: 0`. The first line is `{"kind":"session",...}`. A `tool_call` with `"toolName":"commandExecution"` and a `tool_result` with the same `toolId` whose output contains `reef-ok`.
- **Denied tool, claude.** Run `ANTHROPIC_MODEL=sonnet REEF_VERIFY_RUN=<id> $H run --name deny-claude -- --backend claude "Create the file deny.txt with the Write tool, content x"`. A `tool_result` with `"isError":true`, and the scratch cwd has no `deny.txt` (`ls /private/tmp/reef-verify/<id>/cwd`).
- **Read-only sandbox, codex.** Run `REEF_VERIFY_RUN=<id> $H run --name deny-codex -- --backend codex --permission-mode read-only "Run the shell command: touch deny.txt"`. Either a `tool_result` with `"isError":true` or the model refuses; in both cases no `deny.txt` in the scratch cwd.
- **Bad permission value.** Run `ANTHROPIC_MODEL=sonnet REEF_VERIFY_RUN=<id> $H run --name badmode-claude -- --backend claude --permission-mode bogus "hi"`. `exit: 1`; the last line is `{"kind":"error","text":"claude exited: exit status 1: ..."}` listing the valid modes. If claude exits before the prompt is written, stdout is empty and stderr says `reef-agent: write prompt: ...` instead; both are a pass. Codex: `--permission-mode bogus` gives `exit: 1`, empty jsonl, and stderr `reef-agent: thread/start: ...`.
- **Usage error.** Run `REEF_VERIFY_RUN=<id> $H run --name usage-noprompt -- --backend claude` and `REEF_VERIFY_RUN=<id> $H run --name usage-backend -- --backend nope "hi"`. Both `exit: 1`, empty jsonl. Stderr shows `reef-agent: usage: reef-agent --backend claude|codex ...` and `reef-agent: unknown backend "nope"`.

## Gotchas

- Claude's first `session` line comes after the prompt is sent, and repeats every turn. Codex prints `session` once, first.
- `text` lines are deltas. Join them to read the reply; never assert on one line.
- Claude with no `--permission-mode` cannot ask, so it denies tools that need permission (Write, Bash) and runs read-only ones (Read). The recipes avoid `bypassPermissions`: the auto-mode safety check on this machine refuses drives that use it.
- Claude sends a `usage` line as each model message ends and the rest at its `result` line, so a crash mid-turn gives an `error` after the usage of the messages that ended. A top-level `tool_call` follows the `usage` of its own message; a subagent's calls (`parentToolId` set) have no message usage and come at once.
- Claude loads the user's hooks, plugins and settings, so start-up takes a few seconds. That is not a hang.
- `reef-agent` without `--cwd` uses its own cwd; the helper runs it in the scratch cwd. Claude reports `/private/tmp/...`, not `/tmp/...`.
- Codex sends each agent message as its own item; between two messages of one turn the adapter emits a `text` line of `"\n\n"`, so text before and after a tool call stays apart.
