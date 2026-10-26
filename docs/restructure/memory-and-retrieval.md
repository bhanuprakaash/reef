# Memory and retrieval service

[Back to restructuring index](index.md) · settled points are R15–R30 in [decisions.md](decisions.md)

> Status: built as package `memory/` with the `reef memory` CLI; not yet called by agents (the MCP tools and context assembler wait, R27).

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
- Context assembler: exact section order, how the delta is rendered, and the measured prompt-size tests. Comes after the prompt audit (R2).
- Local embedder (Ollama) as another provider.
- Reranker, only if the eval set shows ranking is the weak spot.
- Migration from brief, role notes and project memory: none planned (R14, R27); revisit only if real data exists by then.
