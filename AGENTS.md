# Reef: working agreement for every agent

Read this first. Then [`docs/product.md`](docs/product.md) (what we build and the 31
decisions), [`docs/server.md`](docs/server.md) (the data server) and
[`docs/memory.md`](docs/memory.md) (the CLI, MCP tools, `reef chat` and wrap-up).

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

## Slice verification

A slice is not done when its last sub-issue closes. It is done after a verification pass, run by an agent on Opus, never Sonnet:

1. Re-run every acceptance line of every sub-issue against `main`, with the real thing (binary, CLI, server). Record the command and what it showed.
2. `interrogate` the slice's full diff (first sub-issue branch point to `main`): standards, spec against `docs/product.md`, correctness, design.
3. `blast-radius` on what the slice changed that earlier slices depend on; prove the one fact it is safe because of by running code.
4. Every finding becomes a new sub-issue of the slice, or a written reason to skip it. The slice stays open until those close.
5. Close the slice issue with one comment: acceptance lines and their proof, interrogate and blast-radius summaries, skipped findings with reasons.

Each slice issue carries this checklist in a **Verification** section.

## Repo layout (target)

```
agent/        Agent interface, Event type, adapters (agent/claude, agent/codex)
channel/      Runner and context prompt for a channel
cmd/reef-agent  harness CLI
cmd/reef      main server + CLI (later)
store/        SQLite schema and queries
server/       HTTP, SSE, MCP server
web/          React UI; source modules in `web/src/modules`, conventions in [docs/react-style.md](docs/react-style.md)
desktop/      Electron shell
docs/         product.md, agents.md, server.md, decisions
```

Create a folder only when a ticket needs it.

## How we work

- One ticket at a time, on a branch named after the ticket (`agent-interface`, `claude-adapter`). PR to `main`. Never push `main` directly.
- Smallest diff that closes the ticket. Nothing speculative, no option or layer the ticket does not need.
- Prove it: every acceptance line in the ticket is checked by running the real thing. "It compiles" is not proof.
- Tick what you proved: after verifying a line, edit the issue body and check it (`gh issue edit`). A line you could not verify stays unchecked with a short note under it. A line only the user can verify (hand test, approval, merge) is left unchecked and the user is asked, in chat, with the exact steps.
- Decisions that change the product go to `docs/product.md` Decisions table, numbered, with a one-line why. Do not re-open a settled decision; propose a new one with a reason.
- Go: follow `docs/go-style.md` (condensed Uber guide). Standard library first. No frameworks until a ticket proves the need.
- React: follow `docs/react-style.md`.
- Commits: conventional prefix (`feat:`, `fix:`, `docs:`, `chore:`), one-line subject, no ticket ids in the subject. No AI attribution anywhere.
- Comments only for a constraint the code cannot show.
- Plain words in UI and code: feature, channel, team, role, brief, decision, task. "Reef" and "jelly" are brand only.
