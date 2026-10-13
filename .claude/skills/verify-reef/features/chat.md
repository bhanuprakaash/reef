# reef chat

A user runs one role on a feature in the terminal with `reef chat`. The first message
carries the context prompt. Each line is a stored message and a stored turn, and the
agent's reply is stored as a message. `/wrap` makes the agent propose a new brief, notes
and decisions through `reef mcp`; the user approves (all saved in one transaction) or
rejects (nothing saved). `reef chat --channel ID` resumes the session. Ctrl-C stops the
turn. Contract: `docs/memory.md`.

## Sub-features

- `chat-turn` one piped line becomes a `user` message, a `done` turn with a session id and usage, and a reply message from the role.
- `chat-context` the first prompt starts with the context prompt (`--debug` shows it on stderr).
- `chat-tools` the agent calls the `reef` tools (`get_brief`, `record_decision`, `propose_wrap`) from inside the chat.
- `chat-wrap-approve` `/wrap` then `y`: the agent calls `propose_wrap`, the proposal prints, and approve adds a brief version, the role's notes and the proposed decisions, and marks the proposal `approved`.
- `chat-wrap-typed-ahead` a line typed while the agent wraps up prints `ignored while wrapping: <line>` and is not the answer.
- `chat-resume` `reef chat --channel ID` resumes the same session id with the same role and backend.
- `chat-wrap-reject` `/wrap` then `n`: the proposal is `rejected` and no brief, notes or decisions change.
- `chat-wrap-eof` stdin ends at `approve? [y/N]`: the proposal stays `pending` with `proposal N left pending`.
- `chat-wrap-cli` `reef wrap list`, `show`, `approve` and `reject` manage pending proposals outside chat; deciding one twice fails.
- `chat-wrap-sigint` Ctrl-C at `approve? [y/N]`: the proposal stays `pending`, it prints `proposal N left pending` and the resume line, and exits 0.
- `chat-wrap-dedupe` approve (`y` or `yes`) skips a proposed decision whose body the feature already has, so a decision recorded live and proposed again is saved once.
- `chat-name` `--name N` names a new channel; the default is `<role> chat`.
- `chat-errors` a usage error, `--channel 99` and a channel with no backend fail with `reef: ...` and exit 1, before an agent starts.
- `chat-lock` a second `reef chat` on an open channel exits 1 with `channel N is open elsewhere: conflict`.
- `chat-serve-skip` `reef serve` starting while a chat runs leaves that chat's running turn alone and fails only stale turns.
- `chat-interrupt` Ctrl-C during a turn stores it `failed` with `interrupted`, prints the resume line, exits 0 and leaves no processes.
- `chat-codex` new, resume, wrap-up and Ctrl-C work the same with `--backend codex`.
- `chat-token-cleanup` every chat deletes its token when it ends.
- `chat-permission-mode` `--permission-mode acceptEdits` is stored on the channel, and `reef chat --channel ID` resumes with it; `--permission-mode` on `--channel` is refused.
- `chat-quick-session` a channel with no feature starts with a two-line prompt that names the project and says there is no feature yet.
- `chat-bad-backend` a backend typo fails before anything is stored: no channel.
- `chat-index` the context index cuts a decision's first line at 100 runes with `…` and shows each channel's message count.

## How to get to it (user POV)

- In the project folder: `reef chat login --role developer --backend claude`, type lines, `/wrap`, `y`, Ctrl-D.
- Later: `reef chat --channel 1`.

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

- Baseline from the README. No server needed. Every claude chat drive sets `ANTHROPIC_MODEL=sonnet`.
- Setup: `$H reef --data-name chat -- project add` (project 1 is the scratch cwd, so the agent runs there), `$H reef -- feature add login "Email login" --goal "Users sign in with email and password"`.
- Input files in `~/.reef-verify/<id>/`, one line each:
  - `chat-in.txt`: `Call the reef get_brief tool first. Then note: we decided to lock an account after 5 failed logins, to slow down guessing. Reply in one short sentence.`, `/wrap`, `typed-ahead line`.
  - `resume-in.txt`: `What did we decide about failed logins? One sentence.`, `/wrap`.
  - `wrap-eof.txt`: `/wrap`.
  - `long-in.txt`: `Write a 3000-word essay on the history of lighthouses. Do not use any tools.`
  - `codex-in.txt`: `Call the reef get_brief tool, then record a decision with the reef record_decision tool: body 'Unlock locked accounts by email reset', why 'No support staff'. Reply in one short sentence.`
  - `codex-resume-in.txt`: `Say only: ok-codex-2`, `/wrap`.

