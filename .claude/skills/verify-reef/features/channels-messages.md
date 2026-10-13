# Channels and messages

A user talks to the team in channels. A quick session is a channel with no feature; a
feature channel belongs to one feature. Messages are added to a channel and never edited
or removed one by one.

## Sub-features

- `channel-create` a quick-session channel (`featureId` null) and a feature channel; the feature must be of the same project. `backend` is `claude` or `codex` (it defaults to the team lead's), `role` is always `lead`, and `permissionMode` must suit the backend; the reply also has `paused` (`""`), `tokenBudget` (1000000) and `team` (`solo`); teams are in `teams.md`.
- `channel-filter` `GET /channels` filters by `?project=ID` and `?feature=ID`.
- `channel-fixed` update changes the name only; project and feature stay as created.
- `message-append` messages are added with `POST` and read with `GET /messages?channel=ID`; `?after=ID` returns only newer ones.
- `message-thread` a message has `parentId`: `null` at the top level, the root's id for a reply; a parent that is not a top-level message of the same channel is 400.
- `message-append-only` `PUT`, `PATCH` and `DELETE` on a message are 405.
- `channel-cascade` deleting a channel deletes its messages; deleting a feature deletes its channels.

## How to get to it (user POV)

- `curl` against `reef serve`: `/channels`, `/channels/{id}`, `/messages?channel=ID`.

## Driving it with verify-reef

Preconditions:

- Baseline from the README. Fresh server: `$H serve-stop`, then `$H serve-start --data-name channels-messages`. Ids below assume it.
- Setup: `$H api POST /projects '{"name":"shop","path":"/tmp"}'`, `$H api POST /projects '{"name":"blog","path":"/usr"}'`, `$H api POST /features '{"projectId":1,"slug":"login","name":"Login"}'`.

- **Create.** `$H api POST /channels '{"projectId":1,"name":"scratch","backend":"claude"}'` gives 201 id 1, `"featureId":null`, `"team":"solo"`. `$H api POST /channels '{"projectId":1,"featureId":1,"name":"build","backend":"claude"}'` gives id 2, `"featureId":1`. `$H api POST /channels '{"projectId":2,"featureId":1,"name":"x","backend":"claude"}'` (feature of another project), `$H api POST /channels '{"projectId":1,"featureId":99,"name":"x","backend":"claude"}'` and `$H api POST /channels '{"projectId":99,"name":"x","backend":"claude"}'` give 400. Agent rules: `$H api POST /channels '{"projectId":1,"name":"x","backend":"gemini"}'` gives 400 `backend must be claude or codex`, `... "backend":"claude","role":"dev"}'` 400 `role must be lead`, `... "backend":"codex","permissionMode":"acceptEdits"}'` 400 `permissionMode for codex`.
- **Filters.** `$H api GET '/channels?project=1'` lists 1 and 2; `$H api GET '/channels?feature=1'` lists only 2; `$H api GET '/channels?project=2'` is `[]`; `$H api GET '/channels?feature=x'` gives 400.
- **Fixed after create.** `$H api PUT /channels/2 '{"name":"build2","projectId":2,"featureId":null}'` gives 200 with `"name":"build2"`, `"projectId":1`, `"featureId":1`. `$H api PUT /channels/2 '{}'` gives 400; `$H api PUT /channels/99 '{"name":"x"}'` gives 404.
- **Messages.** `$H api POST /messages '{"channelId":2,"author":"user","body":"Start with the form"}'` gives 201 id 1; `$H api POST /messages '{"channelId":2,"author":"architect","body":"Agreed"}'` gives id 2. `$H api GET '/messages?channel=2'` lists both, oldest first. `$H api GET '/messages?channel=2&after=1'` lists only id 2; `$H api GET '/messages?channel=2&after=2'` is `[]`. `$H api GET '/messages?channel=1'` is `[]`.
- **Message errors.** `$H api GET /messages` 400 (channel required), `$H api GET '/messages?channel=99'` 404, `$H api GET '/messages?channel=x'` 400, `$H api POST /messages '{"channelId":99,"author":"a","body":"b"}'` 400, `$H api POST /messages '{"channelId":2,"body":"b"}'` 400.
- **Thread parent.** `$H api POST /messages '{"channelId":2,"parentId":1,"author":"architect","body":"Reply"}'` gives 201 id 3 with `"parentId":1`. A reply to a reply, `... "parentId":3 ...`, and an unknown parent, `... "parentId":99 ...`, give 400 `is not a top-level message of channel 2`. `$H api GET '/messages?channel=2'` has `"parentId":null` on ids 1 and 2 and `1` on id 3.
- **Append-only.** `$H api PUT /messages/1 '{"body":"edit"}'`, `$H api PATCH /messages/1 '{"body":"edit"}'` and `$H api DELETE /messages/1` give 405 `{"error":"method not allowed"}`. `$H sql "select id, body from message"` still shows the original text.
- **Cascade.** `$H api POST /channels '{"projectId":1,"featureId":1,"name":"later","backend":"claude"}'` gives id 3. `$H api DELETE /channels/2` gives 204; `$H api GET '/messages?channel=2'` gives 404; `$H sql "select count(*) from message"` is 0. `$H api DELETE /features/1` gives 204 and `$H api GET /channels/3` gives 404, while `$H api GET /channels/1` (the quick session) still gives 200. `$H api POST /channels '{"projectId":1,"name":"again","backend":"claude"}'` gives id 4, not 2.

## Gotchas

- `?after=ID` compares message ids, not times; poll with the last id you saw.
- A channel's feature is checked once, on create. Moving a channel is not possible; create a new one.
- Messages from author `reef` are written by the server after a restart, see `turns-recovery`.
