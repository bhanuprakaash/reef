# Memory and retrieval service — exploratory proposal

[Back to restructuring index](index.md)

> Status: design direction only. We will drill into implementation choices and tests separately before integrating anything with Reef.

## Intent

Build a **standalone, graph-first, role-governed, evidence-backed memory and retrieval service** in an isolated folder/module. Simulate surrounding Reef services and test it rigorously before plugging it into the product. This is not a proposal to add a temporary SQLite memory layer to the existing flow. Graph storage technology and schema are deliberately undecided pending workload and evaluation criteria.

## Core principle: role-aware memory formation

The writer's role governs the *perspective and authority* of the memory. Its current assignment, team, project, and space govern scope and access. An investigation agent records observed causes, conditions, leads, evidence and findings; it must not silently establish a project-wide coding convention. It may propose a convention for appropriate review. A developer can record implementation findings, QA can record coverage gaps, and a team can record collective coordination and outcomes.

Example: an auth guard failed on a particular date when tenant configuration was missing.
- Investigator: observed failure mode, conditions, evidence and a lead for future investigations.
- Developer: implementation defect and fix details.
- QA: missing regression case and verification.
- Team: collective resolution and handoff lesson.
- Project: validated technical finding or approved decision linked to incident and code.

Do not produce five duplicate copies of the same underlying event.

## Direct conversations and on-demand retrieval

A **Teammate** (the user-facing name for an individual agent) can have multiple independent direct conversations. Each conversation is ordinary session history, without required project IDs, a project selector, or a predefined project association. The user may discuss one project, multiple projects, or unrelated topics in the same conversation. Based on the current question, the Teammate can retrieve relevant memories and traces on demand. The memory service resolves references, checks permissions, preserves provenance and returns bounded context; mentioning a project does not grant access. A transcript does not automatically become the Teammate's personal memory. Knowledge extracted from it receives its own role-aware ownership and scope.

## Logical scopes

- **Teammate (internal: agent):** persistent identity, general expertise, appropriately promoted reusable lessons and history.
- **Direct conversation:** independent session history without mandatory project linkage; relevant authorised knowledge is retrieved on demand and is not automatically permanent personal knowledge.
- **Team:** collective experience, responsibilities, handoffs and operating lessons, not copies of all member/project data.
- **Project:** technical facts, architecture, conventions and accepted decisions.
- **Space:** discussion context, conclusions and open questions.
- **Work item:** investigation/incident/task evidence, findings, plan, execution and outcome.
- **Workspace/organisation:** potential future shared policy scope; not yet settled.

These are ownership and retrieval boundaries within one service, not separate memory databases. A shared agent does not automatically carry Team A's confidential context into Team B.

## Distinguish information classes

- **History:** durable record of what happened, with source and time.
- **Candidate knowledge:** extracted observations, findings, hypotheses, recommendations and lessons.
- **Accepted knowledge:** evidence-supported or appropriately approved; can be revised, superseded, or retired.
- **Working context:** temporary, budgeted retrieval for a particular agent execution.

A claim made during a conversation is not automatically an established fact or authoritative instruction.

## Proposed memory lifecycle

1. Capture source events, messages, tool results, work outcomes and provenance.
2. Extract candidate memories from the actor's role and assignment perspective.
3. Validate scope, authority, permissions, evidence and required approvals in service-side policy (not prompting alone).
4. Resolve entities and relationships; detect duplicates, conflicts and supersession.
5. Store versioned graph knowledge linked to its sources and relevant actors/projects/work.
6. Retrieve only relevant, permitted, current knowledge for the next execution.

Graph relationships describe connections, **not authorization**. Enforce access before content or summaries reach a model. A team or agent's memory should not be a dumping ground for entire transcripts.

## Retrieval direction

Given the actor, current request, conversational context, applicable permissions and context budget (resolving relevant entities such as projects or work items when needed): filter authorized knowledge, search relevant records, expand useful graph relationships, rank by relevance/evidence/recency, and return compact sourced context. Deeper detail should remain available on demand through a narrow memory API/MCP interface. Do not inject the full graph or all prior history into a system prompt.

## Independent evaluation plan

Mock agents, teams, projects, spaces, work items, events and execution outputs. Include:
- Multiple roles deriving different legitimate memories from one incident.
- An investigator attempting to write an unauthorized project convention.
- One agent in two teams/projects, including cross-scope leakage attempts.
- Repeated incidents, duplicate findings, conflicting evidence, corrections and superseded decisions.
- A later investigation that should recover an older relevant failure mode.
- Large history with bounded retrieval and source attribution.

Measure role/scope correctness, access isolation, evidence traceability, retrieval precision and recall, duplication, contradictions, latency, token cost and recovery behavior. Combine deterministic policy/integrity assertions with semantic evaluations; an LLM judge alone is insufficient.

## Separate Reef integration track

Independently investigate oversized/repeated channel system prompts, then define one inspectable context assembler with explicit section provenance, instruction/knowledge separation, token budgets, and fresh/resumed/handoff tests. Only integrate the memory service after both standalone memory and Reef context tests meet agreed criteria.

## Open design questions

Graph engine and deployment model; entity/edge schema; write-policy representation; evidence thresholds; automatic vs approved promotion into personal memory; temporal validity and contradiction resolution; embeddings/search; retention/deletion; API contracts; migration from feature briefs and role notes. None is decided here.