Claude, channel 1:

- **Turn, context, typed-ahead and wrap approve.** `ANTHROPIC_MODEL=sonnet $H reef --stdin ~/.reef-verify/<id>/chat-in.txt --answer y --timeout 300 -- chat login --role developer --backend claude --debug`. stderr starts with `--- prompt ---` and `You are the developer on feature "Email login" (login) in project "cwd".`. stdout has the reply, `proposal 1 from developer`, `brief:`, `notes:`, `decisions:`, `ignored while wrapping: typed-ahead line`, `approve? [y/N] approved: brief, notes and N decisions saved`, and `channel 1: resume with reef chat --channel 1`. Result `feed: sent "y" after approve? [y/N]`, `exit: 0`, `orphans_alive: none`. Then:
  - `$H sql "select id, author, substr(body, 1, 50) from message"`: `user`, `developer`, `user` (`/wrap`), `developer`. The typed-ahead line is not stored.
  - `$H sql "select id, role, backend, status, session_id != '' as has_session, input_tokens + cache_read_tokens > 0 as has_usage from turn"`: two rows `developer|claude|done|1|1`.
  - `$H sql "select id, role, status, decided_at is not null from proposal"`: `1|developer|approved|1`.
  - `$H sql "select count(*) from token"`: `0`, the chat deleted its token when it ended.
  - `$H sql "select author, substr(body, 1, 40) from brief"`, `$H sql "select role, substr(body, 1, 40) from role_notes"`, `$H sql "select id, author, body from decision"`: a brief and notes by `developer`, and the 5-failed-logins decision by `developer`, either recorded live with `record_decision` or proposed at wrap-up (then `N` above is 1).
  - Tool calls: `grep -o '"type":"tool_use","id":"[^"]*","name":"mcp__reef__[a-z_]*"' ~/.claude/projects/-private-tmp-reef-verify-<id>-cwd/<session id>.jsonl` lists `mcp__reef__get_brief`, `mcp__reef__propose_wrap` and usually `mcp__reef__record_decision`. The session id is the turn's `session_id`. Grep before `cleanup`, which removes that folder.
- **Resume and reject.** `ANTHROPIC_MODEL=sonnet $H reef --stdin ~/.reef-verify/<id>/resume-in.txt --answer n --timeout 300 -- chat --channel 1`. The reply names the 5 failed logins (the session remembers). stdout has `proposal 2 from developer` and `rejected: nothing changed`; result `feed: sent "n" after approve? [y/N]`. `$H sql "select id, session_id, status from turn"`: four `done` turns, one session id. `$H sql "select id, status from proposal"`: `1|approved`, `2|rejected`. `$H sql "select count(*) from brief"` is still 1.
- **End of input at the prompt and CLI approval.** `ANTHROPIC_MODEL=sonnet $H reef --stdin ~/.reef-verify/<id>/wrap-eof.txt --timeout 300 -- chat --channel 1` (no `--answer`). stdout has `proposal 3 from developer`, `approve? [y/N]`, then `proposal 3 left pending` and the resume line, `exit: 0`. `$H sql "select id, status from proposal where id = 3"`: `3|pending`. `$H reef -- wrap list` lists proposal 3; `$H reef -- wrap show 3` prints its brief, notes and decisions; `$H reef -- wrap approve 3` approves it. `$H reef -- wrap approve 3` again prints `reef: invalid: proposal 3 is already approved`, `exit: 1`. `$H sql "select id, status from proposal where id = 3"`: `3|approved`.
- **Interrupt.** `ANTHROPIC_MODEL=sonnet $H reef --stdin ~/.reef-verify/<id>/long-in.txt --timeout 12 -- chat --channel 1`. Result `exit: 0 ... sigint_after_s: 12 ... orphans_alive: none`, stdout ends with `channel 1: resume with reef chat --channel 1`. `$H sql "select id, status, error, ended_at is not null from turn where id = 6"`: `6|failed|interrupted|1`.

Codex, channel 2 (no `ANTHROPIC_MODEL` needed):

