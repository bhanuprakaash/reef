# Interrupt

A user presses Ctrl-C while `reef-agent` runs. It stops the backend and every process the
backend started, prints one final `usage` line with the session's token total, and exits 130. This
holds mid-turn, mid-tool, between turns, and (codex) while the backend is still starting.

## Sub-features

- `interrupt-exit` SIGINT gives exit 130 within a few seconds.
- `interrupt-usage` the last stdout line is one `usage` line equal to the sum of all `usage` lines before it (all zeros if no turn finished).
- `interrupt-no-orphans` no child process of `reef-agent`, including a running tool command, is alive afterwards.
- `interrupt-idle` SIGINT between turns, while waiting for a stdin line, behaves the same.
- `interrupt-starting` SIGINT while codex is still starting gives the zero `usage` line and exit 130.

## How to get to it (user POV)

- Press Ctrl-C in the terminal running `reef-agent`. The terminal sends SIGINT to `reef-agent` only: the backend runs in its own process group.

## Driving it with verify-reef

Preconditions:

- Baseline from the README. `pre-pids.txt` exists (doctor ran).

- **Mid-turn, claude.** Run `ANTHROPIC_MODEL=sonnet REEF_VERIFY_RUN=<id> $H interrupt --after 12 --name int-claude -- --backend claude "Write a 3000-word essay on the history of lighthouses. Do not use any tools. Output the full essay as plain text."`. Meta shows `exit: 130`, `exit_after_sigint_s` under 3, `orphans_alive: none`, and no `turn_end` in `kinds:`. `int-claude.children` lists `claude -p ...`.
- **Mid-tool, claude.** Run `REEF_VERIFY_RUN=<id> $H mcp-server`, then `ANTHROPIC_MODEL=sonnet REEF_VERIFY_RUN=<id> $H interrupt --after 12 --name int-tool-claude -- --backend claude --mcp <scratch>/mcp/mcp.json "Call the sleep_60 tool of the vping MCP server."`. `exit: 130`, `orphans_alive: none`. The jsonl has a `tool_call` with `"toolName":"mcp__vping__sleep_60"` and no `tool_result` with its `toolId` (claude's own `ToolSearch` call before it may have one). `int-tool-claude.children` lists the `server.py` MCP server and its `sleep 60` child. MCP tools are pre-approved, so this needs no `bypassPermissions`.
- **Usage line.** The last line is `{"kind":"usage","usage":{...}}` and it is the only line after the last event of the backend. Mid first turn, before any model message has ended, its counts are all `0`. To check a non-zero total, interrupt between turns (next step) or after a few tool calls (Mid-turn, claude).
- **Mid-turn, claude.** Run `ANTHROPIC_MODEL=sonnet REEF_VERIFY_RUN=<id> $H interrupt --after 25 --name int-usage-claude -- --backend claude "Run these one at a time as separate Bash tool calls, in order: sleep 5; echo b1. Then sleep 5; echo b2. Then sleep 5; echo b3. Then b4, b5, b6 the same way. Then say done."`. `exit: 130`, no `turn_end`, at least two `tool_call` lines. Each message that ended gave a `usage` line, and the last line is their sum with non-zero `inputTokens` and `outputTokens`.
- **Between turns, codex.** Run `REEF_VERIFY_RUN=<id> $H interrupt --after 25 --name int-idle-codex -- --backend codex "Reply ok."`. The turn ends (`turn_end`) before SIGINT. `exit: 130`. The last line is a `usage` line equal to the sum of the `usage` lines before it. `orphans_alive: none`.
- **Mid-tool, codex.** Run `REEF_VERIFY_RUN=<id> $H interrupt --after 20 --name int-codex -- --backend codex --permission-mode workspace-write "Run this exact shell command in the foreground, not in the background: python3 -c 'import time; time.sleep(60)'"`. `exit: 130`, `orphans_alive: none`; `int-codex.children` lists the `time.sleep(60)` process if it had started. The last line is a `usage` line with non-zero `inputTokens` and `outputTokens`, equal to the sum of the `usage` lines before it (the one after the `tool_call` came from `turn/interrupt`).
- **While starting, codex.** Run `REEF_VERIFY_RUN=<id> $H interrupt --after 0.3 --name int-start-codex -- --backend codex "Reply ok."`. `exit: 130`, the jsonl ends with one zero `usage` line, `orphans_alive: none`. If the handshake finished within 0.3 s a `session` line comes first; that is still a pass.

## Gotchas

- The user's claude hooks load in `-p` sessions and one blocks a standalone `sleep 60`. Claude then retries it with `run_in_background` and ends the turn, so SIGINT lands between turns. Use the `python3 ... time.sleep(60)` prompt for a mid-tool interrupt.
- A `run_in_background` tool is still a child of `claude`, so it shows in `<name>.children` and must be gone too.
- Pick `--after` so SIGINT lands in the state you want. Read `kinds:` and the jsonl to confirm where it landed (no `turn_end` = mid-turn).
- Claude needs about 1.5 s to exit on SIGTERM, so the adapter's 1 s grace usually ends in SIGKILL. `exit_after_sigint_s` near 1 to 2 is normal.
- `orphans_alive` only knows children seen in 0.2 s samples. A tool that started and ended between samples is not listed; that is not an orphan.
- `new_pids_since_doctor` can list other claude sessions started on this machine meanwhile. Use `orphans_alive` for the verdict.
- Codex sends a model call's usage only after the call's tool finishes. `Interrupt` sends `turn/interrupt` first, and codex then reports the call the turn was in (before: all-zero line on a first-tool stop). A stop while a call is still streaming text (`Write a 2000-word essay on lighthouses. No tools.`, `--after 12`) gets no usage even then: the zero line is expected there.
- A managed `codex app-server --listen` daemon may already run. It is not ours; never kill it.
