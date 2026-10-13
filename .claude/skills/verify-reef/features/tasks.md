# Tasks

A user keeps a global task list. A task belongs to a project and may name a feature. A
task is `todo`, `doing` or `done`, and records when it last changed.

## Sub-features

- `task-crud` create, get, replace and delete; a missing title is 400.
- `task-filter` `GET /tasks` is global and filters by `?project=ID`, `?feature=ID` and `?status=...`; a bad id or status is 400.
- `task-status` `todo` (default), `doing`, `done`; anything else is 400.
- `task-updated` `updatedAt` moves on every update; `createdAt` does not.
- `task-feature-delete` deleting a feature keeps its tasks and sets their `featureId` to null; deleting a project deletes its tasks.

## How to get to it (user POV)

- `curl` against `reef serve`: `/tasks`, `/tasks/{id}`.

## Driving it with verify-reef

Preconditions:

- Baseline from the README. Fresh server: `$H serve-stop`, then `$H serve-start --data-name tasks`. Ids below assume it.
- Setup: `$H api POST /projects '{"name":"shop","path":"/tmp/shop"}'`, `$H api POST /projects '{"name":"blog","path":"/tmp/blog"}'`, `$H api POST /features '{"projectId":1,"slug":"login","name":"Login"}'`.

- **Create.** `$H api POST /tasks '{"projectId":1,"featureId":1,"title":"Build the form"}'` gives 201 id 1, `"status":"todo"`, `"body":""`, `updatedAt` equal to `createdAt`. `$H api POST /tasks '{"projectId":1,"title":"Chore"}'` gives id 2, `"featureId":null`. `$H api POST /tasks '{"projectId":2,"title":"Other"}'` gives id 3.
- **Create errors.** `$H api POST /tasks '{"projectId":1}'` (no title), `$H api POST /tasks '{"projectId":99,"title":"x"}'`, `$H api POST /tasks '{"projectId":2,"featureId":1,"title":"x"}'` (feature of another project) and `$H api POST /tasks '{"projectId":1,"title":"x","status":"blocked"}'` give 400.
- **Filters.** `$H api GET /tasks` lists all 3. `$H api GET '/tasks?project=1'` lists 1 and 2; `$H api GET '/tasks?feature=1'` lists 1; `$H api GET '/tasks?status=todo'` lists 3; `$H api GET '/tasks?project=1&status=done'` is `[]`. `$H api GET '/tasks?status=blocked'` and `$H api GET '/tasks?project=x'` give 400.
- **Update.** `$H api PUT /tasks/1 '{"title":"Build the form","status":"doing","featureId":1}'` gives 200, `"status":"doing"`. `$H sql "select id, status, created_at, updated_at from task where id = 1"` shows `updated_at` later than `created_at`; `$H sql "select id, created_at, updated_at from task where id = 2"` shows task 2 untouched. `$H api GET '/tasks?status=doing'` lists only 1. `$H api PUT /tasks/1 '{"title":"Build the form","status":"done"}'` gives 200 with `"featureId":null` (PUT replaces). `$H api PUT /tasks/1 '{"title":"x","status":"blocked"}'` gives 400; `$H api PUT /tasks/99 '{"title":"x"}'` gives 404.
- **Feature delete keeps tasks.** `$H api PUT /tasks/1 '{"title":"Build the form","status":"todo","featureId":1}'`, then `$H api DELETE /features/1` gives 204; `$H api GET /tasks/1` gives 200 with `"featureId":null`; `$H sql "select count(*) from task"` is still 3.
- **Delete.** `$H api DELETE /tasks/2` gives 204, and `$H api GET /tasks/2` and a second `$H api DELETE /tasks/2` give 404. `$H api DELETE /projects/2` gives 204 and `$H api GET /tasks/3` gives 404 (the project's tasks went with it); task 1 stays.

## Gotchas

- `PUT` replaces the task: leave out `status` and it goes back to `todo`, leave out `featureId` and it becomes null.
- The list is global and has no paging; filter before asserting on a count.
- `updatedAt` has microsecond precision; compare the strings, do not expect whole seconds apart.
