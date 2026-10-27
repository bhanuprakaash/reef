# Live Spaces

A client of `reef serve` runs a Space's agent over REST: it sends a message, follows the
agent's events on a Server-Sent Events stream, stops the agent, wraps up, attaches a free
Space to a work item and decides the proposal. The window uses these routes; this file drives them with
`curl` so a failure points at the server, not the UI. Contract: `docs/server.md` (Spaces
live, Wrap-up and attach, Caps).

## Sub-features

- `live-send` `POST /spaces/{id}/send {"text":...}` answers 202 at once and the user message is stored before the answer; a send while a message chain runs is 202 and its turn runs after the current one; a send while a wrap turn runs is 409; an empty text is 400.
- `live-events` `GET /spaces/{id}/events` starts with `turn {"status":"idle"}` between turns, or the running chain's frames so far; then `turn` running (with the running teammate's `role`, here `lead`), `agent` events, `turn` done or failed, and `pause` frames. Wakes between teammates, approvals and the hop cap are in `teams.md`.
- `live-stop` `POST /spaces/{id}/stop` during a turn stores it `failed`, `interrupted`, stops the agent and frees the chat lock; the next send resumes the session.
- `live-text-commands` the text `/wrap` runs a wrap-up turn; any other text, `/attach` included, is a plain message.
- `live-wrap` `POST /spaces/{id}/wrap` on a feature Space ends with a pending proposal (`GET /spaces/{id}/proposal`); on a quick session it is 400 before any agent starts.
- `live-attach` `POST /spaces/{id}/attach {"workItemId":N}` on a free Space sets the Space's work item and kind and posts a `reef` note with the worktree path; no agent turn runs; a Space that has a work item is 400, a work item with an active Space is 409, a turn running is 409.
- `live-decide` `POST /proposals/{id}/approve` adds a brief version and returns `itemsAdded`; `reject` changes nothing; deciding twice is 400.
- `live-caps` a Space paused by the token budget refuses a send with 409 and the reason; a hop-cap pause (`hop cap: 6 agent hops without a user message`) does not: a user send clears it, answers 202 and runs the turn. `POST /spaces/{id}/continue` clears a pause (409 when not paused or while a turn runs), also runs held wakes (see `teams.md`), and sets a used-up token budget to the tokens used plus 1000000.
- `live-delete` `DELETE /spaces/{id}` on a live Space stops its agent first.

## How to get to it (user POV)

- `curl` against `reef serve`: `POST /spaces` with `backend`, then `/spaces/{id}/send`, `/events`, `/stop`, `/continue`, `/wrap`, `/attach`, `/proposal`, `/proposals/{id}/approve|reject`.
- A body-less POST still needs `Content-Type: application/json` (send `{}`); `$H api` adds the header.

## Driving it with verify-reef

Preconditions: baseline from the README, and a server started with the model set:
`ANTHROPIC_MODEL=sonnet REEF_VERIFY_RUN=<id> $H serve-start --data-name live`. `A=$(cat <scratch>/serve.addr)` for `curl -N`.

