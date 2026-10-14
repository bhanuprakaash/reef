# Turns and restart recovery

A turn is one agent run in a channel. It starts `running` and ends once, as `done` or
`failed`, with its token usage. A turn can only run while a server owns it. A normal stop
(Ctrl-C) ends a live turn as `failed` with `interrupted`. When the server dies instead, or a
turn has no live agent, the turns it left running are marked failed at the next start, and each
affected channel gets one message from `reef` telling the user to check the worktree.

## Sub-features

- `turn-create` a new turn is `running` with `endedAt` null and zero usage; a bad backend, an unknown channel or a missing role is 400.
- `turn-end` `PATCH` to `done` or `failed` sets `endedAt`, `usage`, `error` and `sessionId`; a second `PATCH` is 400; `running` or another status is 400; a negative usage count is 400.
- `turn-session` `PATCH` with `sessionId` and no `status` on a running turn sets only the id (200, still `running`); on an ended turn it is 400.
- `turn-filter` `GET /turns` filters by `?channel=ID` and `?status=running|done|failed`.
- `turn-recovery-crash` after a crash (SIGKILL), the next start marks every running turn `failed` with `server restarted`, and adds one `reef` message per affected channel naming all its turns.
- `turn-recovery-clean` with no running turn, a restart marks nothing and adds no message; a second server refused by the lock never touches the owner's turns.
- `turn-recovery-stop` a turn row with no live agent (made by `POST /turns`, or a dead chat) left running over a normal stop (SIGINT) is failed the same way at the next start; a real server-run turn stopped by SIGINT ends `failed` with `interrupted` at shutdown, and the next start logs no `marked` line and posts no `reef` message.
- `turn-recovery-open-chat` a running turn of a live `reef chat` is left running, gets no `reef` message and is not counted; stale turns beside it are failed.

## How to get to it (user POV)

- `curl` against `reef serve`: `/turns`, `/turns/{id}`, `/messages?channel=ID`.
- Stop and start `reef serve` (Ctrl-C, or the process dying) with a turn running.

## Driving it with verify-reef

Preconditions:

- Baseline from the README. Fresh server: `$H serve-stop`, then `$H serve-start --data-name turns`. Ids below assume it.
- Setup: `$H api POST /projects '{"name":"shop","path":"/tmp"}'`, `$H api POST /features '{"projectId":1,"slug":"login","name":"Login"}'`, `$H api POST /channels '{"projectId":1,"featureId":1,"name":"build","backend":"claude"}'` (channel 1), `$H api POST /channels '{"projectId":1,"name":"scratch","backend":"claude"}'` (channel 2).

