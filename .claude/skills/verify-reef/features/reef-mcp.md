# reef mcp

A teammate's agent reaches the feature's memory through `reef mcp`, a stdio MCP server.
`reef token` (or `reef chat`) makes a token for one role in one channel; `reef mcp` reads
it from `REEF_TOKEN` and serves seven tools on that channel's feature: `get_brief`,
`record_decision`, `add_task`, `search_sessions`, `read_decision`, `read_thread`,
`propose_wrap`. Contract: `docs/memory.md`.

## Sub-features

- `mcp-token` `reef token <feature> --role R` makes channel `<role> (claude code)` and a 64-hex token, and prints a `claude mcp add reef --env REEF_TOKEN=... -- <reef> mcp --data <dir>` line.
- `mcp-tools` `tools/list` names exactly the seven tools.
- `mcp-read` `get_brief`, `read_decision`, `read_thread`, `search_sessions` return the feature's rows as JSON text.
- `mcp-write` `record_decision` and `add_task` write rows with the token's role and feature; `propose_wrap` stores a pending proposal.
- `mcp-scope` a decision or channel of another feature is `not found`; every tool on a quick-session channel fails with `this channel has no feature`.
- `mcp-tool-errors` bad arguments come back as a result with `isError=true`, not a crash; an unknown tool is a JSON-RPC error.
- `mcp-no-token` no `REEF_TOKEN`, or an unknown one, exits 1 before serving.

## How to get to it (user POV)

- `reef token login --role developer`, then run the printed `claude mcp add` line; Claude Code then calls `mcp__reef__<tool>`.
- `reef chat` starts the agent with `reef mcp` already set up (see `chat.md`).

## Driving it with verify-reef

`$H mcp-call TOOL [JSON]` starts `reef mcp --data <run folder>` with the caller's
`REEF_TOKEN`, does the MCP handshake, lists the tools, calls one, closes stdin and waits
for the exit. It prints and logs to `reef-mcp.log`: `> TOOL JSON`, `tools: ...`,
`< isError=false|true <text>`, `reef mcp exit: N`, and any stderr. It exits 0 when a tool
result came back, 1 otherwise.

Preconditions:

- Baseline from the README. No server needed.
- Setup: `$H reef --data-name mcp -- project add`, `$H reef -- feature add login "Email login"`, `$H reef -- feature add billing "Billing"`, `$H reef -- decide login "Use sessions, not JWT" --why "One server"` (decision 1), `$H reef -- decide billing "Use Stripe"` (decision 2).
- `$H reef -- token login --role developer` prints `claude mcp add reef --env REEF_TOKEN=<64 hex> -- /private/tmp/reef-verify/<id>/bin/reef mcp --data /private/tmp/reef-verify/<id>/mcp`. Pass that hex as `REEF_TOKEN=<token>` on every `mcp-call` below. `$H sql "select id, feature_id, name from channel"` shows `1|1|developer (claude code)`; `$H sql "select channel_id, role, length(token) from token"` shows `1|developer|64`.

- **Tools and brief.** `REEF_TOKEN=<token> $H mcp-call get_brief` prints `tools: add_task get_brief propose_wrap read_decision read_thread record_decision search_sessions`, `< isError=false {"brief":"","notes":""}`, `reef mcp exit: 0`.
- **Record a decision.** `REEF_TOKEN=<token> $H mcp-call record_decision '{"body":"Rate limit by IP","why":"No accounts yet"}'` gives `isError=false` with `"author":"developer"`, `"featureId":1`, `"id":3`. `$H sql "select id, feature_id, author, body from decision where id = 3"` shows `3|1|developer|Rate limit by IP`.
- **Add a task.** `REEF_TOKEN=<token> $H mcp-call add_task '{"title":"Add rate limit"}'` gives `"featureId":1`, `"projectId":1`, `"status":"todo"`.
- **Read.** `REEF_TOKEN=<token> $H mcp-call read_decision '{"id":1}'` gives `Use sessions, not JWT`. `REEF_TOKEN=<token> $H mcp-call read_decision '{"id":2}'` gives `isError=true decision 2: not found` (billing's). `REEF_TOKEN=<token> $H mcp-call read_thread '{"channelId":1}'` gives `{"messages":null}`; `'{"channelId":9}'` gives `isError=true channel 9: not found`. `REEF_TOKEN=<token> $H mcp-call search_sessions '{"query":"x"}'` gives `{"hits":null}`.
- **Propose.** `REEF_TOKEN=<token> $H mcp-call propose_wrap '{"brief":"Login by email","notes":"Form first","decisions":[{"body":"Lock after 5 tries","why":""}]}'` gives `{"id":1,"message":"proposal saved; the user will approve it"}`. `$H sql "select id, channel_id, role, status, decisions from proposal"` shows `1|1|developer|pending|[{"body":"Lock after 5 tries","why":""}]`. `$H sql "select count(*) from brief"` is 0: nothing applied.
- **Tool errors.** `REEF_TOKEN=<token> $H mcp-call search_sessions` gives `isError=true validating "arguments": ... missing properties: ["query"]`. `REEF_TOKEN=<token> $H mcp-call propose_wrap '{"brief":"","notes":"","decisions":[]}'` gives `isError=true invalid: proposal brief is required`. `REEF_TOKEN=<token> $H mcp-call nope` gives `< rpc error {"code": -32602, "message": "unknown tool \"nope\""}`. Each still shows `reef mcp exit: 0`.
- **Quick session.** No command makes a token on a channel without a feature, so set it up: `$H sql --write "insert into channel (project_id, name, created_at) values (1, 'quick', '2026-10-13T00:00:00Z'); insert into token (token, channel_id, role, created_at) values ('quick-token', 2, 'developer', '2026-10-13T00:00:00Z')"`. `REEF_TOKEN=quick-token $H mcp-call get_brief` gives `isError=true this channel has no feature`.
- **No token.** `REEF_TOKEN= $H mcp-call get_brief` prints `reef mcp exit: 1`, `stderr: reef: REEF_TOKEN is not set`, and exits 1. `REEF_TOKEN=abc $H mcp-call get_brief` prints `stderr: reef: unknown REEF_TOKEN`.

## Gotchas

- `mcp-call` is a scripted MCP client, not a model. Proof that a real agent calls the tools through its own MCP client is the `propose_wrap` call in `chat.md`.
- Every Bash call is a new shell. Put `REEF_TOKEN=<token>` on each `mcp-call` line.
- Empty lists come back as `null` (`{"hits":null}`, `{"messages":null}`), not `[]`.
- Never run the printed `claude mcp add` line during a verify run: it writes the user's Claude Code config.
