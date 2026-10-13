# Agents

Reef drives coding agents through one Go interface, [`agent/agent.go`](../agent/agent.go).
Two backends implement it: Claude Code ([`agent/claude`](../agent/claude/claude.go)) and
Codex ([`agent/codex`](../agent/codex/codex.go)). The code is the source of truth. This doc
explains what the code does and why.

## The contract

```go
Start(ctx, cfg) error                 // new session in cfg.Cwd
Resume(ctx, cfg, sessionID) error     // continue an earlier session
Send(ctx, prompt) error               // start one turn
Events() <-chan Event                 // valid right after New; closed when the process exits
Interrupt() error                     // stop the turn and the process
```

- One `Start` or `Resume` per `Agent`; a second call returns an error. It spawns the
  process and sends no prompt. `Send` is not safe for concurrent use.
- A turn ends with a `turn_end` or `error` event. Call `Send` again only after that. A
  failure gives one `error`.
- `Events` is unbuffered. The adapter blocks until you read.
- `ctx` bounds only the call it is passed to. Claude ignores it. Codex uses it to abort the
  handshake and returns an error wrapping `ctx.Err()`. `Interrupt` is the only way to stop a
  session, and is safe to call while `Start` runs.
- `Interrupt` kills the process. When it returns, the process is gone and `Events` is
  closed, also if `Start` failed or never ran. You need not drain `Events` first. A second
  call is a no-op. Tool processes in their own process group may outlive a forced kill. To
  go on, `Resume` on a new `Agent` with the id from the last `session` event.
- `Config`: `Cwd`, `PermissionMode` (backend-specific, passed through), `MCPServers`
  (stdio `Command`/`Args`/`Env` or HTTP `URL`), `Env` (`KEY=value`, added to `os.Environ`).

An `Event` is one JSON line (the `reef-agent` wire format). Empty fields are left out.

```json
{"kind":"session","sessionId":"3c1296cc-..."}
{"kind":"text","text":"Output: "}
{"kind":"tool_call","toolId":"toolu_01Q...","toolName":"Bash","input":{"command":"echo hi"}}
{"kind":"tool_result","toolId":"toolu_01Q...","output":"hi"}
{"kind":"tool_result","toolId":"toolu_01Q...","output":"boom","isError":true}
{"kind":"usage","usage":{"inputTokens":4,"outputTokens":123,"cacheReadTokens":22898,"cacheWriteTokens":0}}
{"kind":"turn_end"}
{"kind":"error","text":"No conversation found with session ID: ..."}
```

`text` is a delta, not a whole message. A `tool_result` carries the `toolId` of its
`tool_call`. Inside `usage`, no field is left out.

`reef-agent --backend claude|codex [--cwd DIR] [--resume ID] [--permission-mode MODE]
[--mcp FILE] "prompt"` runs one backend and prints these lines. `FILE` is a JSON object of
server name to `{command, args, env, url}`. Each stdin line is a
follow-up prompt, sent after a `turn_end`. It exits 1 on an `error` event. On Ctrl-C it
prints one `usage` line with the session total and exits 130.

## Claude Code

Spawn, one long-lived process per session (in `cfg.Cwd`):

```
claude -p --input-format stream-json --output-format stream-json --verbose \
  --include-partial-messages [--permission-mode MODE] [--mcp-config PATH] [--resume ID]
```

Input: one JSON line on stdin per `Send`. Keep stdin open for the whole session. Output is
read with `bufio.Reader`, so a line has no size cap.

```json
{"type":"user","message":{"role":"user","content":"<prompt>"}}
```

Output: one JSON line per message, every line has `session_id`. Samples, trimmed:

