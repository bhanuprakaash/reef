# Memory and retrieval service

[Back to restructuring index](index.md) · settled points are R15–R30 in [decisions.md](decisions.md)

> Status: done. Built as package `memory/` with the `reef memory` CLI, and called by agents through the MCP tools `remember`, `recall` and `read_memory` and by the context assembler (decisions 90 to 95 and 110 to 125 in [docs/product.md](../product.md#decisions)). This is history; [docs/memory.md](../memory.md) says what the code does now.

## In one paragraph

Memory is many small items, each one statement with a kind, a scope (its shelf), an author and role, a status (candidate, accepted, retired, rejected) and links to what it is about, where it came from and what it replaces. A Teammate writes with `remember`, reads with `recall` and `read_memory`. Recall filters to the shelves the Teammate can see from where it stands, searches by words and by meaning, follows links one step, ranks and cuts at a budget. The wake prompt is built from the same tables by fixed queries with fixed counts, sent once per session, with a small delta on later wakes. The writer's role decides whether an item is accepted on write or waits for the user.

## Who sees what at wake

| Standing in | Shelves loaded |
| --- | --- |
| A Work item's Space, as a member of Team T | project(s) of the item, the Work item, team T, self |
| A free Space | project if the Space names one, the Space, team if invited as a team, self |
| A direct conversation | self |

Everything else is one `recall` away. Scopes are relevance filters, not access control (R1).

## Still open

- Space continuation: what triggers a new Space for the same Work item, and what carries over.
- Local embedder (Ollama) as another provider.
- Reranker, only if the eval set shows ranking is the weak spot.
- Migration from brief, role notes and project memory: none planned (R14, R27); revisit only if real data exists by then.

## Memory slice

What the memory-wiring slice must do, after the context assembler (R31–R36) lands:

- MCP tools `remember`, `recall` and `read_memory` (R23), replacing `record_decision` and `read_decision`; the "How to work" tool line names them.
- Swap the sources of "What we know": project memory becomes up to 10 project items, your notes up to 5 Teammate items, decisions become work-item decision items (all live); add findings (5) and team lessons (5) (R24). Each part ends `N of M; recall(..., scope=...)`.
- The delta (R34) runs over accepted memory items in scope since the role's previous turn, not the decision table.
- Which items fill a fixed count. Not settled. Options: the newest; the most relevant to the Work item (a recall with its title and brief); pinned first, then the newest (needs a pinned field).
- Compaction refreshes the snapshot (R24). Reef cannot see a compaction yet; find the signal in the claude and codex adapters first.
- The tasks and channels count line (R33) follows the Work item model: sub-items and Spaces (R5, R11, R13), when those exist.