- **Create.** `$H api POST /turns '{"channelId":1,"role":"developer","backend":"claude"}'` gives 201 id 1, `"status":"running"`, `"endedAt":null`, all four usage numbers 0. `$H api POST /turns '{"channelId":1,"role":"developer","backend":"gpt"}'`, `$H api POST /turns '{"channelId":99,"role":"developer","backend":"claude"}'` and `$H api POST /turns '{"channelId":1,"backend":"claude"}'` give 400.
- **End once.** `$H api PATCH /turns/1 '{"status":"done","sessionId":"3c1296cc","usage":{"inputTokens":4,"outputTokens":123,"cacheReadTokens":22898,"cacheWriteTokens":0}}'` gives 200, `"status":"done"`, `endedAt` set, that usage. `$H api PATCH /turns/1 '{"status":"failed"}'` gives 400 `invalid: turn 1 already ended`. `$H api POST /turns '{"channelId":1,"role":"developer","backend":"claude"}'` (id 2) then `$H api PATCH /turns/2 '{"status":"failed","error":"boom"}'` gives `"status":"failed"`, `"error":"boom"`. `$H api PATCH /turns/1 '{"sessionId":"x"}'` gives 400 `invalid: turn 1 already ended`.
- **Bad end.** `$H api POST /turns '{"channelId":1,"role":"tester","backend":"codex"}'` (id 3), then `$H api PATCH /turns/3 '{"status":"running"}'` and `$H api PATCH /turns/3 '{"status":"bogus"}'` give 400; `$H api PATCH /turns/3 '{"status":"done","usage":{"inputTokens":-5}}'` gives 400 `invalid: usage counts cannot be negative`; `$H api PATCH /turns/3 '{"error":"x"}'` gives 400 `invalid: error and usage need a status`; `$H api PATCH /turns/99 '{"status":"done"}'` gives 404. Turn 3 stays running.
- **Filters.** `$H api GET '/turns?status=running'` lists only 3; `$H api GET '/turns?channel=1'` lists 1, 2, 3; `$H api GET '/turns?channel=2'` is `[]`; `$H api GET '/turns?status=bogus'` and `$H api GET '/turns?channel=x'` give 400.
- **Second server does not recover.** `$H serve-second --same-addr` prints `ok: refused`. `$H api GET /turns/3` is still `running` and `$H api GET '/messages?channel=1'` is `[]`.
- **Crash recovery.** Add two more running turns: `$H api POST /turns '{"channelId":1,"role":"developer","backend":"claude"}'` (id 4) and `$H api POST /turns '{"channelId":2,"role":"developer","backend":"codex"}'` (id 5). `$H serve-stop --kill` prints `exit=137`. `$H serve-start --data-name turns`. `serve.log` has `marked 3 running turns failed: server restarted`. `$H api GET '/turns?status=running'` is `[]`; `$H api GET /turns/3` is `failed`, `"error":"server restarted"`, `endedAt` set. `$H api GET '/messages?channel=1'` is exactly one message, author `reef`, `Turn 3 by tester, turn 4 by developer failed: server restarted. The worktree may be dirty; retry or roll back.`; `$H api GET '/messages?channel=2'` is one message, `Turn 5 by developer failed: server restarted. ...`. Turn 1 is still `done` with its usage and turn 2 still `failed` with `boom`: `$H api GET /turns/1`, `$H api GET /turns/2`. Before the kill, `$H api PATCH /turns/4 '{"sessionId":"abc"}'` gives 200 `running`; after the restart `$H api GET /turns/4` is `failed` with `"sessionId":"abc"`.
- **Clean restart.** `$H serve-stop` prints `exit=0`. `$H serve-start --data-name turns`. `serve.log` has no new `marked` line after this start. `$H sql "select count(*) from message where author = 'reef'"` is still 2 and `$H sql "select id, status from turn"` is unchanged.
- **Stop with a turn running.** `$H api POST /turns '{"channelId":2,"role":"tester","backend":"claude"}'` (id 6), `$H serve-stop`, `$H serve-start --data-name turns`. `serve.log` has `marked 1 running turns failed: server restarted`. `$H api GET /turns/6` is `failed`; `$H api GET '/messages?channel=2'` now has a second `reef` message, `Turn 6 by tester failed: server restarted. ...`. A turn the server really runs is not this case. To see it, add a project on `<scratch>/cwd` (not `/tmp`: a live agent there leaves claude state that `$H cleanup` does not remove) and a channel on it, start the server with `ANTHROPIC_MODEL=sonnet`, `send` a long prompt, then `serve-stop` mid-turn. It ends `failed` with `"error":"interrupted"` at shutdown, so the next start has no `marked` line and adds no `reef` message.
- **Open chat is skipped.** Needs a live `reef chat`, so it uses its own folder. `$H serve-stop`. `$H reef --data-name turns-chat -- project add`, make `<scratch>/cwd` a git repo with one commit (the feature's chat runs in a worktree; README, Baseline preconditions), `$H reef -- feature add login "Email login"`, `$H reef -- token login --role tester` (channel 1). Make a stale turn on channel 1, as if its chat had died: `$H sql --write "insert into turn (channel_id, role, backend, status, started_at) values (1, 'tester', 'claude', 'running', '2026-10-13T00:00:00Z')"` (turn 1). Write `Write a 3000-word essay on the history of lighthouses. Do not use any tools.` to `~/.reef-verify/<id>/long-in.txt`. Start a chat in the background (Bash `run_in_background`): `ANTHROPIC_MODEL=sonnet $H reef --stdin ~/.reef-verify/<id>/long-in.txt --timeout 20 -- chat login --role developer --backend claude` (channel 2, turn 2). Poll `$H sql "select id, channel_id, status, session_id != '' from turn where status = 'running'"` until it shows `2|2|running|1` beside `1|1|running|0`. Then `$H serve-start --data-name turns-chat`: `serve.log` has `marked 1 running turns failed: server restarted`. `$H api GET '/turns?status=running'` lists only turn 2; `$H api GET '/messages?channel=1'` is one `reef` message, `Turn 1 by tester failed: server restarted. ...`; `$H api GET '/messages?channel=2'` has no `reef` message. When the chat ends (`sigint_after_s: 20`, `exit: 0`, `orphans_alive: none`), `$H api GET /turns/2` is `failed`, `"error":"interrupted"`. `$H serve-stop`.

## Gotchas

- Recovery happens at start, after the lock and the bind and before the first request, so a `GET` right after `serve-start` already shows `failed`.
- Only SIGKILL is a crash. SIGINT stops the server cleanly, and a real turn still running then ends `failed` with `interrupted` at shutdown (its agent stopped with the server). Only a row with no live agent stays `running` over the stop and gets `server restarted` at the next start.
- Turns in the message are listed in id order. `serve.log` counts turns; the message is one per channel.
- A turn's `usage` has all four numbers whatever you send; a field you leave out is 0.
- A live `reef chat` holds `chat-<channel id>.lock` in the data folder; recovery skips that channel. The chat must still be in its turn when `serve-start` runs, so start the server as soon as the poll shows the session id.
