# Live channels

A client of `reef serve` runs a channel's agent over REST: it sends a message, follows the
agent's events on a Server-Sent Events stream, stops the agent, wraps up, promotes a quick
session and decides the proposal. The window uses these routes; this file drives them with
`curl` so a failure points at the server, not the UI. Contract: `docs/server.md` (Channels
live, Wrap-up and promotion, Caps).

## Sub-features

- `live-send` `POST /channels/{id}/send {"text":...}` answers 202 at once and the user message is stored before the answer; a send while a turn runs is 409; an empty text is 400.
- `live-events` `GET /channels/{id}/events` starts with `turn {"status":"idle"}` between turns, or the running chain's frames so far; then `turn` running (with the running teammate's `role`, here `lead`), `agent` events, `turn` done or failed, and `pause` frames. Wakes between teammates, approvals and the hop cap are in `teams.md`.
- `live-stop` `POST /channels/{id}/stop` during a turn stores it `failed`, `interrupted`, stops the agent and frees the chat lock; the next send resumes the session.
- `live-text-commands` the text `/wrap` runs a wrap-up turn; `/promote` sent as text is a plain message.
- `live-wrap` `POST /channels/{id}/wrap` on a feature channel ends with a pending proposal (`GET /channels/{id}/proposal`); on a quick session it is 400 before any agent starts.
- `live-promote` `POST /channels/{id}/promote {"slug","name"}` (or `{"featureId":N}` for an existing feature) on a quick session sets the channel's feature and ends with a pending proposal holding the first brief; both forms together, or neither, is 400; a taken slug is 409 and starts no agent; a channel with a feature is 400.
- `live-decide` `POST /proposals/{id}/approve` adds a brief version and returns `decisionsAdded`; `reject` changes nothing; deciding twice is 400.
- `live-caps` a channel paused by the token budget or the tool cap refuses a send with 409 and the reason; a hop-cap pause (`hop cap: 6 agent hops without a user message`) does not: a user send clears it, answers 202 and runs the turn. `POST /channels/{id}/continue` clears a pause (409 when not paused or while a turn runs), also runs held wakes (see `teams.md`), and sets a used-up token budget to the tokens used plus 1000000.
- `live-delete` `DELETE /channels/{id}` on a live channel stops its agent first.

## How to get to it (user POV)

- `curl` against `reef serve`: `POST /channels` with `backend`, then `/channels/{id}/send`, `/events`, `/stop`, `/continue`, `/wrap`, `/promote`, `/proposal`, `/proposals/{id}/approve|reject`.
- A body-less POST still needs `Content-Type: application/json` (send `{}`); `$H api` adds the header.

## Driving it with verify-reef

Preconditions: baseline from the README, and a server started with the model set:
`ANTHROPIC_MODEL=sonnet REEF_VERIFY_RUN=<id> $H serve-start --data-name live`. `A=$(cat <scratch>/serve.addr)` for `curl -N`.

- **Setup.** `$H api POST /projects '{"name":"live","path":"<scratch>/cwd"}'`, `$H api POST /features '{"projectId":1,"slug":"login","name":"Login"}'`, `$H api POST /channels '{"projectId":1,"featureId":1,"name":"build","backend":"claude","permissionMode":"acceptEdits"}'` (channel 1, `"role":"lead"`), `$H api POST /channels '{"projectId":1,"name":"idea","backend":"claude"}'` (channel 2, quick session).
- **Events between turns.** `curl -sN --max-time 2 http://$A/channels/1/events` prints `event: turn` / `data: {"status":"idle"}` first.
- **Send.** Start `curl -sN --max-time 120 http://$A/channels/1/events > ~/.reef-verify/<id>/live-events.txt &`, then `$H api POST /channels/1/send '{"text":"Remember the word pineapple. List the files here with a tool, then reply in one sentence."}'` gives `HTTP 202`; at once `$H api GET '/messages?channel=1'` already has the user message. A second send before the turn ends gives 409 `has a turn running`. When `$H sql "select status from turn where channel_id=1"` is `done`, the events file has `turn` running, `agent` lines with `"kind":"tool_call"` and `"kind":"text"`, and `turn` done. `$H api POST /channels/1/send '{"text":""}'` gives 400.
- **Stop and resume.** `$H api POST /channels/1/send '{"text":"Write a 1500-word story about lighthouses. Use no tools."}'`, wait 5 seconds, `$H api POST /channels/1/stop` gives 204; the turn row is `failed|interrupted`. `$H api POST /channels/1/send '{"text":"What word did I ask you to remember? Answer with the word only."}'`, then the newest `lead` message contains `pineapple`.
- **Text commands.** `$H api POST /channels/1/send '{"text":"/promote"}'` gives 202; after the turn the user message is `/promote`, `$H api GET /channels/1/proposal` is 404 and `$H api GET /features/1/brief/versions` is `[]`.
- **Wrap.** `$H api POST /channels/2/wrap` gives 400 `quick session`. `$H api POST /channels/1/wrap` gives 202; after the turn `$H api GET /channels/1/proposal` is 200 `pending`. `$H api POST /proposals/1/reject` gives 200 `rejected` and brief versions stay `[]`; a second reject gives 400 `already rejected`.
- **Promote.** `$H api POST /channels/2/send '{"text":"I want a search bar that filters products as you type. Reply in one sentence."}'` and wait for `done`. `$H api POST /channels/2/promote '{"slug":"login","name":"Again"}'` gives 409 `is taken` and `pgrep -f "reef-verify/<id>"` shows no new agent. `$H api POST /channels/2/promote '{"slug":"search","name":"Search"}'` gives 202 with `"featureId":2` and posts a `reef` message `This feature works in <data>/worktrees/1/search on feature/search.` (the worktree itself is made at the channel's next wake); after the turn `GET /channels/2/proposal` is `pending`; `$H api POST /proposals/2/approve` gives 200 with `decisionsAdded`, and `GET /features/2/brief/versions` has one version by `lead`. A second promote on channel 2 gives 400 `already has feature`.
- **Caps.** `$H sql --write "update channel set token_budget=1 where id=1"`, then a send gives 409 `paused: token budget of 1 used: N tokens` and the row is paused. `$H api POST /channels/1/continue` gives 204, then `$H sql "select token_budget, paused from channel where id=1"` is `<N + 1000000>|` (N from the 409 message); a second continue gives 409 `is not paused`.
- **Delete live.** `$H api POST /channels/1/send '{"text":"Reply ok."}'`, wait for `done` (the agent stays live), note `pgrep -f "reef-verify/<id>/bin/reef mcp"`, then `$H api DELETE /channels/1` gives 204 and that `reef mcp` process and its agent are gone.
- `$H serve-stop`.

## Gotchas

- `send` answers before the turn runs; poll `$H sql "select status from turn ..."` (or the events stream) before checking the reply.
- The events stream sends a `: keepalive` comment every 15 seconds; `curl --max-time` ends it.
- A turn that fails for any reason other than Stop also stops the agent; the next send starts it again by resume.
- `/promote` as text is a message on purpose: only the promote route promotes.
- A paused channel's 409 text ends with `: conflict`; match on `paused:` and the reason.
- Agents start lazily, on a channel's first turn, so a "starts no agent" `pgrep` check on a channel that has had no turn is true whatever the server does. Count agents before and after the call, on a channel that already ran.