```jsonc
// system/init: comes after the first user message, not at spawn. Repeats every turn.
{"type":"system","subtype":"init","cwd":"/private/tmp/scratch","session_id":"3c1296cc-...","permissionMode":"bypassPermissions"}
// text delta (needs --include-partial-messages)
{"type":"stream_event","event":{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"`ls"}},"session_id":"..."}
// tool call: complete block, input is whole
{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_01Q...","name":"Bash","input":{"command":"echo hello-from-tool"}}]},"session_id":"..."}
// complete text block: skipped, the deltas already gave the text
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Output: `hello-from-tool`"}]},"session_id":"..."}
// tool result: content is a string or a list of {"text":...} blocks
{"type":"user","message":{"role":"user","content":[{"tool_use_id":"toolu_01Q...","type":"tool_result","content":"hello-from-tool","is_error":false}]},"session_id":"..."}
// turn end, usage is per turn
{"type":"result","subtype":"success","is_error":false,"result":"Output: ...","session_id":"...","usage":{"input_tokens":4,"cache_creation_input_tokens":10112,"cache_read_input_tokens":22898,"output_tokens":123}}
// failed resume: process exits 1, no "result" text, message in errors[]
{"type":"result","subtype":"error_during_execution","is_error":true,"session_id":"0000...","usage":{...},"errors":["No conversation found with session ID: 0000..."]}
```

Noise, ignored: `system` with subtype `hook_started`, `hook_response`, `commands_changed`,
`status`; `rate_limit_event`.

```json
{"type":"rate_limit_event","rate_limit_info":{"status":"allowed"},"session_id":"..."}
```

## Codex

Transport is `codex app-server` (JSON-RPC over stdio), not `codex exec --json`.
`exec` runs one process per turn, prints a message only once it is whole (no text deltas),
can only be stopped by killing it, and `exec resume` lacks `-s`. `app-server` keeps one
thread alive across turns, streams text deltas, and resumes by thread id. Its protocol is
experimental; `codex app-server generate-json-schema --out DIR` shows the shape.

Spawn: `codex app-server` (stdio is the default transport), in `cfg.Cwd`. Then, one JSON
object per line:

```
-> {"id":1,"method":"initialize","params":{"clientInfo":{"name":"reef","title":null,"version":"0"}}}
<- {"id":1,"result":{"userAgent":"reef/0.162.1 ...","codexHome":"..."}}
-> {"method":"initialized"}
-> {"id":2,"method":"thread/start","params":{"cwd":"/tmp/cx","approvalPolicy":"never",
     "sandbox":"workspace-write","config":{"mcp_servers":{...}}}}      // or thread/resume + threadId
<- {"id":2,"result":{"thread":{"id":"01a133d7-...","cwd":"/tmp/cx"},...}}
```

`sandbox` and `config` are set only when `PermissionMode` / `MCPServers` are. One goroutine
owns stdout: it waits for each response, skipping other lines, then emits `session` once and
the events, then calls `Wait`, so a process that exits on its own is reaped. Then each `Send`:

```
-> {"id":3,"method":"turn/start","params":{"threadId":"01a133d7-...","input":[{"type":"text","text":"<prompt>"}]}}
```

The response to `turn/start` carries no events. The events arrive as notifications.
A JSON-RPC `error` response after the handshake (for example to `turn/start`) becomes an
`error` event. During the handshake it is the error `Start` or `Resume` returns. Samples, trimmed:

```jsonc
{"method":"turn/started","params":{"threadId":"...","turn":{"id":"...","status":"inProgress"}}}
// text delta
{"method":"item/agentMessage/delta","params":{"threadId":"...","turnId":"...","itemId":"msg_...","delta":"I"}}
// complete message: skipped
{"method":"item/completed","params":{"item":{"type":"agentMessage","id":"msg_...","text":"I'll list the files...","phase":"commentary"}}}
// shell command
{"method":"item/started","params":{"item":{"type":"commandExecution","id":"exec-0ff8...","command":"/bin/zsh -lc 'ls -la'","cwd":"/tmp/cx","status":"inProgress","aggregatedOutput":null,"exitCode":null}}}
{"method":"item/completed","params":{"item":{"type":"commandExecution","id":"exec-0ff8...","status":"completed","aggregatedOutput":"total 0\n...","exitCode":0}}}
// file edit
{"method":"item/started","params":{"item":{"type":"fileChange","id":"exec-69db...","changes":[{"path":"/tmp/cx/notes2.md","kind":{"type":"add"},"diff":"x\n"}],"status":"inProgress"}}}
{"method":"item/completed","params":{"item":{"type":"fileChange","id":"exec-69db...","changes":[...],"status":"completed"}}}
// MCP tool
{"method":"item/started","params":{"item":{"type":"mcpToolCall","id":"exec-fb96...","server":"reef-ping","tool":"reef_ping","status":"inProgress","arguments":{}}}}
{"method":"item/completed","params":{"item":{"type":"mcpToolCall","id":"exec-fb96...","status":"completed","result":{"content":[{"type":"text","text":"pong"}]},"error":null}}}
// usage: one per model call, several per turn. "last" is that call.
{"method":"thread/tokenUsage/updated","params":{"tokenUsage":{"total":{...},"last":{"inputTokens":15026,"cachedInputTokens":14080,"outputTokens":118,"reasoningOutputTokens":45}}}}
// turn end
{"method":"turn/completed","params":{"threadId":"...","turn":{"id":"...","status":"completed","error":null}}}
// failed turn: an "error" notification comes first (ignored), then this
{"method":"turn/completed","params":{"turn":{"status":"failed","error":{"message":"{\"type\":\"error\",\"status\":400,...}"}}}}
```

Noise, ignored: `thread/started`, `thread/status/changed`, `mcpServer/startupStatus/updated`,
`turn/diff/updated`, the `error` notification, and `item/*` for `userMessage`, `reasoning`
and `agentMessage`.

## Mapping

| Event kind | Claude Code source | Codex source |
| --- | --- | --- |
| `session` | `system`/`init` line, `session_id`. Repeats every turn, same id. First one comes after the first `Send`. | Response to `thread/start` / `thread/resume`, `thread.id`. Once, as the first event, before any `Send`. |
| `text` | `stream_event` with `content_block_delta` / `text_delta`, `delta.text` | `item/agentMessage/delta`, `params.delta` |
| `tool_call` | `assistant` line, each `tool_use` block: `id`, `name`, `input` | `item/started` for `commandExecution` (name `commandExecution`, input `{"command":...}`), `fileChange` (name `fileChange`, input `changes`), `mcpToolCall` (name `mcp__<server>__<tool>`, input `arguments`) |
| `tool_result` | `user` line, each `tool_result` block: `tool_use_id`, text of `content`, `is_error` | `item/completed` for the same three item types. Output: `aggregatedOutput` / status (`fileChange`) / MCP error message or joined `result.content` text. `isError` if status is not `completed` or `exitCode` is not 0. |
| `usage` | `result` line `usage`: `input_tokens`, `output_tokens`, `cache_read_input_tokens`, `cache_creation_input_tokens` (`cacheWriteTokens`). One per turn, just before `turn_end` or `error`. | `thread/tokenUsage/updated` `last`. `inputTokens` minus `cachedInputTokens` minus `cacheWriteInputTokens`, `outputTokens`, `cachedInputTokens`, `cacheWriteInputTokens`. This assumes `inputTokens` includes both cache fields; `cacheWriteInputTokens` was 0 in every live run, so unconfirmed. One per model call, several per turn. |
| `turn_end` | `result` line with `is_error` false | `turn/completed` with any status but `failed` |
| `error` | `result` with `is_error` true (`result` and `errors[]` joined). Also a non-zero exit when the last event was not already an `error`: `claude exited: ...: <stderr>`. | `turn/completed` with status `failed` (`turn.error.message`), or a JSON-RPC error response. Also a non-zero exit on its own: `codex exited: <err>: <stderr>`. |

Both: a tool id is the same in its call and result. Claude's tool input also streams as
`input_json_delta` fragments; the adapter ignores them and uses the whole `tool_use` block.
Codex item types not listed (`webSearch`, `reasoning` ...) give no event.

## Gotchas

Permissions
- Claude: with no `--permission-mode`, `-p` cannot ask, so tools that need permission are
  denied. Values: `acceptEdits`, `auto`, `bypassPermissions`, `manual`, `dontAsk`, `plan`.
  A bad value exits 1 with the list on stderr.
- Codex: `PermissionMode` is the thread `sandbox`: `read-only`, `workspace-write`,
  `danger-full-access`. `approvalPolicy` is always `never`. A bad value fails `thread/start`.
  Unset means the user's `config.toml` decides.
- Codex server requests (approval, elicitation) get a `-32601` reply. That path is untested.

cwd
- The process and, for Codex, `thread/start` get `cfg.Cwd`. Codex needs no git repo.
- Claude reports the real path (`/private/tmp` for `/tmp`). It stores sessions per cwd, so
  `Resume` needs the same `Cwd`.

Env
- Both: `os.Environ()` plus `cfg.Env`. Claude also loads the user's hooks, plugins and
  settings in `-p`, which adds start-up time. `ANTHROPIC_MODEL` picks the Claude model
  (tests use `sonnet`).

MCP config injection
- Claude: `--mcp-config PATH` to a 0600 temp file, removed after exit. This keeps MCP `env`
  secrets off argv. The file is `{"mcpServers":{"name":{"command":..,"args":..,"env":..}}}`. HTTP
  servers need `"type":"http"`; a bare `{"url":...}` is dropped silently. The adapter adds
  it when `URL` is set. Tool names are `mcp__<server>__<tool>`. MCP tools are deferred, so
  the model may call `ToolSearch` first. The user's own MCP config still loads.
- Codex: `thread/start` `config.mcp_servers`, same fields. With approval `never`, MCP calls
  fail with "requires approval" unless the server has
  `default_tools_approval_mode: "approve"`. The adapter sets it.

Session and thread ids
- Claude `session_id` is on every line. `--resume ID` keeps the same id (no new id without
  `--fork-session`; the adapter does not fork).
- Codex thread id is the session id. `thread/resume` takes it as `threadId`.
- Codex `thread/resume` replays the last turn's usage before any turn starts. The adapter
  drops `usage` outside `turn/started` .. `turn/completed`.

Turns
- Claude: two prompts written back to back merge into one turn with one `result`. Wait for
  `turn_end` before the next `Send`.

Interrupt and orphans
- Both start the process in its own group and set `cmd.WaitDelay` to 2 s. `Interrupt` sends
  SIGTERM to the group, then SIGKILL after 1 s. SIGTERM first lets the backend stop the tools it started: those run in
  their own process groups, so SIGKILL alone would orphan them.
- Claude tools started with `run_in_background` survive. Known leak.
- Codex has `turn/interrupt`. It is unused: the contract is "process gone".
- A `codex app-server --listen unix://` daemon may already run. The adapter spawns its own
  stdio server and ignores it.

Noise and errors
- Unknown or unparsable lines give no event, on both backends.
- Claude: a failed resume is exit 1 plus a `result` with `is_error`. The adapter emits
  `usage` then exactly one `error` (from the result). The exit error
  (`claude exited: ... <stderr>`) is added only when the last event was not already an
  `error`, so a crash between turns still reports.
- Codex: a non-zero exit on its own gives one `codex exited: <err>: <stderr>` error.
- Both keep the last 4 KiB of stderr, and emit no error after `Interrupt`.

## Writing a third adapter

For a new backend (Jellyfish next):

1. Create `agent/<name>/`. Add `var _ agent.Agent = (*Agent)(nil)` and `New()`. Make
   `Events` in `New`, so it is valid before `Start`.
2. Find how the CLI streams JSON and takes follow-up prompts on one process. Prefer one
   long-lived process per session, with text deltas. Save real output lines to a file first
   and write the parser against them.
3. `Start` / `Resume`: build the command. Set `Dir` to `cfg.Cwd`, `Env` to
   `os.Environ()` plus `cfg.Env`, and `Setpgid: true`. Pass `PermissionMode` through as
   the backend's own value. Inject `MCPServers` in the backend's format, keeping secrets off
   argv. Check HTTP servers work. Set `cmd.WaitDelay` and keep the last 4 KiB of stderr.
   Allow one start per `Agent`; return an error on a second. Start the process; send no
   prompt.
4. `Send`: write one prompt to the process. Return at once; the turn runs on `Events`.
5. Reader goroutine: parse each line into zero or more `Event`s, send them on `Events`,
   close `Events` when the process exits. Emit `session` with the id `Resume` needs. Map
   the backend's lines with the table above: a delta to `text`, whole-input tool starts to
   `tool_call`, finishes to `tool_result` (same `toolId`), token counts to `usage` (input
   without cache, cache read separate), end of turn to `turn_end`. Failures end the turn
   with `error`. Skip complete messages whose text you already sent as deltas. Skip noise.
6. Make every turn end in a `turn_end` or `error`. Add an `error` with the stderr tail for a
   non-zero exit, unless the last event was already an `error`. One `error` per failure.
   Emit nothing after `Interrupt`.
7. `Interrupt`: SIGTERM the process group, SIGKILL after 1 s, wait for exit and for the
   reader to finish. Use `sync.Once`. Make the reader stop when interrupted, so a blocked
   send to `Events` does not hang. Return nil if never started.
8. Test with the real CLI: new session, tool call (shell, file, MCP), resume with the id,
   a failed resume, `Interrupt` mid-tool and check no process is left (`pgrep`), bad
   permission value. Add contract tests against a fake binary in `testdata/` put first on
   `PATH`, as `agent/claude` and `agent/codex` do.
9. Add the backend to `newAgent` in `cmd/reef-agent/main.go`. Run it with
   `reef-agent --backend <name> "prompt"`.
10. Add a column to the mapping table and any new gotchas here. Record the CLI flags you
    use and check them against `<cli> --help`.
