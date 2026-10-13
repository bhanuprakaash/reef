# Agents

## Codex transport

The Codex adapter talks to `codex app-server` (JSON-RPC over stdio), not `codex exec --json`.
`exec` runs one process per turn, prints a message only once it is whole (no text deltas),
and can only be stopped by killing it. `app-server` keeps one thread alive across turns
(`turn/start` per `Send`), streams `item/agentMessage/delta`, and resumes by thread id
(`thread/resume`). MCP servers go in the `config` of `thread/start` as `mcp_servers`.
Its protocol is marked experimental; `codex app-server generate-json-schema` shows the shape.
