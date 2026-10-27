# reef chat

A user runs one role on a work item in the terminal with `reef chat`. The first message
carries the context prompt. Each line is a stored message and a stored turn, and the
agent's reply is stored as a message. `/wrap` makes the agent propose a new brief and memory
items through `reef mcp`; the user approves (items first, then the brief and the Space
close in one transaction; a failure part-way leaves it pending) or rejects (nothing saved). `reef chat --space ID` resumes the session. Ctrl-C stops the
turn. The chat's Space is in team `solo` with its lead `lead` (`--role` may only name it), and `reef chat`
routes nothing. Contract: `docs/memory.md`.

## Sub-features

- `chat-turn` one piped line becomes a `user` message, a `done` turn with a session id and usage, and a reply message from the role.
- `chat-context` the first prompt starts with the context prompt (`--debug` shows it on stderr).
- `chat-tools` the agent calls the `reef` tools (`get_brief`, `remember`, `propose_wrap`) from inside the chat.
- `chat-wrap-approve` `/wrap` then `y`: the agent calls `propose_wrap`, the proposal prints, and approve adds a brief version and the proposed items as memory rows (a team item as a candidate), and marks the proposal `approved`.
- `chat-wrap-typed-ahead` a line typed while the agent wraps up prints `ignored while wrapping: <line>` and is not the answer.
- `chat-resume` `reef chat --space ID` resumes the same session id with the same role and backend.
- `chat-wrap-reject` `/wrap` then `n`: the proposal is `rejected` and no brief or memory item changes.
- `chat-wrap-eof` stdin ends at `approve? [y/N]`: the proposal stays `pending` with `proposal N left pending`.
- `chat-wrap-cli` `reef wrap list`, `show`, `approve` and `reject` manage pending proposals outside chat; deciding one twice fails.
- `chat-wrap-sigint` Ctrl-C at `approve? [y/N]`: the proposal stays `pending`, it prints `proposal N left pending` and the resume line, and exits 0.
- `chat-wrap-dedupe` approve (`y` or `yes`) skips a proposed item whose exact body is already accepted or candidate in its scope, so an item saved live with `remember` and proposed again is saved once.
- `chat-name` `--name N` names a new Space; the default is `<role> chat`.
- `chat-errors` a usage error, `--space 99` and a Space with no backend fail with `reef: ...` and exit 1, before an agent starts.
- `chat-lock` a second `reef chat` on an open Space exits 1 with `space N is open elsewhere: conflict`.
- `chat-serve-skip` `reef serve` starting while a chat runs leaves that chat's running turn alone and fails only stale turns.
- `chat-interrupt` Ctrl-C during a turn stores it `failed` with `interrupted`, prints the resume line, exits 0 and leaves no processes.
- `chat-codex` new, resume, wrap-up and Ctrl-C work the same with `--backend codex`.
- `chat-token-cleanup` every chat deletes its token when it ends.
- `chat-permission-mode` `--permission-mode acceptEdits` is stored on the Space, and `reef chat --space ID` resumes with it; `--permission-mode` on `--space` is refused.
- `chat-quick-session` a Space with no work item starts with a two-line prompt that names the project and says there is no work item yet.
- `chat-bad-backend` a backend typo fails and leaves no Space (the Space made for it is removed).
- `chat-background-task` a background task that ends while a later turn runs adds nothing to that turn's stored reply or to the next one's.
- `chat-paused` `reef chat --space ID` on a paused Space, or one whose token budget is used, still opens; a line typed there is not run or stored and prints `space N is paused: <reason>` and `/continue to go on`; a used-up budget is stored as the pause; `/continue` clears it and the next line runs.
- `chat-index` the context index cuts a decision's first line at 100 runes with `…` and counts the Spaces with messages.

## How to get to it (user POV)

- In the project folder: `reef chat 1 --backend claude`, type lines, `/wrap`, `y`, Ctrl-D.
- Later: `reef chat --space 1`.

## Driving it with verify-reef

