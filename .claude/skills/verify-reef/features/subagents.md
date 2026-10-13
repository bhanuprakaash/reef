# Subagent tool calls

When a claude session starts a subagent (the `Agent` tool), the subagent's own tool calls and
results are in the stream too. Each carries `parentToolId`, the `toolId` of the `Agent` call
that started it, so a reader can nest them under that call. Top-level events have none.

## Sub-features

- `subagent-parent` a subagent's `tool_call` and `tool_result` carry `parentToolId` equal to the `Agent` call's `toolId`.
- `subagent-top-level` the `Agent` call itself and other top-level tool events have no `parentToolId`.
- `subagent-codex` codex sets no `parentToolId`; codex subagents give no events.

## How to get to it (user POV)

- Ask a claude session to delegate work to a subagent: `reef-agent --backend claude "Use a subagent to ..."`.

## Driving it with verify-reef

Preconditions:

- Baseline from the README.

- **Subagent, claude.** Write `sub-hello` to `/private/tmp/reef-verify/<id>/cwd/sub.txt`. Run `ANTHROPIC_MODEL=sonnet REEF_VERIFY_RUN=<id> $H run --name subagent-claude -- --backend claude "Use the Agent tool to start a subagent that reads the file sub.txt in the current directory with the Read tool. Then tell me its contents."`. `exit: 0`.
- **Parent link.** `grep '"toolName":"Agent"' ~/.reef-verify/<id>/subagent-claude.jsonl` gives one `tool_call`, toolId `A`, with no `parentToolId`. `grep '"parentToolId"' ...` gives a `tool_call` with `"toolName":"Read"`, a `file_path` ending in `/cwd/sub.txt`, `"parentToolId":"A"`, and its `tool_result` with output containing `sub-hello` and the same `parentToolId`.
- **Agent result.** A `tool_result` with `toolId` `A` and no `parentToolId` comes after the subagent's events.
- **Codex.** Not applicable: `grep -c parentToolId` on any codex jsonl is `0`. Report `subagent-codex` as a known gap, not a failure.

## Gotchas

- Subagent text is not streamed, so `text` lines never carry `parentToolId`.
- The model may answer without a subagent. If there is no `Agent` call, re-run once with the same prompt; do not report the link as proven from a run without one.
- The subagent has the same permissions as the session: Read runs, Bash or Write is denied with an `isError` `tool_result` that still carries `parentToolId`.