- **New codex chat with tool calls.** `$H reef --stdin ~/.reef-verify/<id>/codex-in.txt --timeout 300 -- chat login --role reviewer --backend codex --debug`. stderr has the context prompt for `reviewer`, with the approved brief and `channel 1: developer chat, ...` in the index. stdout has the reply and `channel 2: resume with reef chat --channel 2`, `exit: 0`, `orphans_alive: none`. `$H sql "select id, role, backend, status, session_id from turn where channel_id = 2"`: one `reviewer|codex|done|<thread id>` row. `$H sql "select author, body, why from decision where author = 'reviewer'"`: `reviewer|Unlock locked accounts by email reset|No support staff`. Tool calls: `grep -o 'tools\.mcp__reef__[a-z_]*' ~/.codex/sessions/*/*/*/rollout-*-<thread id>.jsonl` lists `tools.mcp__reef__get_brief` and `tools.mcp__reef__record_decision` (codex calls MCP tools from its `exec` tool).
- **Codex resume and wrap approve.** `$H reef --stdin ~/.reef-verify/<id>/codex-resume-in.txt --answer y --timeout 300 -- chat --channel 2`. stdout has `ok-codex-2`, `proposal 4 from reviewer`, `approved: brief, notes and N decisions saved`; result `feed: sent "y" after approve? [y/N]`. The new turns have the same session id; `$H sql "select id, role, status from proposal where id = 4"`: `4|reviewer|approved`.
- **Codex interrupt.** `$H reef --stdin ~/.reef-verify/<id>/long-in.txt --timeout 12 -- chat --channel 2`. Result `exit: 0 ... sigint_after_s: 12 ... orphans_alive: none` and the resume line. `$H sql "select id, backend, status, error from turn where channel_id = 2 order by id desc limit 1"`: `codex|failed|interrupted`.

Permission mode and quick session (claude, `chat-extra` folder, project 1, feature `login`):

- **Permission mode kept on resume.** `ANTHROPIC_MODEL=sonnet $H reef --stdin <two lines> --timeout 300 -- chat login --role developer --backend claude --permission-mode acceptEdits`. `$H sql "select id, permission_mode from channel"` shows `acceptEdits`. `ANTHROPIC_MODEL=sonnet $H reef --stdin <a line asking what was said first> --timeout 300 -- chat --channel ID --debug` replies with the earlier message, and `ps` of the live `claude` (or its `--permission-mode` in `reef.log` children) shows `acceptEdits`. `$H reef -- chat --channel ID --permission-mode default` prints the usage, `exit: 1`.
- **Quick session.** `$H sql --write "insert into channel (project_id, name, role, backend, created_at) values (1, 'quick', 'lead', 'claude', '2026-10-13T00:00:00Z')"` (REST `POST /channels` works too). `ANTHROPIC_MODEL=sonnet $H reef --stdin <one line> --timeout 300 -- chat --channel ID --debug`: stderr starts with `You are the lead in project "<name>" (<folder>).` and `This is a quick session: it has no feature yet`, with no `feature` name, brief or index.

After the chats:

- **Tokens.** `$H sql "select count(*) from token"` is `0` after every chat above.
- **Backend typo.** `$H reef -- chat login --role tester --backend nope` prints `reef: invalid: unknown backend "nope"`, `exit: 1`. `$H sql "select count(*) from channel"` is still 2.
- **Index line.** Record a decision whose first line is longer than 100 runes, with a second line (a double-quoted argument with a real newline): `$H reef -- decide login "Ünïcode-first line: <80 × é>TAIL` newline `second line must not show"`. `$H sql "select max(id) from decision"` gives its id `N`; the model decides how many decisions came before it. `$H reef -- context login --role developer` shows `decision N: Ünïcode-first line: <80 × é>… (user, <date>)`: cut at 100 runes, no `TAIL`, no second line. `### Channels` shows `channel 1: developer chat, <date>, 11 messages` and `channel 2: reviewer chat, <date>, 7 messages`, the counts `$H sql "select channel_id, count(*) from message group by channel_id"` gives.

Claude, a second folder (`chat-extra`). Setup: `$H reef --data-name chat-extra -- project add`, `$H reef -- feature add login "Email login"`. Input files in `~/.reef-verify/<id>/`: `dedupe-in.txt`, two lines: `Use the reef record_decision tool now, once: body 'Sessions expire after 30 minutes of inactivity', why 'Limit stolen-session risk'. Also: when I later type /wrap, include that exact same decision again in the propose_wrap decisions list, even though it is already recorded. Reply in one short sentence.`, `/wrap`.