- **Setup.** `$H api POST /projects '{"name":"live","path":"<scratch>/cwd"}'`, `$H api POST /work-items '{"projectId":1,"title":"Login"}'`, `$H api POST /spaces '{"projectId":1,"workItemId":1,"name":"build","backend":"claude","permissionMode":"acceptEdits"}'` (Space 1, `"role":"lead"`), `$H api POST /spaces '{"projectId":1,"name":"idea","backend":"claude"}'` (Space 2, quick session).
- **Events between turns.** `curl -sN --max-time 2 http://$A/spaces/1/events` prints `event: turn` / `data: {"status":"idle"}` first.
- **Send.** Start `curl -sN --max-time 120 http://$A/spaces/1/events > ~/.reef-verify/<id>/live-events.txt &`, then `$H api POST /spaces/1/send '{"text":"Remember this secret word: pineapple. List the files in this directory with a tool, then read one of them with a tool, then summarize what you saw in two sentences."}'` gives `HTTP 202`; at once `$H api GET '/messages?space=1'` already has the user message. A send before the turn ends is 202 and queues behind it (`teams.md`, Send while busy), so do not send a second one here. When `$H sql "select status from turn where space_id=1"` is `done`, the events file has `turn` running, `agent` lines with `"kind":"tool_call"` and `"kind":"text"`, and `turn` done. `$H api POST /spaces/1/send '{"text":""}'` gives 400.
- **Stop and resume.** `$H api POST /spaces/1/send '{"text":"Write a 1500-word story about lighthouses. Use no tools."}'`, wait 5 seconds, `$H api POST /spaces/1/stop` gives 204; the turn row is `failed|interrupted`. `$H api POST /spaces/1/send '{"text":"What was the secret word? Answer with the word only."}'`, then the newest `lead` message contains `pineapple`.
- **Text commands.** `$H api POST /spaces/1/send '{"text":"/attach"}'` gives 202; after the turn the user message is `/attach`, `$H api GET /spaces/1/proposal` is 404 and `$H api GET /work-items/1/brief/versions` is `[]`.
- **Wrap.** `$H api POST /spaces/2/wrap` gives 400 `quick session`. `$H api POST /spaces/1/wrap` gives 202; after the turn `$H api GET /spaces/1/proposal` is 200 `pending`. `$H api POST /proposals/1/reject` gives 200 `rejected` and brief versions stay `[]`; a second reject gives 400 `already rejected`.
- **Attach.** `$H api POST /spaces/2/send '{"text":"I want a search bar that filters products as you type. Answer in one or two sentences, use no tools."}'` and wait for `done`. `$H api POST /work-items '{"projectId":1,"title":"Search"}'` (2). `$H api POST /spaces/2/attach '{"workItemId":1}'` gives 409 `conflict: work item 1 already has an active Space`. `$H api POST /spaces/2/attach '{"workItemId":2}'` gives 200 with `"workItemId":2` and `"kind":"work_item"` and posts a `reef` message `This Space is now attached to work item "Search" (2). It works in <data>/worktrees/1/2 on work-item/2.` (the worktree itself is made at the Space's next wake); `$H api GET '/spaces?workItem=2'` has Space 2. A second attach on Space 2 gives 400 `already has work item`.
- **Caps.** `$H sql --write "update space set token_budget=1 where id=1"`, then a send gives 409 `paused: token budget of 1 used: N tokens` and the row is paused. `$H api POST /spaces/1/continue` gives 204, then `$H sql "select token_budget, paused from space where id=1"` is `<N + 1000000>|` (N from the 409 message); a second continue gives 409 `is not paused`.
- **Delete live.** `$H api POST /spaces/1/send '{"text":"Reply with one word."}'`, wait for `done` (the agent stays live), note the server's `claude` children, `pgrep -P $(cat <scratch>/serve.pid) -x claude`, and with the real `claude` its `reef mcp` too, `pgrep -f "reef-verify/<id>/bin/reef mcp"` (the fake starts `reef mcp` only for a tool call), then `$H api DELETE /spaces/1` gives 204 and that agent and its `reef mcp` are gone.
- `$H serve-stop`.

## Gotchas

- `send` answers before the turn runs; poll `$H sql "select status from turn ..."` (or the events stream) before checking the reply.
- The events stream sends a `: keepalive` comment every 15 seconds; `curl --max-time` ends it.
- A turn that fails for any reason other than Stop also stops the agent; the next send starts it again by resume.
- Every other prompt here matches a fake `claude` scenario (`fake/scenario.go`); keep the wording. On the fake, the `/attach` turn fails with `no scenario`, which the step does not check.
- A paused Space's 409 text ends with `: conflict`; match on `paused:` and the reason.
- Agents start lazily, on a Space's first turn, so a "starts no agent" `pgrep` check on a Space that has had no turn is true whatever the server does. Count agents before and after the call, on a Space that already ran.
