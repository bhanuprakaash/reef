# Spaces and messages

A user talks to the team in Spaces. A quick session is a Space with no feature; a
feature Space belongs to one feature. Messages are added to a Space and never edited
or removed one by one.

## Sub-features

- `space-create` a quick-session Space (`featureId` null) and a feature Space; the feature must be of the same project. `backend` is `claude` or `codex` (it defaults to the team lead's), a request `role` is empty or `lead`, and the reply's `role` is the name of the team's lead role (`lead` in `solo`, `boss` for a team whose lead is `boss`, where `"role":"boss"` is 400 `role must be lead`), and `permissionMode` must suit the backend; the reply also has `paused` (`""`), `tokenBudget` (1000000) and `team` (`solo`); teams are in `teams.md`.
- `space-filter` `GET /spaces` filters by `?project=ID` and `?workItem=ID`.
- `space-fixed` update changes the name only; project and feature stay as created.
- `message-append` messages are added with `POST` and read with `GET /messages?space=ID`; `?after=ID` returns only newer ones.
- `message-thread` a message has `parentId`: `null` at the top level, the root's id for a reply; a parent that is not a top-level message of the same Space is 400.
- `message-append-only` `PUT`, `PATCH` and `DELETE` on a message are 405.
- `space-cascade` deleting a Space deletes its messages; deleting a feature deletes its Spaces.

## How to get to it (user POV)

- `curl` against `reef serve`: `/spaces`, `/spaces/{id}`, `/messages?space=ID`.

## Driving it with verify-reef

Preconditions:

- Baseline from the README. Fresh server: `$H serve-stop`, then `$H serve-start --data-name spaces-messages`. Ids below assume it.
- Setup: `$H api POST /projects '{"name":"shop","path":"/tmp"}'`, `$H api POST /projects '{"name":"blog","path":"/usr"}'`, `$H api POST /work-items '{"projectId":1,"title":"Login"}'`.

- **Create.** `$H api POST /spaces '{"projectId":1,"name":"scratch","backend":"claude"}'` gives 201 id 1, `"workItemId":null`, `"team":"solo"`. `$H api POST /spaces '{"projectId":1,"workItemId":1,"name":"build","backend":"claude"}'` gives id 2, `"workItemId":1`. `$H api POST /spaces '{"projectId":2,"workItemId":1,"name":"x","backend":"claude"}'` (feature of another project), `$H api POST /spaces '{"projectId":1,"workItemId":99,"name":"x","backend":"claude"}'` and `$H api POST /spaces '{"projectId":99,"name":"x","backend":"claude"}'` give 400. Agent rules: `$H api POST /spaces '{"projectId":1,"name":"x","backend":"gemini"}'` gives 400 `backend must be claude or codex`, `... "backend":"claude","role":"dev"}'` 400 `role must be lead`, `... "backend":"claude","team":"unknown"}'` 400 `invalid: unknown team unknown`, `... "backend":"codex","permissionMode":"acceptEdits"}'` 400 `permissionMode for codex`.
- **Filters.** `$H api GET '/spaces?project=1'` lists 1 and 2; `$H api GET '/spaces?workItem=1'` lists only 2; `$H api GET '/spaces?project=2'` is `[]`; `$H api GET '/spaces?workItem=x'` gives 400.
- **Fixed after create.** `$H api PUT /spaces/2 '{"name":"build2"}'` gives 200 with `"name":"build2"`, `"projectId":1`, `"workItemId":1`. `$H api PUT /spaces/2 '{"name":"build2","projectId":2,"workItemId":null}'` gives 400 `invalid: bad JSON: json: unknown field "projectId"` (only `name` and `permissionMode` can change). `$H api PUT /spaces/2 '{}'` gives 400; `$H api PUT /spaces/99 '{"name":"x"}'` gives 404.
- **Messages.** `$H api POST /messages '{"spaceId":2,"author":"user","body":"Start with the form"}'` gives 201 id 1; `$H api POST /messages '{"spaceId":2,"author":"architect","body":"Agreed"}'` gives id 2. `$H api GET '/messages?space=2'` lists both, oldest first. `$H api GET '/messages?space=2&after=1'` lists only id 2; `$H api GET '/messages?space=2&after=2'` is `[]`. `$H api GET '/messages?space=1'` is `[]`.
- **Message errors.** `$H api GET /messages` 400 (Space required), `$H api GET '/messages?space=99'` 404, `$H api GET '/messages?space=x'` 400, `$H api POST /messages '{"spaceId":99,"author":"a","body":"b"}'` 400, `$H api POST /messages '{"spaceId":2,"body":"b"}'` 400.
- **Thread parent.** `$H api POST /messages '{"spaceId":2,"parentId":1,"author":"architect","body":"Reply"}'` gives 201 id 3 with `"parentId":1`. A reply to a reply, `... "parentId":3 ...`, and an unknown parent, `... "parentId":99 ...`, give 400 `is not a top-level message of space 2`. `$H api GET '/messages?space=2'` has `"parentId":null` on ids 1 and 2 and `1` on id 3.
- **Append-only.** `$H api PUT /messages/1 '{"body":"edit"}'`, `$H api PATCH /messages/1 '{"body":"edit"}'` and `$H api DELETE /messages/1` give 405 `{"error":"method not allowed"}`. `$H sql "select id, body from message"` still shows the original text.
- **Cascade.** `$H api POST /spaces '{"projectId":1,"workItemId":1,"name":"later","backend":"claude"}'` gives id 3. `$H api DELETE /spaces/2` gives 204; `$H api GET '/messages?space=2'` gives 404; `$H sql "select count(*) from message"` is 0. `$H api DELETE /work-items/1` gives 204 and `$H api GET /spaces/3` gives 404, while `$H api GET /spaces/1` (the quick session) still gives 200. `$H api POST /spaces '{"projectId":1,"name":"again","backend":"claude"}'` gives id 4, not 2.

## Gotchas

- `?after=ID` compares message ids, not times; poll with the last id you saw.
- A Space's feature is checked once, on create. Moving a Space is not possible; create a new one.
- Messages from author `reef` are written by the server after a restart, see `turns-recovery`.