- **Name, `yes` and a decision proposed twice.** `ANTHROPIC_MODEL=sonnet $H reef --stdin ~/.reef-verify/<id>/dedupe-in.txt --answer yes --timeout 300 -- chat login --role developer --backend claude --name "Design room"`. stdout has `proposal 1 from developer` with `- Sessions expire after 30 minutes of inactivity` under `decisions:`, and `approved: brief, notes and 0 decisions saved`; result `feed: sent "yes" after approve? [y/N]`, `exit: 0`. `$H sql "select id, name from channel"`: `1|Design room`. `$H sql "select author, body from decision"`: one row, `developer|Sessions expire after 30 minutes of inactivity`. `$H sql "select id, status from proposal"`: `1|approved`.
- **Ctrl-C at the prompt.** `ANTHROPIC_MODEL=sonnet $H reef --stdin ~/.reef-verify/<id>/wrap-eof.txt --sigint-at-prompt --timeout 300 -- chat --channel 1`. stdout has `proposal 2 from developer`, `approve? [y/N]`, then `proposal 2 left pending`, then `channel 1: resume with reef chat --channel 1`. Result `feed: sent SIGINT after approve? [y/N]`, `exit: 0`, `orphans_alive: none`, and no `sigint_after_s` (the timeout did not fire). `$H sql "select id, status, decided_at is null from proposal where id = 2"`: `2|pending|1`.
- **Errors.** `$H reef -- chat --channel 99` prints `reef: channel 99: not found`. `$H reef -- chat login --role developer` (no backend) and `$H reef -- chat --channel 1 --name x` print the usage, `reef: usage: reef chat <feature> --role R --backend claude|codex [--name N]` and two more lines. Each `exit: 1`, `children: 0`. `$H sql "select count(*) from channel"` is still 1.
- **No session.** `$H reef -- token login --role tester` makes channel 2, which has no turn and no backend. `$H reef -- chat --channel 2` prints `reef: invalid: unknown backend ""`, `exit: 1`.
- **Chat lock and serve skip.** Make a stale turn on channel 2, as if its chat had died: `$H sql --write "insert into turn (channel_id, role, backend, status, started_at) values (2, 'tester', 'claude', 'running', '2026-10-13T00:00:00Z')"` (turn 4). Start a chat on channel 1 in the background (Bash `run_in_background`): `ANTHROPIC_MODEL=sonnet $H reef --stdin ~/.reef-verify/<id>/long-in.txt --timeout 60 -- chat --channel 1`. Poll `$H sql "select id, channel_id, status, session_id != '' from turn where status = 'running'"` until it shows `5|1|running|1` beside `4|2|running|0`. Then, while it runs:
  - Lock: run the binary directly, since the helper runs one `reef` at a time: `cd /private/tmp/reef-verify/<id>/cwd && /private/tmp/reef-verify/<id>/bin/reef chat --channel 1 --data /private/tmp/reef-verify/<id>/chat-extra </dev/null; echo "exit: $?"` prints `reef: channel 1 is open elsewhere: conflict`, `exit: 1`. `$H sql "select status from turn where id = 5"` is still `running`.
  - Serve skip: `$H serve-start --data-name chat-extra`. `serve.log` has `marked 1 running turns failed: server restarted`. `$H api GET '/turns?status=running'` lists only turn 5. `$H api GET '/messages?channel=2'` is one `reef` message, `Turn 4 by tester failed: server restarted. ...`; `$H api GET '/messages?channel=1'` has no `reef` message. `$H serve-stop`.
  - When the background chat ends: its result has `sigint_after_s: 60`, `exit: 0`, `orphans_alive: none` and the resume line. `$H sql "select id, status, error from turn where id = 5"`: `5|failed|interrupted`.

## Gotchas

- The model may record the decision live with `record_decision` in the first turn. Then the wrap-up proposes no decisions and prints `0 decisions saved`. Both are correct; the `decision` row is the proof either way.
- If the model does not call `propose_wrap`, stdout says `no proposal made` and the result says `feed: no proposal made, answer not sent`. Re-run with a new data folder rather than reading on.
- The stored user message is `/wrap`; only the agent gets the wrap-up prompt.
- An interrupted turn's partial reply is not stored, so it adds one message (the user line) to the channel's count.
- Claude stores sessions per cwd, and the chat runs in the project folder (the scratch cwd). Resume only works inside the same run. Codex threads are global, in `~/.codex/sessions`; cleanup leaves them.
- Never run two `$H reef` at once in one run: they share the run's output files. `$H sql`, `$H api` and `$H serve-*` can run beside one background `$H reef`; a second chat for the lock check runs the binary directly.
- The model may skip the repeated decision in its proposal. Then `decisions:` does not list it and the step proves nothing about the skip; re-run it on a new data folder.
