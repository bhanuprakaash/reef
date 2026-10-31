# Queue and review

A user queues work items that each have an owner (`ownerBotId`). Queuing an item starts it at once,
with its Space led by the owner (the other participants are members) and a first prompt that carries the
item's title, description, acceptance lines and brief; nothing limits how many items run at once. An item
with no owner cannot be queued. The lead splits the work into sub-items, the developer and the tester do
it, and the lead sets the sub-items done and submits the item for review with its proof. The user sends
the item back with a note, which restarts it in its Space, or sets it Done, which approves its wrap-up. A
restart starts what is queued. Contract: `docs/server.md` (Work items) and
`docs/restructure/decisions.md` (R8, R41, R42, R44, R47, R48).

## Sub-features

- `queue-first-starts` `POST /work-items/1/approve {"key":"k1"}` on an item with an owner answers `in_progress`, opens one Space for the item led by its owner, and the lead's turn replays the `queue-lead` recording.
- `queue-prompt` the first message of that Space starts `Work item 1: Login` and has the description and `Brief: sessions, not JWT`, and no acceptance line. The first prompt of the Space's first turn lists them: `Acceptance lines:`, then `1. [ ] wrong password is refused` and `2. [ ] session lasts a day`.
- `queue-second-runs` a second item queued while the first runs is `in_progress` at once, has a Space whose first message starts `Work item 2: Logout`, and `GET /inbox` has a `started` item for it, `Started from the queue: Logout`, with no `reef` note in its Space.
- `queue-needs-owner` approve on an item with no owner is 400 `needs an owner before it is approved`; the item stays `backlog` with no Space.
- `queue-needs-you-alone` a cap pause moves the paused item to `needs_you`; the other running item stays `in_progress`, before and after Continue.
- `queue-submit-review` the lead of a running item submits for review in a live turn (the scripted lead replays `submit-lead`, which calls `submit_for_review` through `reef mcp`): the item is `review`, and the other running item is still `in_progress`.
- `queue-submit-proof` after that submit `GET /work-items/1/proof` shows both lines ticked, the note `a wrong password gets 401` and the command `go test ./...` with its output, and `GET /spaces/1/proposal` is a pending `wrap` proposal: the lead's wrap-up ran when its turn ended.
- `queue-restart-starts` a running item submits through `reef mcp` while the server is down (no live turn) and an item is left `queued` in the database: after a restart the queued item is `in_progress` with a Space, and `GET /inbox` has a `started` item for it, with no `reef` note in its Space.
- `queue-restart-wraps` the submit of that item was never heard by a live server, so the restart runs its wrap-up: Space 2 gets a pending `wrap` proposal, and `PATCH {"status":"done"}` on the item approves it, closes the Space and sets the item `done`.
- `queue-review-keeps-space` `DELETE /spaces/2` while item 2 is in review is 409 and the Space stays.
- `queue-send-back-starts` `PATCH {"status":"queued","note":"SAY IT TWICE"}` on an item in `review` is 200: the item is `in_progress` again at once and keeps its one Space.
- `queue-send-back-note` once the sent-back item starts, the newest user message of its Space is `Sent back from review by the user:` and the note, the lead's turn replays `sent-back-lead`, and the wrap-up proposal it waited on is `rejected`.
- `flow-queue-starts` two items owned by the lead with the developer and tester as participants, item 1 queued: item 1 is `in_progress` with its one Space (members lead, developer, tester, the lead coordinating) and the first prompt `Work item 1: Checkout form`; item 2 is still `backlog` with no Space.
- `flow-team-works` the lead's reply `@developer build the form` wakes the developer, who writes `form.txt` in the item's worktree; its reply wakes the tester, who runs `grep` on it; the turns of the one Space are lead, developer, tester, lead.
- `flow-submit-review` the lead sets both sub-items `done` and submits: item 1 is `review`, `GET /work-items/1/proof` has the tick with its note and the command with its output, and the wrap-up proposal of the Space is `pending`.
- `flow-next-starts` queuing item 2 while item 1 is in review makes it `in_progress` at once with one Space whose first message is `Work item 2: Order history`; the inbox has one `started` FYI item for it.
- `flow-send-back` `PATCH {"status":"queued","note":"ADD A PHONE FIELD"}` on item 1 in review is 200: item 1 restarts at once (`in_progress`, or `review` again if the lead already resubmitted) next to the running item 2, its wrap-up is `rejected` and it has no second Space.
- `flow-resubmit` after item 2 is dropped, item 1 has heard the note in its own Space: the lead is woken with `Sent back from review by the user:` and the note and submits again, and a new wrap-up is `pending`.
- `flow-done` `PATCH {"status":"done"}` on item 1 in review is 200: its wrap-up is `approved`, a brief version is saved, its Space is closed and the item is `done`.

## How to get to it (user POV)

- In the window (Approve) or by `POST /work-items/{id}/approve` with a `key` on a top-level backlog item that has an owner (`ownerBotId`). A restart of `reef serve` starts what is queued.

## Driving it with verify-reef

`$H feature queue-and-review` runs the `queue-*` lines and prints one PASS/FAIL line per sub-feature. `$H feature work-item-flow flow` runs the `flow-*` lines, in order, on one server: a team `crew` (lead, developer, tester) from a file the script writes, one project, two work items owned by the lead with the developer and tester as participants. The scripted `claude` replays `flow-lead-start` (two `create_sub_item` calls, then `@developer build the form`), `flow-developer` (`Write` of `form.txt`, then `@tester check the form`), `flow-tester` (`Bash` `grep`, then `@lead the form passes`), `flow-lead-submit` (two `set_sub_item_status`, then `submit_for_review`), `queue-lead` for item 2 and `flow-lead-resubmit` for the sent-back note `ADD A PHONE FIELD`; a proposal's wrap-up replays `wrap-propose`. The sub-items are items 3 and 4, so the recordings name those ids.

Preconditions:

- Baseline from the README. `ANTHROPIC_MODEL=sonnet` on every `serve-start` (the script does it); the default fake `claude` replays `queue-lead` for the lead's first prompt (a prompt that starts `Work item N: `).
- The scratch cwd is the project; the script makes it a git repo with one commit. The owner is the seeded `lead` bot, id 1.
- In every Bash call set `H=.claude/skills/verify-reef/scripts/verify-reef` and `REEF_VERIFY_RUN=<id>`.

```sh
REEF_VERIFY_RUN=<id> $H feature queue-and-review
```

## Gotchas

- The server learns of a submit when the chain of turns of the submitting Space ends, because `reef mcp` is another process: `queue-submit-review` needs a live turn that submits (the scenario `submit-lead`: the user text `SUBMIT FOR REVIEW` sent to the lead). A submit from `mcp-call` starts nothing live; `queue-restart-starts` uses it on purpose while the server is down.
- The script sets the token budget on the work item (`work_item.token_budget`), where the cap pause reads it.
- A sent-back item already has its Space, so the queue starts it without opening another or sending the first prompt; the lead is woken in that Space with a user message that carries the note. A send back without a note is 400.
