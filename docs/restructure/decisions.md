# Restructure decisions

Settled in discussion, 2026-10-23. Numbered R1… so they do not collide with `docs/product.md`; they move there when implementation starts. Re-open one only with a new numbered decision and a reason.

| # | Decision | Why |
| --- | --- | --- |
| R1 | Reef stays single-user. The one user creates every Teammate, Team, Space and Work item | Nobody shares a Reef, so scopes are retrieval filters for relevance, not access control |
| R2 | Memory service first; the prompt-assembly audit runs alongside it | Memory is the hardest open problem and the assembler is where memory plugs in |
| R3 | Feature stops being an object. It becomes one Work item type among bug, incident, investigation, task, improvement | Work is broader than features; one container, typed |
| R4 | A Team is a roster of Teammates plus routing rules, a responsibility text, a queue and coordination memory. It has no chat of its own | "Who works together and how" stays separate from "where talking happens" |
| R5 | A Space is where talk happens. A Work item has one active Space, plus continuations when one grows too long. Free Spaces with no Work item are allowed | One place holds the whole story of a piece of work |
| R6 | Phases (design, implementation, testing) are threads inside the Space, or sub-items. Never a Space per phase | Splitting phases splits context and costs fresh sessions |
| R7 | A Teammate proposes a Work item and suggests a Team from its responsibility text and the project's default team. The user creates and assigns. The Team's lead picks it up and opens the Space | Teammates never assign; the board stays the user's |
| R8 | Serial: one Work item per Team at a time. The top of the queue is next | Matches one-ticket-at-a-time and the single-writer rule |
| R9 | A direct conversation is with one Teammate. Typing `@Y` or "add Teammate" turns it into a Space with the user, X and Y; the conversation is marked moved and linked; Y gets a summary, never the transcript. Ask a Teammate stays in the chat | Adding someone is one motion; the two gestures stay distinct |
| R10 | Work item statuses: Backlog, Queued, In progress, Needs you, Review, Done, Dropped. Only the user sets Done. Journey stages are dropped | The Team gets to Review; acceptance is the user's |
| R11 | Proof is a field on the Work item: PR, test command and output, screenshots, ticked acceptance lines. The lead fills it at Review. A parent's status and proof derive from its children plus one end-to-end check | Verification becomes first-class instead of a comment |
| R12 | Approval gates are removed. Caps, checkpoints with rollback and Needs you remain | Review with proof checks the result; gates checked intent and stalled the queue |
| R13 | A Team creates sub-items under its Work item without approval; they stay with the same Team and surface as FYI. Inbox kinds: Needs you (blocks), Review (blocks that item), FYI (does not block) | Work moves while the user is away; the user can still drop or reassign |
| R14 | Clean break. Today's Features and channels are not migrated | Nobody uses Reef yet |
