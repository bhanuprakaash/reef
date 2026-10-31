# Projects and work items

A user keeps projects (a name and a repo folder) and, inside a project, work items. A
work item has a type, a title, a description, a status and acceptance lines; a top-level one has a
brief that grows by versions, and one set of notes per role. Since memory items (`docs/product.md` decision 91) nothing reads the notes:
a role's own items are in scope `bot`. Their routes still work. Deleting a project deletes
its work items.

## Sub-features

- `project-crud` create, list, get, replace and delete a project; a missing field, a relative path or a path that is not a folder is 400, an unknown id 404; the path is stored with symlinks resolved (`/tmp` reads back as `/private/tmp` on macOS).
- `work-item-crud` work items list, filter by `?project=ID`, `?parent=ID`, `?status=S`, `?type=T`, get, replace and delete; type and status default; `acceptanceLines` read back with the item.
- `work-item-nesting` a sub-item of a sub-item is `invalid`; a brief on a sub-item is `invalid`; `projectId` and `parentId` never change after create.
- `work-item-status` a `PUT` ignores the body's `status`; `PATCH {"status":"queued"}` moves a backlog item (the user moves backlog and queued both ways); the user cannot move an item from queued to in_progress, the queue does (R41).
- `brief-versions` each `PUT .../brief` adds a version; the brief is the latest; old versions stay, oldest first.
- `role-notes` `PUT .../notes/{role}` creates or replaces one note per work item and role; it can be deleted.
- `project-cascade` deleting a project deletes its work items; ids are never reused.
- `flow-sub-items` the lead of a work item's Team makes two sub-items with `create_sub_item`: both are top-level item 1's children (`parentId` 1, type `task`, status `backlog`), have no Space of their own, and the inbox has one `sub_item` FYI item for each (`$H feature work-item-flow flow`).
- `flow-propose` a Bot's `propose_work_item` in a Space of the project adds a top-level `backlog` item with `proposedBy` the Bot, no owner (`ownerBotId` null), its acceptance lines, and a `proposed` FYI item in the inbox (`$H feature work-item-flow conv`).
- `decision-create` `POST /decisions` needs `workItemId`, `body` and `author`; `why` defaults to empty; `messageId` is optional.
- `decision-message` a `messageId` must be a message in a Space of the same work item, else 400.
- `decision-read` `GET /decisions?workItem=ID` lists a work item's decisions, oldest first; `GET /decisions/{id}` gets one.
- `decision-append-only` `PUT`, `PATCH` and `DELETE` on a decision are 405.
- `decision-cascade` deleting the message's Space keeps the decision and sets `messageId` null; deleting the work item deletes its decisions.

## How to get to it (user POV)

- `curl` against `reef serve`: `/projects`, `/work-items`, `/work-items/{id}/brief`, `/work-items/{id}/notes/{role}`.
- Decisions: `curl` against `reef serve`: `/decisions?workItem=ID`, `/decisions/{id}`.

## Driving it with verify-reef

Preconditions:

- Baseline from the README. Fresh server: `$H serve-stop`, then `$H serve-start --data-name work-items`. Ids below assume it.

