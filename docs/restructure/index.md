# Reef restructuring proposal

> Status: exploratory design discussion. This is a direction of travel, not an approved implementation specification. No existing Reef behavior is changed by these documents.

## Contents

1. [Why restructure?](#why-restructure)
2. [Proposed domain concepts](#proposed-domain-concepts)
3. [Teammates and teams](#teammates-and-teams)
4. [Direct conversations without mandatory context association](#direct-conversations-without-mandatory-context-association)
5. [Spaces, channels, and work](#spaces-channels-and-work)
6. [Memory and retrieval service](memory-and-retrieval.md)
7. [Reef context and prompt cleanup](#reef-context-and-prompt-cleanup)
8. [Boundaries and unresolved questions](#boundaries-and-unresolved-questions)
9. [Proposed exploration sequence](#proposed-exploration-sequence)

## Why restructure?

Reef currently centers much of its workflow and memory on a Feature and its channels, roles, notes, and stages. Real company work is broader: investigations, hotfixes, incidents, bugs, maintenance, ideas, and features. A Feature should not be a prerequisite for collaboration or execution. A feature can remain an optional kind of work, rather than the universal container.

We want persistent, individually recognizable Teammates that can collaborate across teams and projects without leaking unrelated context.

## Proposed domain concepts

- **Teammate:** The chosen user-facing term for one persistent individual AI collaborator, with expertise, instructions, tools, scripts, capabilities, and history. **Agent** may remain an internal technical term. Do not use **employee** in the product model; users should not need to understand templates or runtime instances.
- **Direct conversation:** One independent, persistent chat session with one Teammate. A Teammate can have many conversations, each with its own history. It has no mandatory project, team, feature, work-item, or project-ID association.
- **Team:** A composition of existing agents, with responsibilities, coordination rules, and collective experience. An agent may join multiple teams without being cloned.
- **Project:** A repository or project boundary with its own durable technical knowledge, decisions, and conventions.
- **Space:** A persistent place to invite agents or teams and discuss, investigate, brainstorm, and plan. A discussion need not create work or follow prescribed stages.
- **Work item:** Project-level unit of work, optionally typed as investigation, incident, bug, task, improvement, feature, etc. Supports parent/child items, ownership by team or agent, and links to source discussions and outcomes.
- **Channel:** Event/integration stream, distinct from conversational Spaces; e.g. Slack messages, infra alerts, webhooks.
- **Automation:** Polls or receives events, deduplicates and routes them under explicit permissions; an incoming message is data, not automatically an instruction to execute.

Names and precise boundaries remain open to discussion.

## Teammates and teams

Create Teammates independently, then add them to teams. A single Teammate may participate in several teams with different responsibilities. The visible model is one identity, many memberships; execution sessions and working contexts are isolated internally. A Teammate's general knowledge does not imply automatic access to all prior teams' private context. Teams may remember collective handoffs and operating lessons without copying entire project histories.

Team templates are not a required initial user-facing concept. Direct conversations do not require a team or Space.

## Direct conversations without mandatory context association

- Open a Teammate directly and create any number of separate, resumable ordinary chat sessions. A new conversation does not clone the Teammate.
- A conversation is just a conversation: do not require or persist a project selector, project-ID list, team, feature, work-item link, or predefined context scope as part of the direct-conversation model.
- A user may discuss any topic, including one project, multiple projects, or none, and change topics naturally in the same conversation.
- The Teammate can request relevant memory and traces on demand based on the user's question and the ongoing conversation. Retrieval resolves entities, relevance, provenance and permissions; it must not assume that every project mentioned is authorised or inject whole project histories.
- The conversation transcript is history, not automatically permanent personal memory. Extracted findings retain their own proper source, role, scope and access policy in the memory graph.
- Conversations can yield work or lead to collaborative Spaces, but neither is mandatory.

## Spaces, channels, and work

Spaces host free-form conversations. They may yield plans, decisions, backlog items, or nothing actionable. Channels ingest events and may trigger investigation or assignment through automations. Project-level work items can be assigned to teams or individuals, decomposed into subtasks, and linked to spaces, events, branches, and outcomes. No universal Grill/Plan/Implement stage sequence is required; policies may depend on work type and team.

## Reef context and prompt cleanup

Separately audit the existing channel/session prompt path: locally, an unexpectedly long system prompt was observed (roughly seven or eight screens). This is a reported symptom, not yet a diagnosed bug. Trace initial assembly, role/team instructions, memory injection, resumed sessions, and repeated turns; establish measured prompt sizes and duplication tests. Design one inspectable, bounded context assembler distinguishing instructions from retrieved knowledge. Do not conflate this effort with the new memory engine.

## Boundaries and unresolved questions

- What knowledge may move from project/work scope into a Teammate's general memory, and under whose approval?
- How are agent/team roles, authority, evidence, and access permissions represented?
- What are the precise Space, Team, Work and Channel lifecycle rules?
- How should automated routing and execution approval work?
- Which graph technology and retrieval strategy pass realistic evaluations?
- How will existing feature/channel data migrate, if this proposal is accepted?

## Proposed exploration sequence

1. Document and debate the domain model, especially memory authority and scopes.
2. Prototype a standalone graph-memory and retrieval service with mocked surrounding services and rigorous scenario-based evaluation; do not plug it directly into Reef.
3. Independently audit and test Reef's context/prompt assembly.
4. Define a narrow integration contract and acceptance criteria, then consider migration and implementation in separate follow-up proposals.

This folder records proposals only. It does not select a graph database, change existing data models, or authorize implementation.