`$H reef --stdin FILE --timeout S -- chat ...` runs `reef chat` in the scratch cwd with
FILE as stdin. At the end of FILE the chat exits by itself. `--timeout S` (default 240)
sends SIGINT after S seconds, which is Ctrl-C. The result line has `children:` (the agent
and its `reef mcp`) and `orphans_alive:`.

Lines already waiting on stdin when the wrap turn ends are ignored, so a plain file with
`/wrap` then `y` never approves. Add `--answer A`: the helper writes FILE's lines through
a FIFO and keeps it open, polls the live output for `approve? [y/N]`, then writes `A` and
closes stdin. On `no proposal made` it closes without an answer. The result adds `feed:
sent "y" after approve? [y/N]` (or why it sent nothing), also in `reef.log`.
`--sigint-at-prompt` instead of `--answer` sends SIGINT to `reef` at `approve? [y/N]` and
keeps stdin open until `reef` exits, so the exit comes from Ctrl-C, not end of input. The
result adds `feed: sent SIGINT after approve? [y/N]`.

Preconditions:

- Baseline from the README. No server needed. Every claude chat drive sets `ANTHROPIC_MODEL=sonnet`. A chat on a work item runs in the work item's worktree `<data folder>/worktrees/<project id>/<work item id>`, so make the scratch cwd a git repo with one commit first (README, Baseline preconditions).
- Setup: `$H reef --data-name chat -- project add` (project 1 is the scratch cwd, so the agent runs there), `$H reef -- work-item add "Email login" --description "Users sign in with email and password"` (work item 1).
- Input files in `~/.reef-verify/<id>/`, one line each:
  - `chat-in.txt`: `Call the reef get_brief tool first. Then note: we decided to lock an account after 5 failed logins, to slow down guessing. Reply in one short sentence.`, `/wrap`, `typed-ahead line`.
  - `resume-in.txt`: `What did we decide about failed logins? One sentence.`, `/wrap`.
  - `wrap-eof.txt`: `/wrap`.
  - `long-in.txt`: `Write a 3000-word essay on the history of lighthouses. Do not use any tools.`
  - `codex-in.txt`: `Call the reef get_brief tool, then save a decision with the reef remember tool: kind decision, body 'Unlock locked accounts by email reset', why 'No support staff'. Reply in one short sentence.`
  - `codex-resume-in.txt`: `Say only: ok-codex-2`, `/wrap`.

Claude, Space 1:

