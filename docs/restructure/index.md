# Reef restructuring proposal

> Status: done. The domain model, memory service and context assembler are built, and their decisions are in [docs/product.md](../product.md#decisions) (the R to number table is in [decisions.md](decisions.md)). This folder is history: it says Feature and channel where Reef did then. What is still open is under Open questions below.

## Contents

1. [Why restructure?](#why-restructure)
2. [Domain concepts](#domain-concepts)
3. [Ask a Teammate](#ask-a-teammate)
4. [Channels and automation](#channels-and-automation)
5. [Memory and retrieval service](memory-and-retrieval.md)
6. [Reef context and prompt cleanup](#reef-context-and-prompt-cleanup)
7. [Open questions](#open-questions)
8. [Exploration sequence](#exploration-sequence)

## Why restructure?

Reef centers its workflow and memory on a Feature and its channels, roles, notes and stages. Real work is broader: investigations, hotfixes, incidents, bugs, maintenance, ideas, features. A Feature should not be a prerequisite for collaboration or execution (R3).

We want persistent, individually recognizable Teammates that work across Teams and Projects without carrying unrelated context.

## Domain concepts

- **Teammate:** one persistent AI collaborator with expertise, instructions, tools and history. **Agent** stays the internal term; **employee** is not used.
- **Direct conversation:** one chat with one Teammate, no project, team or Work item attached. Many per Teammate. Becomes a Space when a second Teammate is added (R9).
- **Team:** roster, routing rules, responsibility text, queue, coordination memory (R4). A Teammate may sit in several Teams.
- **Project:** a repository boundary with its own technical facts, conventions and decisions.
- **Space:** where talk happens; tied to a Work item or free (R5).
- **Work item:** typed board entry (feature, bug, incident, investigation, task, improvement) with sub-items, status, proof, assignee and links to source Spaces (R10, R11).
- **Channel:** incoming event stream (Slack, alerts, webhooks), distinct from Spaces.
- **Automation:** receives events, dedupes and routes them. An incoming message is data, not an instruction.
- **Ask a Teammate:** a bounded question from one Teammate to another, answered in the asking chat.

## Ask a Teammate

From a conversation or Space, a Teammate asks another a bounded question and reports the attributed reply. Example: Alex (architect) asks Sam (developer) why Redis was chosen.

- Recorded as a linked consultation: requester, recipient, question, minimal context, reply. Not a new conversation under the recipient.
- The recipient gets a summary, never the source transcript by default.
- Depth, duration and cost are capped; no Teammate-to-Teammate loops. Open: the exact limits.
- **Ask** is for clarification; **add to Space** (R9) is for ongoing discussion; **Work item** is for an outcome.

## Channels and automation

Not yet discussed. Direction: a Channel ingests events; an Automation may propose a Work item or an investigation, never create or execute on its own.

## Reef context and prompt cleanup

Locally the channel prompt reached roughly seven or eight screens. Diagnosed on the Mac database: it was the wakes, not the first prompt. One wake was 90,755 characters because every wake resent the whole thread; channel-fixes stopped that (0 duplicate characters in the eval). The first prompt was 3,300 to 3,700 characters: persona about 1,300, team rules about 1,100, feature context about 900.

The context assembler (R31–R36) is still worth building: one place for the first prompt in three sections, a count line instead of unbounded lists, and a delta so a resumed role learns of new decisions and a new brief. It reads today's tables; the memory slice swaps the sources (see [memory-and-retrieval.md](memory-and-retrieval.md) "Memory slice").

## Open questions

- Memory: settled as R15–R30.
- Ask a Teammate: depth, duration and cost limits.
- Space continuation: what triggers a new Space for the same Work item.
- Sub-item cap: whether a Team may create at most N sub-items under one parent without a Review. Deferred.
- Channel and Automation lifecycle and approval.

## Exploration sequence

1. Settle the domain model. In progress; see decisions.md.
2. Prototype a standalone memory and retrieval service with mocked surroundings and scenario-based evaluation.
3. Audit and test Reef's prompt assembly. Plan: [channel-fixes.md](channel-fixes.md).
4. Define the integration contract and acceptance criteria, then implement.
