# Caps

A Space's agents cannot run away. Teammates that only talk to each other pause the Space after 6 hops, a
teammate that works (Edit, Bash, any tool other than Reef's own) resets the count, and every turn of a work item,
in any of its Spaces, counts against one token budget. A pause moves the work item to Needs you and the inbox says
why; the user's Continue clears it. Contract: `docs/server.md` (Caps) and `docs/restructure/decisions.md` (R49, R54).

## Sub-features

- `hop-work-resets` a developer whose turns call `Edit` and a tester whose turns call `Bash`, each handing back to the other, run 12 hops and the Space is not paused: a tool call other than Reef's own resets the count (`$H feature work-item-flow caps`).
- `hop-talk-pauses` a lead and a developer whose turns only call `post_to_space` pause after 6 hops with `hop cap: 6 agent hops without a user message`; the work item goes to `needs_you` and `GET /inbox` has a `held` item for the Space (`$H feature work-item-flow caps`).
- `budget-per-item` two Spaces of one work item count their turns against one budget, the work item's `tokenBudget`; `continue` raises that budget.

The other lines that hold a cap are `hop-cap` and `restart-held` (`teammates.md`), `live-caps` (`spaces.md`) and
`queue-needs-you-holds` (`queue-and-review.md`).

## How to get to it (user POV)

- A Space of a Team that keeps waking itself shows the banner `Paused: hop cap: 6 agent hops without a user message`; the work item sits in Needs you in the inbox and on the board. Continue runs the held wake.
- A work item that used its token budget refuses the next message with the reason; Continue raises the budget by 1,000,000.

## Driving it with verify-reef

`$H feature work-item-flow caps` runs the first two sub-features and prints one PASS/FAIL line each.

Preconditions: baseline from the README. `ANTHROPIC_MODEL=sonnet` on `serve-start`; `$S/cwd` a git repo with one commit.

Hop count and tool calls, a folder of its own (`--data-name hop`) with the team `full` (`$H reef --data-name hop -- team apply $PWD/teams/full.json`) and `$S/cwd` a repo with one commit. The fake scenarios are `work-lead`, `work-developer`, `work-tester`, `talk-lead` and `talk-developer`:

- **Setup.** `$H api POST /projects '{"name":"hop","path":"'$S'/cwd"}'`, then three work items `workpong`, `talkpong` and `budget` (ids 1 to 3). `$H sql --write "update work_item set status = 'in_progress' where id in (1, 2)"`. `$H api POST /spaces '{"projectId":1,"workItemId":N,"name":"sN","team":"full"}'` for N 1 and 2 (Space ids 1 and 2).
- **Work resets the count.** `$H api POST /spaces/1/send '{"text":"workpong"}'` (the lead answers `@developer work`, the developer calls `Edit` and answers `@tester check`, the tester calls `Bash` and answers `@developer again`, and so on). When `$H sql "select count(*) from turn where space_id = 1 and status = 'done'"` is 13, `$H api POST /spaces/1/stop`. `$H sql "select role, status from turn where space_id = 1 order by id"` is `lead`, then `developer` and `tester` alternating, 13 `done` rows (the lead's answer and 12 hops), and `$H sql "select paused from space where id = 1"` is empty. `$H sql "select turn.role, t.name, count(*) from tool_call t join turn on turn.id = t.turn_id where turn.space_id = 1 group by turn.role, t.name"` is `developer|Edit` and `tester|Bash` six times each.
- **Talk only pauses at 6.** `$H api POST /spaces/2/send '{"text":"talkpong"}'`, then `wait_for /spaces/2 '"paused":"hop cap'`. `$H sql "select role, status from turn where space_id = 2 order by id"` is seven `done` rows, `lead` and `developer` alternating from `lead`; the only tool in `tool_call` is `mcp__reef__post_to_space`. `$H sql "select status from work_item where id = 2"` is `needs_you`. `$H api GET /inbox` is one item `{"kind":"held","group":"needs_you","spaceId":2,"workItemId":2,"text":"hop cap: 6 agent hops without a user message (1 wake held)",...}`.
- **One budget per work item.** `$H api POST /spaces '{"projectId":1,"workItemId":3,"name":"first","team":"full"}'` (id 3), `$H sql --write "update space set closed_at = '2026-10-27T00:00:00Z' where id = 3"`, `$H api POST /spaces '{"projectId":1,"workItemId":3,"name":"second","team":"full"}'` (id 4). `$H sql --write "insert into turn (space_id, role, backend, status, input_tokens, started_at, ended_at) values (3, 'lead', 'claude', 'done', 700000, '2026-10-27T00:00:00Z', '2026-10-27T00:00:01Z'), (4, 'lead', 'claude', 'done', 400000, '2026-10-27T00:00:02Z', '2026-10-27T00:00:03Z')"`. Neither Space is over 1,000,000 alone. `$H api POST /spaces/4/send '{"text":"Say ok in one word"}'` is 409 `space 4 is paused: token budget of 1000000 used: 1100000 tokens: conflict`. `$H api POST /spaces/4/continue` is 204; `$H api GET /work-items/3` and `GET /spaces/4` have `"tokenBudget":2100000`, and a send runs a turn.

## Gotchas

- The hop cap counts talk-only turns woken by an agent, not the turn that answers the user, so a pause happens after `1 + 6` turns. A tool call other than Reef's own (`mcp__reef__*`, except `mcp__reef__browser_*`) resets the count, so recordings of working roles never reach the cap.
- `$H feature work-item-flow caps` stops Space 1 itself once it has 13 done turns; the working pair would otherwise run for ever. It checks the first 13 done turns, because a 14th may finish before the stop lands.
- The hop count lives in the server's memory; the pause and the wakes it holds are stored.
- The scripted `claude` replays `work-lead`, `work-developer`, `work-tester`, `talk-lead` and `talk-developer` (`fake/scenario.go`).
