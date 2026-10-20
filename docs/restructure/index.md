# Reef restructuring proposal

> Status: exploratory design discussion. This is a direction of travel, not an approved implementation specification. No existing Reef behavior is changed by these documents.

## Contents

1. [Why restructure?](#why-restructure)
2. [Proposed domain concepts](#proposed-domain-concepts)
3. [Teammates and teams](#teammates-and-teams)
4. [Direct conversations without mandatory context association](#direct-conversations-without-mandatory-context-association)
5. [Ask a Teammate](#ask-a-teammate)
6. [Spaces, teams, and the global Work Board](#spaces-teams-and-the-global-work-board)
7. [Memory and retrieval service](memory-and-retrieval.md)
8. [Reef context and prompt cleanup](#reef-context-and-prompt-cleanup)
9. [Boundaries and unresolved questions](#boundaries-and-unresolved-questions)
10. [Proposed exploration sequence](#proposed-exploration-sequence)

## Why restructure?

Reef currently centers much of its workflow and memory on a Feature and its channels, roles, notes, and stages. Real company work is broader: investigations, hotfixes, incidents, bugs, maintenance, ideas, and features. A Feature should not be a prerequisite for collaboration or execution. A feature can remain an optional kind of work, rather than the universal container.

We want persistent, individually recognizable Teammates that can collaborate across teams and projects without leaking unrelated context.

## Proposed domain concepts

- **Teammate:** The chosen user-facing term for one persistent individual AI collaborator, with expertise, instructions, tools, scripts, capabilities, and history. **Agent** may remain an internal technical term. Do not use **employee** in the product model; users should not need to understand templates or runtime instances.
- **Direct conversation:** One independent, persistent chat session with one Teammate. A Teammate can have many conversations, each with its own history. It has no mandatory project, team, feature, work-item, or project-ID association.
- **Team:** A composition of existing Teammates, with responsibilities, coordination rules, a shared work queue and collective experience. A Teammate may join multiple teams without being cloned. Its value is coordinated ownership, not exclusive technical capability.
- **Project:** A repository or project boundary with its own durable technical knowledge, decisions, and conventions.
- **Space:** A persistent place to invite Teammates or teams and discuss, investigate, brainstorm, and plan. A discussion need not create work or follow prescribed stages.
- **Work item:** Globally visible/tracked board entry, optionally related to one or more projects and typed as investigation, incident, bug, task, improvement, feature, etc. Supports parent/child items, assignment or pickup by a Team or individual Teammate, and links to source discussions and outcomes. The board is a shared workflow, not a mandatory entry point for chatting.
- **Channel:** Event/integration stream, distinct from conversational Spaces; e.g. Slack messages, infra alerts, webhooks.
- **Automation:** Polls or receives events, deduplicates and routes them under explicit permissions; an incoming message is data, not automatically an instruction to execute.
- **Consultation / Ask a Teammate:** A bounded question from one Teammate to another, linked to the originating conversation, with an inspectable attributed reply. This is not a work assignment or a new user-visible direct chat.

Names and precise boundaries remain open to discussion.

## Teammates and teams

Create Teammates independently, then add them to teams. A single Teammate may participate in several teams with different responsibilities. The visible model is one identity, many memberships; execution sessions and working contexts are isolated internally. A Teammate's general knowledge does not imply automatic access to all prior teams' private context. Teams may remember collective handoffs and operating lessons without copying entire project histories.

Team templates are not a required initial user-facing concept. Direct conversations do not require a team or Space. A Teammate's skills and tool capabilities are distinct from primary responsibilities and execution permissions. For sustained multi-specialist delivery, a Teammate may suggest a Work Item and suitable Team, while still handling appropriate authorised work individually. Delegation is not compulsory solely because of a role title.

## Direct conversations without mandatory context association

- Open a Teammate directly and create any number of separate, resumable ordinary chat sessions. A new conversation does not clone the Teammate.
- A conversation is just a conversation: do not require or persist a project selector, project-ID list, team, feature, work-item link, or predefined context scope as part of the direct-conversation model.
- A user may discuss any topic, including one project, multiple projects, or none, and change topics naturally in the same conversation.
- The Teammate can request relevant memory and traces on demand based on the user's question and the ongoing conversation. Retrieval resolves entities, relevance, provenance and permissions; it must not assume that every project mentioned is authorised or inject whole project histories.
- The conversation transcript is history, not automatically permanent personal memory. Extracted findings retain their own proper source, role, scope and access policy in the memory graph.
- Conversations can yield work or lead to collaborative Spaces, but neither is mandatory.

## Ask a Teammate

From an existing direct conversation, a Teammate may ask another Teammate a relevant bounded question and bring the answer back without requiring the user to switch chats, create a Space or create a Work Item. For example, Alex (Architect) can ask Sam (Developer) why Redis was selected, then report Sam's attributed reply to the user.

- Record a linked, inspectable consultation with requester, recipient, question, minimal necessary context, reply, and provenance; do not silently create a new ordinary direct conversation under the recipient.
- The recipient only receives information authorised for that request; never forward the entire source transcript by default or bypass memory access policy.
- Limit consultation depth, duration and cost, and prevent uncontrolled Teammate-to-Teammate loops. Further delegation or execution needs appropriate policy.
- **Ask** is for clarification; **Space** is for ongoing multi-party discussion; **delegate/assign** is for an outcome requiring execution and usually tracked as a Work Item.
- A consultation can suggest inviting participants to a Space or proposing Work, but does not automatically create either.

## Spaces, teams, and the global Work Board

Users may start anywhere: talk directly to a Teammate, create a Space and invite relevant Teammates or Teams, message a Team through a Space, or create Work directly. A Space is for free-form collaboration and may produce plans, decisions, backlog entries or nothing actionable. Channels ingest events and automations may propose investigation or assignment.

The **global Work Board** tracks durable outcomes from direct conversations, Spaces, Teams, channels, or user entry. Individuals and Teams can propose backlog entries; subject to permissions and user intent, entries can be assigned to a Teammate or Team, or picked up by a Team. A Team coordinates multi-person delivery, breakdown, handoffs, verification and collective ownership. The Work Item keeps its history when ownership changes; the source conversation or Space remains linked evidence, not the task container.

A Teammate can investigate, plan and perform authorised small fixes directly. Their configured **primary responsibility** informs when to recommend a Work Item and suitable Team for broader delivery, but does not artificially disable their technical capabilities. Proposing Work, creating it, assigning it, and executing it are separate actions governed by permissions and approval policy. No universal Grill/Plan/Implement sequence is required.

## Reef context and prompt cleanup

Separately audit the existing channel/session prompt path: locally, an unexpectedly long system prompt was observed (roughly seven or eight screens). This is a reported symptom, not yet a diagnosed bug. Trace initial assembly, role/team instructions, memory injection, resumed sessions, and repeated turns; establish measured prompt sizes and duplication tests. Design one inspectable, bounded context assembler distinguishing instructions from retrieved knowledge. Do not conflate this effort with the new memory engine.

## Boundaries and unresolved questions

- What knowledge may move from project/work scope into a Teammate's general memory, and under whose approval?
- How are agent/team roles, authority, evidence, and access permissions represented?
- What are the precise Space, Team, Work and Channel lifecycle rules?
- When may a Teammate propose versus autonomously create/assign Work, and when may Teams pick up backlog items?
- What are the consultation limits, consent rules and escalation path from a question to a Space or Work Item?
- How should automated routing and execution approval work?
- Which graph technology and retrieval strategy pass realistic evaluations?
- How will existing feature/channel data migrate, if this proposal is accepted?

## Proposed exploration sequence

1. Document and debate the domain model, especially memory authority and scopes.
2. Prototype a standalone graph-memory and retrieval service with mocked surrounding services and rigorous scenario-based evaluation; do not plug it directly into Reef.
3. Independently audit and test Reef's context/prompt assembly.
4. Define a narrow integration contract and acceptance criteria, then consider migration and implementation in separate follow-up proposals.

This folder records proposals only. It does not select a graph database, change existing data models, or authorize implementation.