- **Projects.** `$H api POST /projects '{"name":"shop","path":"/tmp"}'` gives 201 id 1. `$H api POST /projects '{"name":"blog","path":"/usr/bin"}'` gives id 2. `$H api GET /projects` lists both, oldest first. `$H api PUT /projects/1 '{"name":"shop2","path":"/usr"}'` gives 200 with the new name; `$H api PUT /projects/1 '{"name":"shop2"}'` gives 400; `$H api GET /projects/99` gives 404.
- **Work items.** `$H api POST /work-items '{"projectId":1,"type":"bug","title":"Sign-in fails","description":"500 on submit","acceptanceLines":[{"text":"no 500"},{"text":"error shown"},{"text":"test added"}]}'` gives 201, id 1, `"status":"backlog"` and three `acceptanceLines` with `position` 1, 2 and 3. `$H api POST /work-items '{"projectId":1,"parentId":1,"type":"task","title":"Reproduce"}'` and the same with `"title":"Fix it"` give ids 2 and 3. `$H api GET '/work-items?parent=1'` lists items 2 and 3; `$H api GET '/work-items?project=1&type=bug'` lists item 1; `$H api GET '/work-items?status=open'` gives 400 `invalid: unknown status "open"`; `$H api GET '/work-items?project=x'` gives 400.
- **Nesting.** `$H api POST /work-items '{"projectId":1,"parentId":2,"title":"Nested"}'` gives 400 `invalid: work item 2 is a sub-item and cannot have sub-items`. `$H api PUT /work-items/2/brief '{"body":"b","author":"lead"}'` gives 400 `invalid: work item 2 is a sub-item and has no brief`.
- **Update.** `$H api PUT /work-items/1 '{"title":"Sign-in fails","status":"queued"}'` gives 200, `"status":"backlog"` (the body's status is ignored), `"type":"bug"` (kept) and the three lines (kept); `'{"title":"x","acceptanceLines":[]}'` clears them. `$H api PATCH /work-items/1 '{"status":"queued"}'` gives 200 `"status":"queued"`; `$H api PATCH /work-items/1 '{"status":"in_progress"}'` gives 400 `invalid: work item 1: user cannot move an item from queued to in_progress`.
- **Brief.** `$H api GET /work-items/1/brief` gives 404 (none yet). `$H api PUT /work-items/1/brief '{"body":"first","author":"user"}'` gives 201 id 1; `$H api PUT /work-items/1/brief '{"body":"second","author":"user"}'` gives id 2. `$H api GET /work-items/1/brief` shows `second`; `$H api GET /work-items/1/brief/versions` shows `first` then `second`. `$H api PUT /work-items/1/brief '{"body":"x"}'` gives 400; `$H api GET /work-items/99/brief` gives 404. `$H sql "select id, work_item_id, body, author from brief"` shows both rows.
- **Notes.** `$H api GET /work-items/1/notes/developer` gives 404. `$H api PUT /work-items/1/notes/developer '{"body":"first"}'` then `$H api PUT /work-items/1/notes/developer '{"body":"second"}'` both give 200; `$H api GET /work-items/1/notes/developer` shows `second`; `$H sql "select work_item_id, role, body from role_notes"` has one row. `$H api PUT /work-items/1/notes/tester '{"body":""}'` gives 200 (empty is allowed). `$H api DELETE /work-items/1/notes/developer` gives 204, then `$H api GET /work-items/1/notes/developer` and a second `$H api DELETE /work-items/1/notes/developer` give 404. `$H api PUT /work-items/99/notes/developer '{"body":"x"}'` gives 404.
- **Cascade and ids.** `$H api DELETE /projects/2` gives 204; `$H api GET /projects/2` gives 404; `$H sql "select id, project_id from work_item"` has no project 2 row. `$H api POST /projects '{"name":"new","path":"/"}'` gives id 3, not 2. `$H api DELETE /work-items/1` gives 204 and `$H sql "select count(*) from brief"` is 0.
- **Sub-items and proposals by agents.** `$H feature work-item-flow flow` and `$H feature work-item-flow conv` drive `flow-sub-items` and `flow-propose` on the scripted `claude`: the lead's first turn replays `flow-lead-start` (two `create_sub_item` calls), a conversation Bot's turn replays `flow-propose-kit` (one `propose_work_item`). The tools by hand are in `reef-mcp.md`.

Decisions, on a fresh server (`$H serve-stop`, then `$H serve-start --data-name decisions`). Ids below assume it:

- Setup: `$H api POST /projects '{"name":"shop","path":"/tmp"}'`, `$H api POST /work-items '{"projectId":1,"title":"Login"}'`, `$H api POST /work-items '{"projectId":1,"title":"Signup"}'`, `$H api POST /spaces '{"projectId":1,"workItemId":1,"name":"build","backend":"claude"}'`, `$H api POST /spaces '{"projectId":1,"workItemId":2,"name":"build","backend":"claude"}'`, `$H api POST /messages '{"spaceId":1,"author":"user","body":"Use sessions"}'` (message 1), `$H api POST /messages '{"spaceId":2,"author":"user","body":"Email only"}'` (message 2).

- **Create.** `$H api POST /decisions '{"workItemId":1,"messageId":1,"body":"Use sessions, not JWT","why":"One server, no token refresh","author":"user"}'` gives 201 id 1 with `"messageId":1`. `$H api POST /decisions '{"workItemId":1,"body":"No social login","author":"architect"}'` gives id 2, `"messageId":null`, `"why":""`.
- **Create errors.** All 400: `$H api POST /decisions '{"workItemId":1,"body":"x"}'` (no author), `$H api POST /decisions '{"workItemId":1,"author":"user"}'` (no body), `$H api POST /decisions '{"body":"x","author":"user"}'` (no work item), `$H api POST /decisions '{"workItemId":99,"body":"x","author":"user"}'`, `$H api POST /decisions '{"workItemId":1,"messageId":99,"body":"x","author":"user"}'` and `$H api POST /decisions '{"workItemId":1,"messageId":2,"body":"x","author":"user"}'` (message 2 is in work item 2's Space).
- **Read.** `$H api GET '/decisions?workItem=1'` lists 1 and 2; `$H api GET '/decisions?workItem=2'` is `[]`; `$H api GET /decisions` gives 400; `$H api GET '/decisions?workItem=99'` gives 404; `$H api GET '/decisions?workItem=x'` gives 400. `$H api GET /decisions/1` gives 200; `$H api GET /decisions/99` gives 404.
- **Append-only.** `$H api PUT /decisions/1 '{"body":"edit"}'`, `$H api PATCH /decisions/1 '{"body":"edit"}'` and `$H api DELETE /decisions/1` give 405 `{"error":"method not allowed"}`; `$H api POST /decisions/1 '{}'` gives 405 too. `$H sql "select id, body from decision"` still shows the original text.
- **Cascade.** `$H api DELETE /spaces/1` gives 204; `$H api GET /decisions/1` is still 200 with `"messageId":null`. `$H api DELETE /work-items/1` gives 204; `$H api GET /decisions/1` and `$H api GET '/decisions?workItem=1'` give 404; `$H sql "select count(*) from decision"` is 0.

## Gotchas

- Create and update take the whole object; a left-out field takes its default, except `type`, `status`, `tokenBudget` and `acceptanceLines`, which a `PUT` keeps; a `PUT` without `description` blanks it.
- Unknown JSON fields in a body are 400 naming the field; server-set fields the entity returns (`id`, `createdAt`, and `updatedAt` where it has one) are accepted and ignored; `updatedAt` on a project is 400.
- An id in the path that matches nothing is 404; an id inside the body that matches nothing is 400.
- A failed create (409 or 400) writes no row, so the next create still gets the next id.
- A decision cannot be fixed or retracted through the API; to correct one, add a new decision that supersedes it.
- `author` is free text, not checked against a role list.
- The `messageId` rule is checked once, on create; losing the message later only nulls the link.
