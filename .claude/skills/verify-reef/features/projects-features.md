# Projects and work items

A user keeps projects (a name and a repo folder) and, inside a project, work items. A
work item has a type, a title, a description, a status and acceptance lines; a top-level one has a
brief that grows by versions, and one set of notes per role. Since memory items (`docs/product.md` decision 91) nothing reads the notes:
a role's own items are in scope `teammate`. Their routes still work. Deleting a project deletes
its work items.

## Sub-features

- `project-crud` create, list, get, replace and delete a project; a missing field, a relative path or a path that is not a folder is 400, an unknown id 404; the path is stored with symlinks resolved (`/tmp` reads back as `/private/tmp` on macOS).
- `work-item-crud` work items list, filter by `?project=ID`, `?parent=ID`, `?status=S`, `?type=T`, get, replace and delete; type and status default; `acceptanceLines` read back with the item.
- `work-item-nesting` a sub-item of a sub-item is `invalid`; a brief on a sub-item is `invalid`; `projectId` and `parentId` never change after create.
- `brief-versions` each `PUT .../brief` adds a version; the brief is the latest; old versions stay, oldest first.
- `role-notes` `PUT .../notes/{role}` creates or replaces one note per work item and role; it can be deleted.
- `project-cascade` deleting a project deletes its work items; ids are never reused.

## How to get to it (user POV)

- `curl` against `reef serve`: `/projects`, `/work-items`, `/work-items/{id}/brief`, `/work-items/{id}/notes/{role}`.

## Driving it with verify-reef

Preconditions:

- Baseline from the README. Fresh server: `$H serve-stop`, then `$H serve-start --data-name projects-features`. Ids below assume it.

- **Projects.** `$H api POST /projects '{"name":"shop","path":"/tmp"}'` gives 201 id 1. `$H api POST /projects '{"name":"blog","path":"/usr/bin"}'` gives id 2. `$H api GET /projects` lists both, oldest first. `$H api PUT /projects/1 '{"name":"shop2","path":"/usr"}'` gives 200 with the new name; `$H api PUT /projects/1 '{"name":"shop2"}'` gives 400; `$H api GET /projects/99` gives 404.
- **Work items.** `$H api POST /work-items '{"projectId":1,"type":"bug","title":"Sign-in fails","description":"500 on submit","acceptanceLines":[{"text":"no 500"},{"text":"error shown"},{"text":"test added"}]}'` gives 201, id 1, `"status":"backlog"` and three `acceptanceLines` with `position` 1, 2 and 3. `$H api POST /work-items '{"projectId":1,"parentId":1,"type":"task","title":"Reproduce"}'` and the same with `"title":"Fix it"` give ids 2 and 3. `$H api GET '/work-items?parent=1'` lists items 2 and 3; `$H api GET '/work-items?project=1&type=bug'` lists item 1; `$H api GET '/work-items?status=open'` gives 400 `invalid: unknown status "open"`; `$H api GET '/work-items?project=x'` gives 400.
- **Nesting.** `$H api POST /work-items '{"projectId":1,"parentId":2,"title":"Nested"}'` gives 400 `invalid: work item 2 is a sub-item and cannot have sub-items`. `$H api PUT /work-items/2/brief '{"body":"b","author":"lead"}'` gives 400 `invalid: work item 2 is a sub-item and has no brief`.
- **Update.** `$H api PUT /work-items/1 '{"title":"Sign-in fails","status":"queued"}'` gives 200, `"status":"backlog"` (the body's status is ignored), `"type":"bug"` (kept) and the three lines (kept); `'{"title":"x","acceptanceLines":[]}'` clears them. `$H api PATCH /work-items/1 '{"status":"queued"}'` gives 200 `"status":"queued"`; `$H api PATCH /work-items/1 '{"status":"in_progress"}'` gives 400 `invalid: work item 1: user cannot move an item from queued to in_progress`.
- **Brief.** `$H api GET /work-items/1/brief` gives 404 (none yet). `$H api PUT /work-items/1/brief '{"body":"first","author":"user"}'` gives 201 id 1; `$H api PUT /work-items/1/brief '{"body":"second","author":"user"}'` gives id 2. `$H api GET /work-items/1/brief` shows `second`; `$H api GET /work-items/1/brief/versions` shows `first` then `second`. `$H api PUT /work-items/1/brief '{"body":"x"}'` gives 400; `$H api GET /work-items/99/brief` gives 404. `$H sql "select id, work_item_id, body, author from brief"` shows both rows.
- **Notes.** `$H api GET /work-items/1/notes/developer` gives 404. `$H api PUT /work-items/1/notes/developer '{"body":"first"}'` then `$H api PUT /work-items/1/notes/developer '{"body":"second"}'` both give 200; `$H api GET /work-items/1/notes/developer` shows `second`; `$H sql "select work_item_id, role, body from role_notes"` has one row. `$H api PUT /work-items/1/notes/tester '{"body":""}'` gives 200 (empty is allowed). `$H api DELETE /work-items/1/notes/developer` gives 204, then `$H api GET /work-items/1/notes/developer` and a second `$H api DELETE /work-items/1/notes/developer` give 404. `$H api PUT /work-items/99/notes/developer '{"body":"x"}'` gives 404.
- **Cascade and ids.** `$H api DELETE /projects/2` gives 204; `$H api GET /projects/2` gives 404; `$H sql "select id, project_id from work_item"` has no project 2 row. `$H api POST /projects '{"name":"new","path":"/"}'` gives id 3, not 2. `$H api DELETE /work-items/1` gives 204 and `$H sql "select count(*) from brief"` is 0.

## Gotchas

- Create and update take the whole object; a left-out field takes its default, except `type`, `status`, `tokenBudget` and `acceptanceLines`, which a `PUT` keeps; a `PUT` without `description` blanks it.
- Unknown JSON fields in a body are 400 naming the field; server-set fields the entity returns (`id`, `createdAt`, and `updatedAt` where it has one) are accepted and ignored; `updatedAt` on a project is 400.
- An id in the path that matches nothing is 404; an id inside the body that matches nothing is 400.
- A failed create (409 or 400) writes no row, so the next create still gets the next id.