- **Turn, context, typed-ahead and wrap approve.** `ANTHROPIC_MODEL=sonnet $H reef --stdin ~/.reef-verify/<id>/chat-in.txt --answer y --timeout 300 -- chat 1 --backend claude --debug`. stderr starts with `--- prompt ---`, `## You` and `You are the lead on work item "Email login" (1) in project "cwd".`. stdout has the reply, `proposal 1 from lead`, `brief:`, `items:` (`- decision (work_item): lock an account after 5 failed logins (slows down password guessing)`, `- fact (project): login lives at /login`, `- lesson (teammate): answer in one short sentence`), `ignored while wrapping: typed-ahead line`, `approve? [y/N] approved: brief and N items saved`, and `space 1: resume with reef chat --space 1`. Result `feed: sent "y" after approve? [y/N]`, `exit: 0`, `orphans_alive: none`. Then:
  - `$H sql "select id, author, substr(body, 1, 50) from message"`: `user`, `lead`, `user` (`/wrap`), `lead`. The typed-ahead line is not stored.
  - `$H sql "select id, role, backend, status, session_id != '' as has_session, input_tokens + cache_read_tokens > 0 as has_usage from turn"`: two rows `lead|claude|done|1|1`.
  - `$H sql "select id, role, status, decided_at is not null from proposal"`: `1|lead|approved|1`.
  - `$H sql "select count(*) from token"`: `0`, the chat deleted its token when it ended.
  - `$H sql "select c.role, t.name from space c join team t on t.id = c.team_id"`: `lead|solo`: the Space is in team `solo` with its lead.
  - `$H sql "select author, substr(body, 1, 40) from brief"`, `$H sql "select id, kind, scope_kind || ':' || scope_id, author, status from memory"`: a brief by `lead`, and the proposed items by `lead`, `accepted`, in `work_item:1`, `project:1` and `teammate:1` (the id of the solo lead's Teammate).
  - Tool calls: `grep -o '"name":"mcp__reef__[a-z_]*"' ~/.claude/projects/<key>/<session id>.jsonl` lists `mcp__reef__get_brief`, `mcp__reef__propose_wrap` and usually `mcp__reef__remember`. The session id is the turn's `session_id`. `<key>` here is the worktree path `<scratch>/chat/worktrees/1/1` with every character outside A-Z a-z 0-9 turned into `-`, not the scratch cwd. Grep before `cleanup`; it does not remove that folder (`rm -rf ~/.claude/projects/-tmp-reef-verify-<id>-*`, see `full-team.md`, Gotchas).
- **Resume and reject.** `ANTHROPIC_MODEL=sonnet $H reef --stdin ~/.reef-verify/<id>/resume-in.txt --answer n --timeout 300 -- chat --space 1`. The reply names the 5 failed logins (the session remembers; on the fake it replays `recall-failed-logins`). stdout has `proposal 2 from lead` and `rejected: nothing changed`; result `feed: sent "n" after approve? [y/N]`. `$H sql "select id, session_id, status from turn"`: four `done` turns; turns 1 and 3 share one session id, and each wrap-up turn (2 and 4) has its own, since it runs in a fork of the session (`claude --fork-session`). `$H sql "select id, status from proposal"`: `1|approved`, `2|rejected`. `$H sql "select count(*) from brief"` is still 1.
- **End of input at the prompt and CLI approval.** `ANTHROPIC_MODEL=sonnet $H reef --stdin ~/.reef-verify/<id>/wrap-eof.txt --timeout 300 -- chat --space 1` (no `--answer`). stdout has `proposal 3 from lead`, `approve? [y/N]`, then `proposal 3 left pending` and the resume line, `exit: 0`. `$H sql "select id, status from proposal where id = 3"`: `3|pending`. `$H reef -- wrap list` lists proposal 3; `$H reef -- wrap show 3` prints its brief, items and retire ids; `$H reef -- wrap approve 3` approves it. `$H reef -- wrap approve 3` again prints `reef: invalid: proposal 3 is already approved`, `exit: 1`. `$H sql "select id, status from proposal where id = 3"`: `3|approved`.
- **Interrupt.** `ANTHROPIC_MODEL=sonnet $H reef --stdin ~/.reef-verify/<id>/long-in.txt --timeout 12 -- chat --space 1`. Result `exit: 0 ... sigint_after_s: 12 ... orphans_alive: none`, stdout ends with `space 1: resume with reef chat --space 1`. `$H sql "select id, status, error, ended_at is not null from turn where id = 6"`: `6|failed|interrupted|1`.

Codex, Space 2 (no `ANTHROPIC_MODEL` needed):

- **New codex chat with tool calls.** `$H reef --stdin ~/.reef-verify/<id>/codex-in.txt --timeout 300 -- chat 1 --backend codex --debug`. stderr has the context prompt for `lead`, with the approved brief and `1 earlier space (search_sessions).` in `### On demand`. stdout has the reply and `space 2: resume with reef chat --space 2`, `exit: 0`, `orphans_alive: none`. `$H sql "select id, role, backend, status, session_id from turn where space_id = 2"`: one `lead|codex|done|<thread id>` row. `$H sql "select kind, author, body, why from memory where author = 'lead'"`: `decision|lead|Unlock locked accounts by email reset|No support staff`. Tool calls: `grep -o 'tools\.mcp__reef__[a-z_]*' ~/.codex/sessions/*/*/*/rollout-*-<thread id>.jsonl` lists `tools.mcp__reef__get_brief` and `tools.mcp__reef__remember` (codex calls MCP tools from its `exec` tool).
- **Codex resume and wrap approve.** `$H reef --stdin ~/.reef-verify/<id>/codex-resume-in.txt --answer y --timeout 300 -- chat --space 2`. stdout has `ok-codex-2`, `proposal 4 from lead`, `approved: brief and N items saved`; result `feed: sent "y" after approve? [y/N]`. The new turns have the same session id; `$H sql "select id, role, status from proposal where id = 4"`: `4|lead|approved`.
- **Codex interrupt.** `$H reef --stdin ~/.reef-verify/<id>/long-in.txt --timeout 12 -- chat --space 2`. Result `exit: 0 ... sigint_after_s: 12 ... orphans_alive: none` and the resume line. `$H sql "select id, backend, status, error from turn where space_id = 2 order by id desc limit 1"`: `codex|failed|interrupted`.

Permission mode and quick session (claude, `chat-extra` folder, project 1, work item 2 `Email login`):

- **Permission mode kept on resume.** `ANTHROPIC_MODEL=sonnet $H reef --stdin <two lines> --timeout 300 -- chat 2 --backend claude --permission-mode acceptEdits`. `$H sql "select id, permission_mode from space"` shows `acceptEdits`. `ANTHROPIC_MODEL=sonnet $H reef --stdin <a line asking what was said first> --timeout 300 -- chat --space ID --debug` replies with the earlier message, and `ps` of the live `claude` (or its `--permission-mode` in `reef.log` children) shows `acceptEdits`. `$H reef -- chat --space ID --permission-mode default` prints the usage, `exit: 1`.
- **Paused.** Write `Say only: x` to `~/.reef-verify/<id>/paused-in.txt` and `/continue` then `Say only: back` to `continue-in.txt`. On that Space: `$H sql --write "update space set paused='test pause' where id=ID"`, then `$H reef --stdin ~/.reef-verify/<id>/paused-in.txt -- chat --space ID` prints `space ID is paused: test pause` and `/continue to go on`, `exit: 0`, `children: 0`; the Space's `turn` and `message` counts do not change. `$H sql --write "update space set paused='', token_budget=1 where id=ID"`, the same command prints `paused: token budget of 1 used: N tokens`, and `$H sql "select paused from space where id=ID"` now holds that reason. `ANTHROPIC_MODEL=sonnet $H reef --stdin ~/.reef-verify/<id>/continue-in.txt -- chat --space ID` prints `continued` then the reply `back`; `$H sql "select paused, token_budget from space where id=ID"` shows an empty pause and a budget of the tokens used plus 1000000.
- **Quick session.** `$H sql --write "insert into space (project_id, name, role, backend, team_id, created_at) values (1, 'quick', 'lead', 'claude', 1, '2026-10-13T00:00:00Z')"` (`team_id` 1 is `solo`; REST `POST /spaces` works too). `ANTHROPIC_MODEL=sonnet $H reef --stdin <one line> --timeout 300 -- chat --space ID --debug`: stderr starts with `## You` and `You are the lead in project "<name>" (<folder>).` and `This is a quick session: it has no work item yet`, with no work item name, brief or index.

After the chats:

- **Tokens.** `$H sql "select count(*) from token"` is `0` after every chat above (in the `chat` folder; `reef token` in `chat-extra` keeps its own row).
- **Backend typo.** `$H reef -- chat 1 --backend nope` prints `reef: invalid: unknown backend "nope"`, `exit: 1`. `$H sql "select count(*) from space"` is still 2.
- **Index line.** Record a decision whose first line is longer than 100 runes, with a second line (a double-quoted argument with a real newline): `$H reef -- decide 1 "Ünïcode-first line: <80 × é>TAIL` newline `second line must not show"`. `$H sql "select max(id) from memory"` gives its id `N`; the model decides how many items came before it. `$H reef -- context 1 --role developer` shows `decision N: Ünïcode-first line: <80 × é>… (user, <date>)`: cut at 100 runes, no `TAIL`, no second line. `### On demand` shows `2 earlier spaces (search_sessions).`, the two Spaces with messages `$H sql "select space_id, count(*) from message group by space_id"` lists.

Claude, a second folder (`chat-extra`). Setup: `$H reef --data-name chat-extra -- project add`, `$H reef -- work-item add placeholder`, `$H reef -- work-item add "Email login"` (work item 2): the `chat` folder's worktree on this scratch cwd already holds branch `work-item/1`, and one branch fits one worktree, so this folder's chats use item 2 (README, Baseline preconditions). Input files in `~/.reef-verify/<id>/`: `dedupe-in.txt`, two lines: `Use the reef remember tool now, once: kind decision, body 'Sessions expire after 30 minutes of inactivity', why 'Limit stolen-session risk'. Also: when I later type /wrap, include that exact same decision again in the propose_wrap items list, even though it is already saved. Reply in one short sentence.`, `/wrap`.

- **Name, `yes` and a decision proposed twice.** `ANTHROPIC_MODEL=sonnet $H reef --stdin ~/.reef-verify/<id>/dedupe-in.txt --answer yes --timeout 300 -- chat 2 --backend claude --name "Design room"`. stdout has `proposal 1 from lead` with `- Sessions expire after 30 minutes of inactivity` under `items:`, and `approved: brief and 0 items saved` (it was saved live with `remember`); result `feed: sent "yes" after approve? [y/N]`, `exit: 0`. `$H sql "select id, name from space"`: `1|Design room`. `$H sql "select author, body from memory"`: one row, `lead|Sessions expire after 30 minutes of inactivity`. `$H sql "select id, status from proposal"`: `1|approved`.
- **Ctrl-C at the prompt.** `ANTHROPIC_MODEL=sonnet $H reef --stdin ~/.reef-verify/<id>/wrap-eof.txt --sigint-at-prompt --timeout 300 -- chat --space 1`. stdout has `proposal 2 from lead`, `approve? [y/N]`, then `proposal 2 left pending`, then `space 1: resume with reef chat --space 1`. Result `feed: sent SIGINT after approve? [y/N]`, `exit: 0`, `orphans_alive: none`, and no `sigint_after_s` (the timeout did not fire). `$H sql "select id, status, decided_at is null from proposal where id = 2"`: `2|pending|1`.
- **Background task ends mid-turn.** `~/.reef-verify/<id>/bg-chat.txt` has three lines: `Run this with the Bash tool with run_in_background true: sleep 6; echo bgdone. Then reply with just: started.`, `Write about 250 words on how tides work. Do not use any tools.`, `What is 7 times 6? Reply with just the number.`. Run `ANTHROPIC_MODEL=sonnet $H reef --stdin ~/.reef-verify/<id>/bg-chat.txt --timeout 300 -- chat 2 --backend claude`. `exit: 0`, `orphans_alive: none`. `$H sql "select author, substr(replace(body, char(10), ' '), 1, 50) from message where space_id = (select max(id) from space)"`: three `user` rows and three `lead` rows, `started.`, the essay, and exactly `42`: the reply to the third line has no text about the background command. The task must end while the second turn runs. Proof: `follow-ups` drives the same three turns with `reef-agent` and shows `session=4`; or tee claude's output as in the cap recipe below and look for `task_notification` between the second turn's `init` and `result`. If it did not, repeat once with another `sleep` value. Without the fix the last reply is `The background sleep command finished ... Nothing else is pending.42`.
- **Submit from a chat.** A work item with two acceptance lines, `in_progress` (`$H sql --write "update work_item set status = 'in_progress' where id = 1"`), and `$H reef --stdin <file with SUBMIT FOR REVIEW> --timeout 60 -- chat 1 --backend claude --debug`. The `--debug` prompt has `Acceptance lines:`, `1. [ ] ...` and `2. [ ] ...` after `Description:`. The scripted lead replays `submit-lead`: stdout has `Submitted for review.`; `$H sql "select status from work_item where id = 1"` is `review` and `proof` has one row for Space 1. The Space's role is `lead` and its token's role is `lead`, so the submit is the lead's.
- **Errors.** `$H reef -- chat --space 99` prints `reef: space 99: not found`. `$H reef -- chat 2` (no backend) and `$H reef -- chat --space 1 --name x` print the usage, `reef: usage: reef chat <work-item> --backend claude|codex [--role lead] [--name N]` and two more lines. `$H reef -- chat 2 --role developer --backend claude` prints `reef: invalid: reef chat runs the lead of team solo, lead: --role developer is not it`. Each `exit: 1`, `children: 0`. `$H sql "select count(*) from space"` is still 1.
- **No session.** `$H sql --write "insert into space (project_id, work_item_id, name, team_id, created_at) values (1, 1, 'nobackend', 1, '2026-10-13T00:00:00Z')"` makes Space 2, which has no turn and no backend (a Space from `reef token` has backend `claude`). `$H reef -- chat --space 2` prints `reef: invalid: unknown backend ""`, `exit: 1`.
- **Chat lock and serve skip.** Make a stale turn on Space 2, as if its chat had died: `$H sql --write "insert into turn (space_id, role, backend, status, started_at) values (2, 'tester', 'claude', 'running', '2026-10-13T00:00:00Z')"` (turn 4). Start a chat on Space 1 in the background (Bash `run_in_background`): `ANTHROPIC_MODEL=sonnet $H reef --stdin ~/.reef-verify/<id>/long-in.txt --timeout 20 -- chat --space 1`. Poll `$H sql "select id, space_id, status, session_id != '' from turn where status = 'running'"` until it shows `5|1|running|1` beside `4|2|running|0`. Then, while it runs:
  - Lock: run the binary directly, since the helper runs one `reef` at a time: `cd <scratch>/cwd && <scratch>/bin/reef chat --space 1 --data <scratch>/chat-extra </dev/null; echo "exit: $?"` prints `reef: space 1 is open elsewhere: conflict`, `exit: 1`. `$H sql "select status from turn where id = 5"` is still `running`.
  - Serve skip: `$H serve-start --data-name chat-extra`. `serve.log` has `marked 1 running turns failed: server restarted`. `$H api GET '/turns?status=running'` lists only turn 5. `$H api GET '/messages?space=2'` is one `reef` message, `Turn 4 by tester failed: server restarted. ...`; `$H api GET '/messages?space=1'` has no `reef` message. `$H serve-stop`.
  - When the background chat ends: its result has `sigint_after_s: 20`, `exit: 0`, `orphans_alive: none` and the resume line. `$H sql "select id, status, error from turn where id = 5"`: `5|failed|interrupted`.

## Gotchas

- The model may save the decision live with `remember` in the first turn. Then the wrap-up proposes it again or not at all and prints one item fewer in `approved: brief and N items saved`. Both are correct; the `memory` row is the proof either way.
- If the model does not call `propose_wrap`, stdout says `no proposal made` and the result says `feed: no proposal made, answer not sent`. Re-run with a new data folder rather than reading on.
- The stored user message is `/wrap`; only the agent gets the wrap-up prompt.
- The `--debug` prompt holding `You are the lead on work item` right after `## You` holds only while the `solo` lead has no persona or skills.
- An interrupted turn's partial reply is not stored, so it adds one message (the user line) to the Space's count.
- Claude stores sessions per cwd, and the chat runs in the project folder (the scratch cwd). Resume only works inside the same run. Codex threads are global, in `~/.codex/sessions`; cleanup leaves them.
- Never run two `$H reef` at once in one run: they share the run's output files. `$H sql`, `$H api` and `$H serve-*` can run beside one background `$H reef`; a second chat for the lock check runs the binary directly.
- The model may skip the repeated decision in its proposal. Then `decisions:` does not list it and the step proves nothing about the skip; re-run it on a new data folder.
