# Projects and features

A user keeps projects (a name and a repo folder) and, inside a project, features. A
feature has a slug, a name, a goal and a stage, a brief that grows by versions, and one
set of notes per role. Deleting a project deletes its features.

## Sub-features

- `project-crud` create, list, get, replace and delete a project; a missing field is 400, an unknown id 404.
- `feature-crud` features list, filter by `?project=ID`, get, replace and delete; goal and stage default.
- `feature-slug` a slug is lowercase letters, digits and `-`, not starting with `-`; a taken slug in the same project is 409; the same slug in another project is fine; `projectId` and the slug never change after create.
- `brief-versions` each `PUT .../brief` adds a version; the brief is the latest; old versions stay, oldest first.
- `role-notes` `PUT .../notes/{role}` creates or replaces one note per feature and role; it can be deleted.
- `project-cascade` deleting a project deletes its features; ids are never reused.

## How to get to it (user POV)

- `curl` against `reef serve`: `/projects`, `/features`, `/features/{id}/brief`, `/features/{id}/notes/{role}`.

## Driving it with verify-reef

Preconditions:

- Baseline from the README. Fresh server: `$H serve-stop`, then `$H serve-start --data-name projects-features`. Ids below assume it.

- **Projects.** `$H api POST /projects '{"name":"shop","path":"/tmp/shop"}'` gives 201 id 1. `$H api POST /projects '{"name":"blog","path":"/tmp/blog"}'` gives id 2. `$H api GET /projects` lists both, oldest first. `$H api PUT /projects/1 '{"name":"shop2","path":"/tmp/shop2"}'` gives 200 with the new name; `$H api PUT /projects/1 '{"name":"shop2"}'` gives 400; `$H api GET /projects/99` gives 404.
- **Features.** `$H api POST /features '{"projectId":1,"slug":"login","name":"Login"}'` gives 201, `"goal":""`, `"stage":"ideation"`. `$H api POST /features '{"projectId":2,"slug":"login","name":"Login"}'` gives 201 (same slug, other project). `$H api GET '/features?project=1'` lists only feature 1; `$H api GET /features` lists both; `$H api GET '/features?project=x'` gives 400.
- **Slug.** `$H api POST /features '{"projectId":1,"slug":"login","name":"Again"}'` gives 409 `feature slug "login" is taken in its project: conflict`. `$H api POST /features '{"projectId":1,"slug":"Login","name":"x"}'`, `$H api POST /features '{"projectId":1,"slug":"-x","name":"x"}'` and `$H api POST /features '{"projectId":1,"slug":"a_b","name":"x"}'` give 400. `$H api POST /features '{"projectId":1,"slug":"9-ok","name":"Nine"}'` gives 201.
- **Update.** `$H api PUT /features/3 '{"projectId":2,"slug":"nine","name":"Nine","goal":"g","stage":"build"}'` gives 200, `"projectId":1` and `"slug":"9-ok"` (both unchanged), goal `g`, stage `build`.
- **Brief.** `$H api GET /features/1/brief` gives 404 (none yet). `$H api PUT /features/1/brief '{"body":"first","author":"user"}'` gives 201 id 1; `$H api PUT /features/1/brief '{"body":"second","author":"user"}'` gives id 2. `$H api GET /features/1/brief` shows `second`; `$H api GET /features/1/brief/versions` shows `first` then `second`. `$H api PUT /features/1/brief '{"body":"x"}'` gives 400; `$H api GET /features/99/brief` gives 404. `$H sql "select id, feature_id, body, author from brief"` shows both rows.
- **Notes.** `$H api GET /features/1/notes/developer` gives 404. `$H api PUT /features/1/notes/developer '{"body":"first"}'` then `$H api PUT /features/1/notes/developer '{"body":"second"}'` both give 200; `$H api GET /features/1/notes/developer` shows `second`; `$H sql "select feature_id, role, body from role_notes"` has one row. `$H api PUT /features/1/notes/tester '{"body":""}'` gives 200 (empty is allowed). `$H api DELETE /features/1/notes/developer` gives 204, then `$H api GET /features/1/notes/developer` and a second `$H api DELETE /features/1/notes/developer` give 404. `$H api PUT /features/99/notes/developer '{"body":"x"}'` gives 404.
- **Cascade and ids.** `$H api DELETE /projects/2` gives 204; `$H api GET /projects/2` gives 404; `$H sql "select id, project_id from feature"` has no project 2 row. `$H api POST /projects '{"name":"new","path":"/tmp/new"}'` gives id 3, not 2. `$H api DELETE /features/1` gives 204 and `$H sql "select count(*) from brief"` is 0.

## Gotchas

- Create and update take the whole object; a left-out field takes its default, so a `PUT` without `goal` blanks it.
- Unknown JSON fields in a body are 400 naming the field; `id`, `createdAt`, `updatedAt` are accepted and ignored.
- An id in the path that matches nothing is 404; an id inside the body that matches nothing is 400.
- A failed create (409 or 400) writes no row, so the next create still gets the next id.
