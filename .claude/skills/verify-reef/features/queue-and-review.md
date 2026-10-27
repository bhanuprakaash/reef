# Team queue and review

A user queues work items on a Team. The Team works one top-level item at a time: the first
queued item starts at once, with its Space and a first prompt that carries the item's title,
description, acceptance lines and brief. The lead splits the work into sub-items, the developer
and the tester do it, and the lead sets the sub-items done and submits the item for review with its
proof. Review frees the Team, so the next queued item starts by itself. The user sends the item back
with a note, which puts it first in the queue, or sets it Done, which approves its wrap-up. An item
that needs the user keeps the Team busy. A restart starts a queued item of a free Team. Contract:
`docs/server.md` (Work items) and `docs/restructure/decisions.md` (R8, R41, R42, R44, R47, R48).

## Sub-features

- `queue-first-starts` `PATCH /work-items/1 {"status":"queued"}` on a Team with nothing running answers `in_progress`, opens one Space for the item with the Team's lead, and the lead's turn replays the `queue-lead` recording.
- `queue-prompt` the first message of that Space starts `Work item 1: Login` and has the description, each acceptance line numbered, and `Brief: sessions, not JWT`.
- `queue-second-waits` a second item queued on the same Team stays `queued` at position 1 and has no Space.
- `queue-needs-you-holds` a cap pause moves the running item to `needs_you`; the queued item stays queued with no Space, before and after Continue.
- `queue-submit-starts` the lead of the running item submits for review in a live turn (the scripted lead replays `submit-lead`, which calls `submit_for_review` through `reef mcp`): the item is `review`, and with no restart and no `PATCH` the queued item is `in_progress`, has a Space whose first message starts `Work item 2: Logout`, and `GET /inbox` has a `started` item for it, `Started from the queue: Logout`, with no `reef` note in its Space.
- `queue-submit-proof` after that submit `GET /work-items/1/proof` shows both lines ticked, the note `a wrong password gets 401` and the command `go test ./...` with its output, and `GET /spaces/1/proposal` is a pending `wrap` proposal: the lead's wrap-up ran when its turn ended.
- `queue-restart-starts` the running item submits through `reef mcp` while the server is down (no live turn) with one item queued behind it: the store starts the queued item at once, and after a restart it has a Space, and `GET /inbox` has a `started` item for it, with no `reef` note in its Space.
- `queue-restart-wraps` the submit of that item was never heard by a live server, so the restart runs its wrap-up: Space 2 gets a pending `wrap` proposal, and `PATCH {"status":"done"}` on the item approves it, closes the Space and sets the item `done`.
- `queue-review-keeps-space` `DELETE /spaces/2` while item 2 is in review is 409 and the Space stays.
- `queue-send-back-first` `PATCH {"status":"queued","note":"SAY IT TWICE"}` on an item in `review` puts it at position 1 and moves the item queued before it to 2.
- `queue-send-back-starts` when the running item is dropped, the sent-back item starts, ahead of the other queued item, and keeps its one Space.
- `queue-send-back-note` once the sent-back item starts, the newest user message of its Space is `Sent back from review by the user:` and the note, the lead's turn replays `sent-back-lead`, and the wrap-up proposal it waited on is `rejected`.
- `flow-queue-starts` two items queued on one Team: item 1 is `in_progress` with its one Space and the first prompt `Work item 1: Checkout form`; item 2 is `queued` at position 1 with no Space.
- `flow-team-works` the lead's reply `@developer build the form` wakes the developer, who writes `form.txt` in the item's worktree; its reply wakes the tester, who runs `grep` on it; the turns of the one Space are lead, developer, tester, lead.
- `flow-submit-review` the lead sets both sub-items `done` and submits: item 1 is `review`, `GET /work-items/1/proof` has the tick with its note and the command with its output, and the wrap-up proposal of the Space is `pending`.
- `flow-next-starts` with no `PATCH` and no restart, item 2 is `in_progress` and has one Space whose first message is `Work item 2: Order history`; the inbox has one `started` FYI item for it.
- `flow-send-back` `PATCH {"status":"queued","note":"ADD A PHONE FIELD"}` on item 1 in review is 200: item 1 is `queued` at position 1 behind the running item 2, its wrap-up is `rejected` and it has no second Space.
- `flow-resubmit` when item 2 is dropped, item 1 starts in its own Space; the lead is woken with `Sent back from review by the user:` and the note and submits again, and a new wrap-up is `pending`.
- `flow-done` `PATCH {"status":"done"}` on item 1 in review is 200: its wrap-up is `approved`, a brief version is saved, its Space is closed and the item is `done`.

## How to get to it (user POV)

- In the window or by `PATCH /work-items/{id}` with `{"status":"queued"}` on a top-level item that has a Team (`teamId`). A restart of `reef serve` starts what is queued on a free Team.

## Driving it with verify-reef

`$H feature queue-and-review` runs the `queue-*` lines and prints one PASS/FAIL line per sub-feature. `$H feature work-item-flow flow` runs the `flow-*` lines, in order, on one server: a team `crew` (lead, developer, tester) from a file the script writes, one project, two work items. The scripted `claude` replays `flow-lead-start` (two `create_sub_item` calls, then `@developer build the form`), `flow-developer` (`Write` of `form.txt`, then `@tester check the form`), `flow-tester` (`Bash` `grep`, then `@lead the form passes`), `flow-lead-submit` (two `set_sub_item_status`, then `submit_for_review`), `queue-lead` for item 2 and `flow-lead-resubmit` for the sent-back note `ADD A PHONE FIELD`; a proposal's wrap-up replays `wrap-propose`. The sub-items are items 3 and 4, so the recordings name those ids.

Preconditions:

- Baseline from the README. `ANTHROPIC_MODEL=sonnet` on every `serve-start` (the script does it); the default fake `claude` replays `queue-lead` for the lead's first prompt (a prompt that starts `Work item N: `).
- The scratch cwd is the project; the script makes it a git repo with one commit. The Team is the seeded `solo` team, id 1.
- In every Bash call set `H=.claude/skills/verify-reef/scripts/verify-reef` and `REEF_VERIFY_RUN=<id>`.

```sh
REEF_VERIFY_RUN=<id> $H feature queue-and-review
```

## Gotchas

- The server learns of a submit when the chain of turns of the submitting Space ends, because `reef mcp` is another process: `queue-submit-starts` needs a live turn that submits (the scenario `submit-lead`: the user text `SUBMIT FOR REVIEW` sent to the lead). A submit from `mcp-call` starts nothing live; `queue-restart-starts` uses it on purpose while the server is down.
- The script sets the token budget on the work item (`work_item.token_budget`), where the cap pause reads it.
- A sent-back item already has its Space, so the queue starts it without opening another or sending the first prompt; the lead is woken in that Space with a user message that carries the note. A send back without a note is 400.
