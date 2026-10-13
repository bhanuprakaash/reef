# CLI

A user keeps a feature's memory from a terminal with the `reef` CLI: makes the current
folder a project, adds features, reads the brief, records decisions, adds and lists
tasks, and prints the context prompt. Each command opens the database itself, so it works
with no server and while `reef serve` runs on the same data folder. Contract:
`docs/memory.md`.

## Sub-features

- `cli-project` `reef project add` makes the current folder project 1; a second add fails with `... is already project 1`.
- `cli-feature` `reef feature add <slug> <name>` adds a feature to the current folder's project.
- `cli-decide` `reef decide <feature> "<body>" [--why W]` records a decision with author `user`; the feature is an id or a slug.
- `cli-task` `reef task add [<feature>] "<title>"` and `reef task list [<feature>] [--status S]`; a bad status fails.
- `cli-brief` `reef brief <feature>` prints the latest brief, or fails with `has no brief yet`.
- `cli-context` `reef context <feature> --role R` prints the context prompt.
- `cli-no-server` every write above works with no `reef serve` running.
- `cli-with-server` a CLI write while `reef serve` runs on the same folder succeeds and the server reads it at once.
- `cli-errors` an unknown feature, a folder that is no project, and no command fail with `reef: ...` and exit 1.
- `cli-slug-two-projects` a slug used in two projects fails with `invalid: feature slug "login" is in projects 1, 2; use the feature id`; the id works.
- `cli-dash-args` after `--` every argument is positional, even one that starts with `-`.

## How to get to it (user POV)

- `cd` into a repo, then `reef project add`, `reef feature add login "Email login"`, `reef decide login "..."`, `reef task add login "..."`, `reef brief login`, `reef context login --role developer`.

## Driving it with verify-reef

Preconditions:

- Baseline from the README. No server is running for this run (`$H serve-stop` if one is).
- The helper runs `reef` in the scratch cwd `/private/tmp/reef-verify/<id>/cwd` and puts `--data DIR` right after the command words (`task add --data DIR 1 -- "--why not"`), so a `--` in the args still ends the flags. The first call names the folder: `$H reef --data-name cli -- ...`. Ids below assume it is new.

- **Project, no server.** `$H reef --data-name cli -- project add` prints `1  cwd  /private/tmp/reef-verify/<id>/cwd` and `exit: 0`. `$H reef -- project add` prints `reef: /private/tmp/reef-verify/<id>/cwd is already project 1` and `exit: 1`.
- **Feature and decision, no server.** `$H reef -- feature add login "Email login" --goal "Users sign in with email and password"` prints `1  login  Email login`. `$H reef -- decide login "Use sessions, not JWT" --why "One server, no token refresh"` prints `1  Use sessions, not JWT`. `$H sql "select id, feature_id, body, why, author, message_id from decision"` shows `1|1|Use sessions, not JWT|One server, no token refresh|user|`.
- **Tasks.** `$H reef -- task add login "Build the login form"` prints `1  todo  Build the login form`; `$H reef -- task add "Personal todo"` prints `2  todo  Personal todo`. `$H reef -- task list` lists both; `$H reef -- task list login --status todo` lists only 1; `$H reef -- task list --status bogus` fails with `reef: invalid: unknown status "bogus"`. `$H sql "select id, project_id, feature_id from task"` shows `1|1|1` and `2|1|`.
- **Brief.** `$H reef -- brief login` fails with `reef: feature login has no brief yet`.
- **Context.** `$H reef -- context login --role developer` starts with `You are the developer on feature "Email login" (login) in project "cwd".`, has `No brief yet.`, `decision 1: Use sessions, not JWT (user, <date>)`, `task 1 [todo]: Build the login form`, and no `Personal todo`.
- **With a server.** `$H serve-start --data-name cli`. `$H reef -- decide 1 "Lock after 5 failed logins"` prints `2  Lock after 5 failed logins` and `exit: 0`. `$H api GET '/decisions?feature=1'` lists decisions 1 and 2. `$H api PUT /features/1/brief '{"body":"Email and password login","author":"user"}'` gives 201; `$H reef -- brief login` prints `Email and password login`. `$H serve-stop` prints `exit=0`.
- **Errors.** `$H reef -- brief nope` prints `reef: feature "nope": not found`, `exit: 1`. `$H reef -- context login` (no role) prints the context usage line, `exit: 1`. From a folder that is no project, run the binary directly: `cd /private/tmp && /private/tmp/reef-verify/<id>/bin/reef task list --data /private/tmp/reef-verify/<id>/cli` prints `reef: no reef project for /private/tmp; run reef project add`.
- **Dash args.** `$H reef -- task add 1 -- "--why not"` prints `3  todo  --why not`, `exit: 0`. `reef.log` shows `--- reef task add --data <dir> 1 -- '--why not'`.
- **Slug in two projects.** The helper always runs in the scratch cwd, so make the second project from a second folder with the binary: `mkdir /private/tmp/reef-verify/<id>/cwd2 && cd /private/tmp/reef-verify/<id>/cwd2`, then `/private/tmp/reef-verify/<id>/bin/reef project add --data /private/tmp/reef-verify/<id>/cli` prints `2  cwd2  /private/tmp/reef-verify/<id>/cwd2` and `.../bin/reef feature add login "Login again" --data ...` prints `2  login  Login again`. Back in the helper: `$H reef -- brief login` fails with `reef: invalid: feature slug "login" is in projects 1, 2; use the feature id`, `exit: 1`. `$H reef -- brief 1` prints `Email and password login`, `exit: 0`.

## Gotchas

- `reef.log` in the evidence dir has every call: the command with `--data`, each stdout line as `out:`, stderr as `err:`, and `exit:`. Quote it as proof.
- The project name is the scratch folder's name, `cwd`, because `project add` takes the folder name when there is no `--name`.
- `$H reef --data-name NAME` refuses while this run's server uses another folder. Stop it first.
- `sql` reads the last folder `reef --data-name` or `serve-start` used.
