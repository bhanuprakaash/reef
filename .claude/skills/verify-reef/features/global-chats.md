# Global chats and Spaces

A user starts a chat or a Space without a Project (All projects) or under one. Each is a
persistent history that survives a restart, runs in a neutral folder of its own and is only
associated with Projects: a filter never changes it. Contract: `docs/server.md` (Space projects,
Spaces live), product decisions 145 and 150.

## Sub-features

- `global-create` `POST /spaces` with no `projectId` makes a conversation and a free Space with `projectId` 0 and `GET /spaces/{id}/projects` `[]`; with a `projectId` the Space is associated with it at once.
- `global-restart` after `serve-stop` and `serve-start` on the same folder the three Spaces, their messages, turns, session ids and projects are the same, and a send to each resumes its session (`--resume <same id>`).
- `global-neutral` a Space with no work item runs in `<data>/chats/<Space id>`, also when it is associated with a Project: the fake `claude` log shows that `cwd`, the folder is not a git repo and no parent has `.git`.
- `global-no-target` the first prompt of those Spaces has no project folder and no `Base branch`; an associated Project is named only as `Associated projects, for context only; you do not work in them: <name> (<id>).`
- `global-new-chat` a second conversation with the same Bot is a separate Space with its own session: the first keeps its messages and turns, and both are in `GET /bots/{id}/conversations`.
- `global-filter` `GET /spaces?project=ID` lists the Spaces associated with the Project and none of the unassociated ones; asking with any filter changes no Space, no association and no session.
- `global-assoc` `PUT` and `DELETE /spaces/{id}/projects/{projectId}` edit the associations (unknown Project 404, a work item's Space 400); the Space's `projectId` is its lowest-id project.
- `global-delete-project` deleting a Project keeps a Space with no work item; it only loses the association.
- `window-global` All projects in the switcher: a Bot's page has New chat only, New chat starts an unassociated chat that runs in `<data>/chats/<id>`; reopening the page offers Resume chat for the latest chat and starts nothing; New chat again makes a second chat and the first is still listed; under a Project the Bot page lists and resumes only that Project's chats and New chat associates at once; switching the filter changes nothing of a reopened chat (`$H ui global-chats`).

## How to get to it (user POV)

- Window: the sidebar's project switcher, All projects at its top; a Bot's page (`/bots/{id}`) with Resume chat and New chat; the + menu's New conversation and New Space.
- REST: `POST /spaces {"name":"...","botId":1,"kind":"conversation"}` (no `projectId`), `GET /spaces/{id}/projects`, `GET /spaces?project=ID`, `PUT|DELETE /spaces/{id}/projects/{projectId}`.

## Driving it with verify-reef

Preconditions: baseline from the README. Fresh data folder `global`; the scratch cwd a git repo with one commit; `ANTHROPIC_MODEL=sonnet $H serve-start --data-name global`. No `sqlite3` is needed: use `$H api`. `D=<scratch>/global`.

- **Setup.** `$H api POST /projects '{"name":"shop","path":"<scratch>/cwd"}'` (1), `$H api POST /projects '{"name":"blog","path":"/usr"}'` (2). The seeded `solo` lead is Bot 1.
- **Create (`global-create`).** `$H api POST /spaces '{"name":"general chat","botId":1,"kind":"conversation"}'` is 201 id 1 with `"projectId":0`; `'{"name":"loose space","botId":1}'` is id 2, `"kind":"free"`, `"projectId":0`; `'{"name":"chat in shop","projectId":1,"botId":1,"kind":"conversation"}'` is id 3, `"projectId":1`. `$H api GET /spaces/1/projects` and `/spaces/2/projects` are `[]`, `/spaces/3/projects` lists `shop`.
- **Turns.** For `i` in 1 to 3: `$H api POST /spaces/$i/send '{"text":"Say ok in one word. (chat '$i')"}'`, and wait until `$H api GET "/turns?space=$i"` is `done` (scenario `say-ok-one-word`).
- **Neutral folder (`global-neutral`).** The three `fake-claude.log` lines have `cwd=<scratch>/global/chats/1`, `.../2` and `.../3` (Space 3 is associated with `shop` and still runs there). `ls -A $D/chats/1` prints nothing; `git -C $D/chats/1 rev-parse --is-inside-work-tree` fails (`not a git repository`); the loop `d=$D/chats/1; while [ "$d" != / ]; do [ -e $d/.git ] && echo FOUND $d; d=$(dirname $d); done` prints nothing.
- **No repository target (`global-no-target`).** `$H api GET /spaces/N/inspector`, the first turn's `prompt`: it has no `<scratch>/cwd`, no `/tmp/reef-verify` and no `Base branch`; Space 3's has `Associated projects, for context only; you do not work in them: shop (1).`, Spaces 1 and 2 have no such line.
- **Restart (`global-restart`).** Save, for each Space, `GET /spaces/N`, `GET '/messages?space=N'`, `GET '/turns?space=N'` (session ids) and `GET /spaces/N/projects`. `$H serve-stop`, `ANTHROPIC_MODEL=sonnet $H serve-start --data-name global`, save them again: the two files are equal (`tokensUsed` is the same too). `$H api POST /spaces/1/send '{"text":"Say ok in one word. (again)"}'` and the same for Space 2: the new turn has the same `sessionId` and its `fake-claude.log` line ends with `--resume <that id>`.
- **New chat (`global-new-chat`).** `$H api POST /spaces '{"name":"second general chat","botId":1,"kind":"conversation"}'` is id 4. Space 1 still has its messages and the one session; `$H api GET /bots/1/conversations` lists 1, 4 and 3.
- **Filter (`global-filter`).** `GET /spaces` lists 1 to 4; `GET '/spaces?project=1'` lists only 3; `GET '/spaces?project=2'` is `[]`. Run each of those twice around a snapshot of `GET /spaces/1`, `/spaces/1/projects`, `/spaces/3`, `/spaces/3/projects` and the turns' session ids: the snapshots are equal.
- **Associations (`global-assoc`).** `$H api PUT /spaces/1/projects/1` is 200 `[shop]`, `PUT /spaces/1/projects/2` lists both and `GET '/spaces?project=2'` has Space 1, `GET /spaces/1` has `"projectId":1`; `DELETE /spaces/1/projects/2` drops `blog`; `PUT /spaces/1/projects/99` is 404, `PUT /spaces/99/projects/1` is 404, `PUT /spaces/1/projects/x` is 400, and for a work item's Space (`POST /work-items '{"projectId":1,"title":"Login"}'`, `POST /spaces '{"projectId":1,"workItemId":1,"name":"wi"}'`) `PUT /spaces/N/projects/2` is 400 `stays in its project`. `POST /spaces '{"workItemId":1,"name":"wi"}'` with no `projectId` is 400.
- **Attach.** `$H api POST /spaces/2/attach '{"workItemId":1}'` on a Space with no project is 200, `"kind":"work_item"` with the work item's `projectId`, and `GET /spaces/2/projects` lists that project; a free Space associated with `blog` and `shop` attaches to a `shop` item the same way and keeps both projects.
- **Delete a Project (`global-delete-project`).** A second Space of `blog` (`POST /spaces '{"name":"in blog","projectId":2,"botId":1}'`), then `$H api DELETE /projects/2` is 204 and `GET /spaces/<id>` is 200 with `"projectId":0` and `GET /spaces/<id>/projects` is `[]`.
- **Window (`window-global`).** `$H serve-stop`, `ANTHROPIC_MODEL=sonnet $H serve-start --data-name ui`, build the web app first, `$H ui global-chats`: every line `PASS`, screenshots `ui/global-chats-all-projects-chat.png`, `-new-chat-second.png` and `-filter-switched.png`. Open them. The scenario makes its own Bot and second Project through REST and needs no `sqlite3`.
- `$H serve-stop`, `$H cleanup`.

## Gotchas

- The fake `claude` replies from a scenario by prompt: use the prompts above. The window scenario sends `Describe a reef.` and similar, which match `short-answer`.
- `projectId` 0 in a reply means no project; the column is NULL. Associations live in `space_project`; `$H sql "select * from space_project"` shows them when `sqlite3` exists.
- `<data>/chats/<id>` is made at a Space's first agent start, so a Space that never ran has no folder, and `DELETE /spaces/{id}` removes the folder.
- A free Space that is attached runs in the work item's worktree from its next wake; its older sessions were started in the chats folder and resume there (`spaces.md`, Attach).
- Repository work needs a work item: nothing here gives a neutral Space a repository.
