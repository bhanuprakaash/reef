# MCP servers

A user gives the session extra MCP servers with `reef-agent --mcp FILE`. FILE is a JSON
object of server name to `{command, args, env}` (stdio) or `{url}` (HTTP). The session can
call their tools with no approval prompt, in any permission mode.

## Sub-features

- `mcp-call` the model calls a tool of a `--mcp` server; the result comes back in a `tool_result`.
- `mcp-preapproved` the call succeeds in the default permission mode (no `--permission-mode`).
- `mcp-bad-name` (claude) a server name with characters outside `A-Z a-z 0-9 _ -` is refused at start.
- `mcp-bad-file` a missing or non-JSON FILE is refused at start.

## How to get to it (user POV)

- `reef-agent --backend claude|codex --mcp servers.json "prompt"`.

## Driving it with verify-reef

Preconditions:

- Baseline from the README.
- `REEF_VERIFY_RUN=<id> $H mcp-server` printed `/private/tmp/reef-verify/<id>/mcp/mcp.json`. It defines server `vping` with tool `secret_word`, which returns `kumquat-4417`.

- **Call, claude.** Run `ANTHROPIC_MODEL=sonnet REEF_VERIFY_RUN=<id> $H run --name mcp-claude -- --backend claude --mcp /private/tmp/reef-verify/<id>/mcp/mcp.json "Call the secret_word tool of the vping MCP server and tell me the word."`. `exit: 0`. A `tool_call` with `"toolName":"mcp__vping__secret_word"` and a `tool_result` with the same `toolId`, `"output":"kumquat-4417"` and no `isError`. `~/.reef-verify/<id>/mcp-calls.log` has `{"call": "secret_word"}`.
- **Call, codex.** Run `REEF_VERIFY_RUN=<id> $H run --name mcp-codex -- --backend codex --mcp /private/tmp/reef-verify/<id>/mcp/mcp.json "Call the secret_word tool of the vping MCP server and tell me the word."`. Same proof: `toolName` `mcp__vping__secret_word`, output `kumquat-4417`, a new line in `mcp-calls.log`.
- **Bad name, claude.** Write `{"v.ping": {"command": "true"}}` to `~/.reef-verify/<id>/badname.json`. Run `REEF_VERIFY_RUN=<id> $H run --name mcp-badname-claude -- --backend claude --mcp ~/.reef-verify/<id>/badname.json "hi"`. `exit: 1`, empty jsonl, stderr `reef-agent: mcp server name "v.ping": only letters, digits, _ and - allowed`.
- **Bad file.** Run `REEF_VERIFY_RUN=<id> $H run --name mcp-nofile -- --backend codex --mcp /nonexistent.json "hi"`. `exit: 1`, stderr `reef-agent: open /nonexistent.json: no such file or directory`.

## Gotchas

- Claude defers MCP tools, so it may call `ToolSearch` first. Look for the `mcp__vping__secret_word` call, not the first `tool_call`.
- `kumquat-4417` cannot be guessed. A reply with the word and no matching `tool_result` means the proof is wrong, not that the tool ran.
- The user's own MCP config still loads on both backends. Other `mcp__*` tools in the stream are theirs.
- Codex does not check server names; only claude refuses `v.ping`.
- The helper has no HTTP MCP server. An HTTP (`url`) entry needs its own server; report it as not run if none is at hand.
- Claude's temp MCP config (`$TMPDIR/reef-mcp-*.json`) is removed when claude exits; a leftover one after the drive is a finding.
