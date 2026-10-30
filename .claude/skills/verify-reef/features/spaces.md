# Spaces and messages

A user talks to a Bot or a Team in Spaces. A free Space belongs to no work item (a quick session); a work item Space is the one
active Space of one top-level work item, where its Team works; a conversation is a Space with one Bot
(`conversations.md`). A Space runs its agents live: the user sends, follows the events, stops, wraps up, attaches a free Space to a
work item and decides the proposal. Messages are added to a Space and never edited or removed one by one.
Contract: `docs/server.md` (Spaces, Spaces live, Wrap-up and attach, Caps).

## Sub-features

- `space-create` a free Space (`workItemId` null) and a work item Space; the work item must be of the same project. `backend` is `claude` or `codex` (it defaults to the team lead's), a request `role` is empty or `lead`, and the reply's `role` is the name of the team's lead role (`lead` in `solo`, `boss` for a team whose lead is `boss`, where `"role":"boss"` is 400 `role must be lead`), and `permissionMode` must suit the backend; the reply also has `paused` (`""`), `tokenBudget` (1000000) and `team` (`solo`); Teams are in `bots.md`.
- `space-filter` `GET /spaces` filters by `?project=ID` and `?workItem=ID`.
- `space-fixed` update changes the name only; project and work item stay as created.
- `message-append` messages are added with `POST` and read with `GET /messages?space=ID`; `?after=ID` returns only newer ones.
- `message-thread` a message has `parentId`: `null` at the top level, the root's id for a reply; a parent that is not a top-level message of the same Space is 400.
- `message-append-only` `PUT`, `PATCH` and `DELETE` on a message are 405.
- `space-cascade` deleting a Space deletes its messages; deleting a work item deletes its Spaces.
- `live-send` `POST /spaces/{id}/send {"text":...}` answers 202 at once and the user message is stored before the answer; a send while a message chain runs is 202 and its turn runs after the current one; a send while a wrap turn runs is 409; an empty text is 400.
- `live-events` `GET /spaces/{id}/events` starts with `turn {"status":"idle"}` between turns, or the running chain's frames so far; then `turn` running (with the running bot's `role`, here `lead`), `agent` events, `turn` done or failed, and `pause` frames. Wakes between bots, and the hop cap are in `bots.md` and `caps.md`.
- `live-stop` `POST /spaces/{id}/stop` during a turn stores it `failed`, `interrupted`, stops the agent and frees the chat lock; the next send resumes the session.
- `live-text-commands` the text `/wrap` runs a wrap-up turn; any other text, `/attach` included, is a plain message.
- `live-wrap` `POST /spaces/{id}/wrap` on a work item Space ends with a pending proposal (`GET /spaces/{id}/proposal`); on a free Space it is 400 before any agent starts.
- `live-attach` `POST /spaces/{id}/attach {"workItemId":N}` on a free Space sets the Space's work item and kind and posts a `reef` note with the worktree path; no agent turn runs; a Space that has a work item is 400, a work item with an active Space is 409, a turn running is 409.
- `live-decide` `POST /proposals/{id}/approve` adds a brief version and returns `itemsAdded`; `reject` changes nothing; deciding twice is 400.
- `live-caps` a Space paused by the token budget refuses a send with 409 and the reason; a hop-cap pause (`hop cap: 6 agent hops without a user message`) does not: a user send clears it, answers 202 and runs the turn. `POST /spaces/{id}/continue` clears a pause (409 when not paused or while a turn runs), also runs held wakes (see `bots.md`), and sets a used-up token budget (the work item's, shared by all its Spaces) to the tokens used plus 1000000.
- `live-delete` `DELETE /spaces/{id}` on a live Space stops its agent first.

## How to get to it (user POV)

- `curl` against `reef serve`: `/spaces`, `/spaces/{id}`, `/messages?space=ID`.
- `curl` against `reef serve`: `POST /spaces` with `backend`, then `/spaces/{id}/send`, `/events`, `/stop`, `/continue`, `/wrap`, `/attach`, `/proposal`, `/proposals/{id}/approve|reject`.
- A body-less POST still needs `Content-Type: application/json` (send `{}`); `$H api` adds the header.

## Driving it with verify-reef

Messages, on a fresh server (`$H serve-stop`, then `$H serve-start --data-name spaces`):

- Baseline from the README. Fresh server: `$H serve-stop`, then `$H serve-start --data-name spaces`. Ids below assume it.
- Setup: `$H api POST /projects '{"name":"shop","path":"/tmp"}'`, `$H api POST /projects '{"name":"blog","path":"/usr"}'`, `$H api POST /work-items '{"projectId":1,"title":"Login"}'`.

- **Create.** `$H api POST /spaces '{"projectId":1,"name":"scratch","backend":"claude"}'` gives 201 id 1, `"workItemId":null`, `"team":"solo"`. `$H api POST /spaces '{"projectId":1,"workItemId":1,"name":"build","backend":"claude"}'` gives id 2, `"workItemId":1`. `$H api POST /spaces '{"projectId":2,"workItemId":1,"name":"x","backend":"claude"}'` (work item of another project), `$H api POST /spaces '{"projectId":1,"workItemId":99,"name":"x","backend":"claude"}'` and `$H api POST /spaces '{"projectId":99,"name":"x","backend":"claude"}'` give 400. Agent rules: `$H api POST /spaces '{"projectId":1,"name":"x","backend":"gemini"}'` gives 400 `backend must be claude or codex`, `... "backend":"claude","role":"dev"}'` 400 `role must be lead`, `... "backend":"claude","team":"unknown"}'` 400 `invalid: unknown team unknown`, `... "backend":"codex","permissionMode":"acceptEdits"}'` 400 `permissionMode for codex`.
- **Filters.** `$H api GET '/spaces?project=1'` lists 1 and 2; `$H api GET '/spaces?workItem=1'` lists only 2; `$H api GET '/spaces?project=2'` is `[]`; `$H api GET '/spaces?workItem=x'` gives 400.
- **Fixed after create.** `$H api PUT /spaces/2 '{"name":"build2"}'` gives 200 with `"name":"build2"`, `"projectId":1`, `"workItemId":1`. `$H api PUT /spaces/2 '{"name":"build2","projectId":2,"workItemId":null}'` gives 400 `invalid: bad JSON: json: unknown field "projectId"` (only `name` and `permissionMode` can change). `$H api PUT /spaces/2 '{}'` gives 400; `$H api PUT /spaces/99 '{"name":"x"}'` gives 404.
- **Messages.** `$H api POST /messages '{"spaceId":2,"author":"user","body":"Start with the form"}'` gives 201 id 1; `$H api POST /messages '{"spaceId":2,"author":"architect","body":"Agreed"}'` gives id 2. `$H api GET '/messages?space=2'` lists both, oldest first. `$H api GET '/messages?space=2&after=1'` lists only id 2; `$H api GET '/messages?space=2&after=2'` is `[]`. `$H api GET '/messages?space=1'` is `[]`.
- **Message errors.** `$H api GET /messages` 400 (Space required), `$H api GET '/messages?space=99'` 404, `$H api GET '/messages?space=x'` 400, `$H api POST /messages '{"spaceId":99,"author":"a","body":"b"}'` 400, `$H api POST /messages '{"spaceId":2,"body":"b"}'` 400.
- **Thread parent.** `$H api POST /messages '{"spaceId":2,"parentId":1,"author":"architect","body":"Reply"}'` gives 201 id 3 with `"parentId":1`. A reply to a reply, `... "parentId":3 ...`, and an unknown parent, `... "parentId":99 ...`, give 400 `is not a top-level message of Space 2`. `$H api GET '/messages?space=2'` has `"parentId":null` on ids 1 and 2 and `1` on id 3.
- **Append-only.** `$H api PUT /messages/1 '{"body":"edit"}'`, `$H api PATCH /messages/1 '{"body":"edit"}'` and `$H api DELETE /messages/1` give 405 `{"error":"method not allowed"}`. `$H sql "select id, body from message"` still shows the original text.
- **Cascade.** `$H api POST /spaces '{"projectId":1,"workItemId":1,"name":"later","backend":"claude"}'` gives id 3. `$H api DELETE /spaces/2` gives 204; `$H api GET '/messages?space=2'` gives 404; `$H sql "select count(*) from message"` is 0. `$H api DELETE /work-items/1` gives 204 and `$H api GET /spaces/3` gives 404, while `$H api GET /spaces/1` (the quick session) still gives 200. `$H api POST /spaces '{"projectId":1,"name":"again","backend":"claude"}'` gives id 4, not 2.

Live Spaces, on a server of its own (`$H serve-stop`, then `--data-name live` as below):

Preconditions: baseline from the README, and a server started with the model set:
`ANTHROPIC_MODEL=sonnet REEF_VERIFY_RUN=<id> $H serve-start --data-name live`. `A=$(cat <scratch>/serve.addr)` for `curl -N`.

- **Setup.** `$H api POST /projects '{"name":"live","path":"<scratch>/cwd"}'`, `$H api POST /work-items '{"projectId":1,"title":"Login"}'`, `$H api POST /spaces '{"projectId":1,"workItemId":1,"name":"build","backend":"claude","permissionMode":"acceptEdits"}'` (Space 1, `"role":"lead"`), `$H api POST /spaces '{"projectId":1,"name":"idea","backend":"claude"}'` (Space 2, quick session).
- **Events between turns.** `curl -sN --max-time 2 http://$A/spaces/1/events` prints `event: turn` / `data: {"status":"idle"}` first.
- **Send.** Start `curl -sN --max-time 120 http://$A/spaces/1/events > ~/.reef-verify/<id>/live-events.txt &`, then `$H api POST /spaces/1/send '{"text":"Remember this secret word: pineapple. List the files in this directory with a tool, then read one of them with a tool, then summarize what you saw in two sentences."}'` gives `HTTP 202`; at once `$H api GET '/messages?space=1'` already has the user message. A send before the turn ends is 202 and queues behind it (`bots.md`, Send while busy), so do not send a second one here. When `$H sql "select status from turn where space_id=1"` is `done`, the events file has `turn` running, `agent` lines with `"kind":"tool_call"` and `"kind":"text"`, and `turn` done. `$H api POST /spaces/1/send '{"text":""}'` gives 400.
- **Stop and resume.** `$H api POST /spaces/1/send '{"text":"Write a 1500-word story about lighthouses. Use no tools."}'`, wait 5 seconds, `$H api POST /spaces/1/stop` gives 204; the turn row is `failed|interrupted`. `$H api POST /spaces/1/send '{"text":"What was the secret word? Answer with the word only."}'`, then the newest `lead` message contains `pineapple`.
- **Text commands.** `$H api POST /spaces/1/send '{"text":"/attach"}'` gives 202; after the turn the user message is `/attach`, `$H api GET /spaces/1/proposal` is 404 and `$H api GET /work-items/1/brief/versions` is `[]`.
- **Wrap.** `$H api POST /spaces/2/wrap` gives 400 `quick session`. `$H api POST /spaces/1/wrap` gives 202; after the turn `$H api GET /spaces/1/proposal` is 200 `pending`. `$H api POST /proposals/1/reject` gives 200 `rejected` and brief versions stay `[]`; a second reject gives 400 `already rejected`.
- **Attach.** `$H api POST /spaces/2/send '{"text":"I want a search bar that filters products as you type. Answer in one or two sentences, use no tools."}'` and wait for `done`. `$H api POST /work-items '{"projectId":1,"title":"Search"}'` (2). `$H api POST /spaces/2/attach '{"workItemId":1}'` gives 409 `conflict: work item 1 already has an active Space`. `$H api POST /spaces/2/attach '{"workItemId":2}'` gives 200 with `"workItemId":2` and `"kind":"work_item"` and posts a `reef` message `This Space is now attached to work item "Search" (2). It works in <data>/worktrees/1/2 on work-item/2.` (the worktree itself is made at the Space's next wake); `$H api GET '/spaces?workItem=2'` has Space 2. A second attach on Space 2 gives 400 `already has work item`.
- **Caps.** `$H sql --write "update space set token_budget=1 where id=1"`, then a send gives 409 `paused: token budget of 1 used: N tokens` and the row is paused. `$H api POST /spaces/1/continue` gives 204, then `$H sql "select token_budget, paused from space where id=1"` is `<N + 1000000>|` (N from the 409 message); a second continue gives 409 `is not paused`.
- **Delete live.** `$H api POST /spaces/1/send '{"text":"Reply with one word."}'`, wait for `done` (the agent stays live), note the server's `claude` children, `pgrep -P $(cat <scratch>/serve.pid) -x claude`, and with the real `claude` its `reef mcp` too, `pgrep -f "reef-verify/<id>/bin/reef mcp"` (the fake starts `reef mcp` only for a tool call), then `$H api DELETE /spaces/1` gives 204 and that agent and its `reef mcp` are gone.
- `$H serve-stop`.

## Gotchas

- `?after=ID` compares message ids, not times; poll with the last id you saw.
- A Space's work item is checked once, on create. A Space keeps its project and work item; attach (`live-attach`) is the one move, from a free Space to a work item.
- Messages from author `reef` are written by the server after a restart, see `turns-recovery`.
- `send` answers before the turn runs; poll `$H sql "select status from turn ..."` (or the events stream) before checking the reply.
- The events stream sends a `: keepalive` comment every 15 seconds; `curl --max-time` ends it.
- A turn that fails for any reason other than Stop also stops the agent; the next send starts it again by resume.
- Every other prompt here matches a fake `claude` scenario (`fake/scenario.go`); keep the wording. On the fake, the `/attach` turn fails with `no scenario`, which the step does not check.
- A paused Space's 409 text ends with `: conflict`; match on `paused:` and the reason.
- Agents start lazily, on a Space's first turn, so a "starts no agent" `pgrep` check on a Space that has had no turn is true whatever the server does. Count agents before and after the call, on a Space that already ran.
