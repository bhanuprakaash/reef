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
- `chat-wrap-approve` `/wrap` then `y`: the agent calls `propose_wrap`, the proposal prints, and approve adds a brief version, the role's notes and the proposed decisions, and marks the proposal `approved`.
- `chat-resume` `reef chat --channel ID` resumes the same session id with the same role and backend.
- `chat-wrap-reject` `/wrap` then `n`: the proposal is `rejected` and no brief, notes or decisions change.
- `chat-interrupt` Ctrl-C during a turn stores it `failed` with `interrupted`, prints the resume line, exits 0 and leaves no processes.

## How to get to it (user POV)

- In the project folder: `reef chat login --role developer --backend claude`, type lines, `/wrap`, `y`, Ctrl-D.
- Later: `reef chat --channel 1`.

## Driving it with verify-reef

`$H reef --stdin FILE --timeout S -- chat ...` runs `reef chat` in the scratch cwd with
FILE as stdin. At the end of FILE the chat exits by itself. `--timeout S` (default 240)
sends SIGINT after S seconds, which is Ctrl-C. The result line has `children:` (the agent
and its `reef mcp`) and `orphans_alive:`.

Preconditions:

- Baseline from the README. No server needed. Every chat drive sets `ANTHROPIC_MODEL=sonnet`.
- Setup: `$H reef --data-name chat -- project add` (project 1 is the scratch cwd, so the agent runs there), `$H reef -- feature add login "Email login" --goal "Users sign in with email and password"`.
- Input files, one line each:
  - `~/.reef-verify/<id>/chat-in.txt`: `We decided to lock an account after 5 failed logins, to slow down guessing. Reply in one short sentence.`, `/wrap`, `y`.
  - `~/.reef-verify/<id>/resume-in.txt`: `What did we decide about failed logins? One sentence.`, `/wrap`, `n`.
  - `~/.reef-verify/<id>/long-in.txt`: `Write a 3000-word essay on the history of lighthouses. Do not use any tools.`

- **Turn, context and wrap approve.** `ANTHROPIC_MODEL=sonnet $H reef --stdin ~/.reef-verify/<id>/chat-in.txt --timeout 300 -- chat login --role developer --backend claude --debug`. stderr starts with `--- prompt ---` and `You are the developer on feature "Email login" (login) in project "cwd".`. stdout has the reply, `proposal 1 from developer`, `brief:`, `notes:`, `decisions:`, `approve? [y/N] approved: brief, notes and N decisions saved`, and `channel 1: resume with reef chat --channel 1`. Result `exit: 0`, `orphans_alive: none`. Then:
  - `$H sql "select id, author, substr(body, 1, 50) from message"`: `user`, `developer`, `user` (`Wrap up this channel. Call the propose_wrap tool ...`), `developer`.
  - `$H sql "select id, role, backend, status, session_id != '' as has_session, input_tokens + cache_read_tokens > 0 as has_usage from turn"`: two rows `developer|claude|done|1|1`.
  - `$H sql "select id, role, status, decided_at is not null from proposal"`: `1|developer|approved|1`.
  - `$H sql "select author, substr(body, 1, 40) from brief"`, `$H sql "select role, substr(body, 1, 40) from role_notes"`, `$H sql "select id, author, body from decision"`: a brief and notes by `developer`, and the 5-failed-logins decision by `developer`, either recorded live with `record_decision` or proposed at wrap-up (then `N` above is 1).
- **Resume and reject.** `ANTHROPIC_MODEL=sonnet $H reef --stdin ~/.reef-verify/<id>/resume-in.txt --timeout 300 -- chat --channel 1`. The reply names the 5 failed logins (the session remembers). stdout has `proposal 2 from developer` and `rejected: nothing changed`. `$H sql "select id, session_id, status from turn"`: four `done` turns, one session id. `$H sql "select id, status from proposal"`: `1|approved`, `2|rejected`. `$H sql "select count(*) from brief"` is still 1.
- **Interrupt.** `ANTHROPIC_MODEL=sonnet $H reef --stdin ~/.reef-verify/<id>/long-in.txt --timeout 12 -- chat --channel 1`. Result `exit: 0 ... sigint_after_s: 12 ... orphans_alive: none`, stdout ends with `channel 1: resume with reef chat --channel 1`. `$H sql "select id, status, error, ended_at is not null from turn where id = 5"`: `5|failed|interrupted|1`.

## Gotchas

- The model may record the decision live with `record_decision` in the first turn. Then the wrap-up proposes no decisions and prints `0 decisions saved`. Both are correct; the `decision` row is the proof either way.
- If the model does not call `propose_wrap`, stdout says `no proposal made` and the next line (`y`) is sent as a normal message. Re-run with a new data folder rather than reading on.
- The wrap-up prompt is stored as the user message, not the text `/wrap`.
- Claude stores sessions per cwd, and the chat runs in the project folder (the scratch cwd). Resume only works inside the same run.
- The helper cannot hold two chats at once, so the chat lock (`channel 1 is open in another reef chat`) is not driven here; `go test ./cmd/reef` covers serve recovery skipping an open chat.
- The drives above use claude. A `--backend codex` chat is not driven here; report it as not run if a line needs it.
