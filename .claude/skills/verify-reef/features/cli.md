# CLI

A user keeps a work item's memory from a terminal with the `reef` CLI: makes the current
folder a project, adds work items, reads the brief, records decisions as memory items,
and prints the context prompt. Each command opens the database itself, so it works
with no server and while `reef serve` runs on the same data folder. Contract:
`docs/memory.md`.

## Sub-features

- `cli-project` `reef project add [--name N]` makes the current folder project 1, named after the folder unless `--name` is given; a second add in the same folder fails with `... is already project 1`. Only that exact path is refused: a subfolder can become its own project.
- `cli-subfolder` a command run in a subfolder uses the project of the nearest parent folder that is one.
- `cli-work-item` `reef work-item add "<title>" [--type T] [--description D]` adds a work item to the current folder's project; a bad type fails.
- `cli-decide` `reef decide <work-item> "<body>" [--why W]` writes an accepted `decision` memory item in the work item's `work_item` scope with author and role `user`; the work item is an id.
- `cli-brief` `reef brief <work-item>` prints the latest brief, or fails with `has no brief yet`.
- `cli-context` `reef context <work-item> --role R` prints the context prompt, with a `Description:` line when the work item has a description.
- `cli-no-server` every write above works with no `reef serve` running.
- `cli-with-server` a CLI write while `reef serve` runs on the same folder succeeds and the server reads it at once.
- `cli-errors` an unknown work item, a folder that is no project, and no command fail with `reef: ...` and exit 1.
- `cli-dash-args` after `--` every argument is positional, even one that starts with `-`.

## How to get to it (user POV)

- `cd` into a repo, then `reef project add`, `reef work-item add "Email login"`, `reef decide 1 "..."`, `reef brief 1`, `reef context 1 --role developer`.

## Driving it with verify-reef

Preconditions:

- Baseline from the README. No server is running for this run (`$H serve-stop` if one is).
- The helper runs `reef` in the scratch cwd `<scratch>/cwd` and puts `--data DIR` right after the command words (`decide --data DIR 1 -- "--why not"`), so a `--` in the args still ends the flags. The first call names the folder: `$H reef --data-name cli -- ...`. Ids below assume it is new.

- **Project, no server.** `$H reef --data-name cli -- project add` prints `1  cwd  <scratch>/cwd` and `exit: 0`. `$H reef -- project add` prints `reef: <scratch>/cwd is already project 1` and `exit: 1`.
- **Work item and decision, no server.** `$H reef -- work-item add "Email login" --description "Users sign in with email and password"` prints `1  feature  Email login`; `$H reef -- work-item add "Crash on start" --type bug` prints `2  bug  Crash on start`. `$H reef -- decide 1 "Use sessions, not JWT" --why "One server, no token refresh"` prints `1  Use sessions, not JWT`. `$H sql "select id, kind, scope_kind || ':' || scope_id, body, why, author, role, status from memory"` shows `1|decision|work_item:1|Use sessions, not JWT|One server, no token refresh|user|user|accepted`; `$H sql "select count(*) from decision"` is `0`.
- **Brief.** `$H reef -- brief 1` fails with `reef: work item 1 has no brief yet`.
- **Context shelf.** `$H reef -- memory add lesson "on the shelf" --scope bot:1` and `... "by role name" --scope bot:lead`, then `$H reef -- context 1 --role lead`: `### Your items` has `lesson 2: on the shelf` only, because `lead` is Bot 1 (the work item's Team, else the Bot named like the role).
- **Context.** `$H reef -- context 1 --role developer` starts with `## You` and `You are developer on work item "Email login" (1) in project "cwd".`, then `Description: Users sign in with email and password`, has `## How to work`, with the tool line `Details are on demand through the reef MCP tools recall, read_memory, read_thread and search_sessions. Record decisions with remember. ...`, then `## What we know` with `### Decisions` and `decision 1: Use sessions, not JWT (user, <date>)`, and no `No brief yet.`.
- **With a server.** `$H serve-start --data-name cli`. `$H reef -- decide 1 "Lock after 5 failed logins"` prints `4  Lock after 5 failed logins` and `exit: 0`. `$H api GET '/memory?scope=work_item:1'` lists items 4 and 1, newest first, both `"author":"user"`. `$H api PUT /work-items/1/brief '{"body":"Email and password login","author":"user"}'` gives 201; `$H reef -- brief 1` prints `Email and password login`. `$H serve-stop` prints `exit=0`.
- **Errors.** `$H reef -- brief nope` prints `reef: work item "nope" is not an id`, `exit: 1`. `$H reef -- context 1` (no role) prints the context usage line, `exit: 1`. From a folder that is no project, run the binary directly, from the folder above `<scratch>` (`/tmp`, or `/private/tmp` on macOS): `cd <that folder> && <scratch>/bin/reef work-item add x --data <scratch>/cli` prints `reef: no reef project for <that folder>; run reef project add`.
- **Dash args.** `$H reef -- decide 1 -- "--why not"` prints `5  --why not`, `exit: 0`. `reef.log` shows `--- reef decide --data <dir> 1 -- '--why not'`.
- **Subfolder.** `$H reef --cwd cwd/sub/deeper -- work-item add "deep"` prints a work item of project 1: the nearest parent that is a project is the scratch cwd. `$H reef --cwd cwd/sub -- project add` prints `2  sub  <scratch>/cwd/sub`, `exit: 0`; `$H reef --cwd cwd/sub/deeper -- work-item add "deeper"` is now in project 2, the nearer parent. `$H reef --cwd cwd/sub -- project add` again prints `reef: <scratch>/cwd/sub is already project 2`, `exit: 1`.
- **Name.** On a new folder: `$H reef --data-name cli-name --cwd named -- project add --name "My Shop"` prints `1  My Shop  <scratch>/named`. `$H reef --cwd named -- work-item add Login`, then `$H reef --cwd named -- context 1 --role developer` starts with `## You` and `You are developer on work item "Login" (1) in project "My Shop".` and has no `Description:` line.

## Gotchas

- `reef.log` in the evidence dir has every call: the command with `--data`, each stdout line as `out:`, stderr as `err:`, and `exit:`. Quote it as proof.
- The project name is the scratch folder's name, `cwd`, because `project add` takes the folder name when there is no `--name`.
- `--cwd DIR` is relative to the run's scratch dir `<scratch>`, not to the scratch cwd, and is made if missing. Without it every call runs in the scratch cwd.
- `$H reef --data-name NAME` refuses while this run's server uses another folder. Stop it first.
- `sql` reads the last folder `reef --data-name` or `serve-start` used.
