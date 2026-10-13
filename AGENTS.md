# Reef: working agreement for every agent

Read this first. Then `docs/product.md` (what we build and the 19 decisions).

## What Reef is

A personal Mac app where every feature keeps its own memory, and a team of AI agents works on it in one window. Go server is the brain, React UI is the face, Electron is the Mac shell. Claude Code and Codex are the agent backends. We build it from scratch and, once there is a baseline, use Reef to build Reef.

## Plan

Eight slices, tracked as GitHub issues labeled `slice`. Each slice is split into small sub-issues before work starts. Order:

| Slice | Issue | Outcome |
| --- | --- | --- |
| 1 Agent adapters | #1 | One `Agent` interface, Claude Code + Codex adapters, `reef-agent` harness, `docs/agents.md` |
| 2 Core data + server | #2 | SQLite schema and REST for project, feature, channel, message, turn, decision, task, brief, role notes |
| 3 Memory loop | #3 | Reef MCP server, `reef` CLI, wrap-up with approval, context injection. Done = one real feature across 3 channels, no handoff file |
| 4 Solo channel | #4 | React UI, streaming channel, feature page, task list, promote quick session, caps |
| 5 Lead + architect | #5 | Teams, roles, routing, threads, @mention, hop cap |
| 6 Full team | #6 | Developer, tester, permissions, worktrees, gates, checkpoints, inbox |
| 7 Mac app | #7 | Electron shell, browser pane with agent control, terminal |
| 8 Polish | #8 | Board, GitHub, inspector, diff review, project memory, workshop |

Memory first: 1, 2, 3 before any UI. 1 and 2 have no dependency on each other.

## Repo layout (target)

```
agent/        Agent interface, Event type, adapters (agent/claude, agent/codex)
cmd/reef-agent  harness CLI
cmd/reef      main server + CLI (later)
store/        SQLite schema and queries
server/       HTTP, WebSocket, MCP server
web/          React UI
desktop/      Electron shell
docs/         product.md, agents.md, decisions
```

Create a folder only when a ticket needs it.

## How we work

- One ticket at a time, on a branch named after the ticket (`agent-interface`, `claude-adapter`). PR to `main`. Never push `main` directly.
- Smallest diff that closes the ticket. Nothing speculative, no option or layer the ticket does not need.
- Prove it: every acceptance line in the ticket is checked by running the real thing. "It compiles" is not proof.
- Decisions that change the product go to `docs/product.md` Decisions table, numbered, with a one-line why. Do not re-open a settled decision; propose a new one with a reason.
- Go: follow `docs/go-style.md` (condensed Uber guide). Standard library first. No frameworks until a ticket proves the need.
- Commits: conventional prefix (`feat:`, `fix:`, `docs:`, `chore:`), one-line subject, no ticket ids in the subject. No AI attribution anywhere.
- Comments only for a constraint the code cannot show.
- Plain words in UI and code: feature, channel, team, role, brief, decision, task. "Reef" and "jelly" are brand only.
