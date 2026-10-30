# Team management

Build and doctor a fresh run, build the window, start the server on its `ui` data folder, then make
the Bots and the team of `teams/full.json` (`$H team-apply` needs the running server):

```sh
H=.claude/skills/verify-reef/scripts/verify-reef
id=$($H build)
REEF_VERIFY_RUN=$id $H doctor
(cd web && npm run build)
ANTHROPIC_MODEL=sonnet REEF_VERIFY_RUN=$id $H serve-start --data-name ui
REEF_VERIFY_RUN=$id $H team-apply "$PWD/teams/full.json"
```

`REEF_VERIFY_RUN=<id> $H ui team-management` creates a Team from `full` through the window,
edits its responsibility, runs a Space whose lead completes a scripted turn, checks deletion,
and captures the Team editor at 1280x800 and 390x844.

For the deletion guard, create a second Team through `POST /teams`, create a top-level queued
Work item assigned to it, then call `DELETE /teams/<name>`. It must return 409 and name that
item. `POST`, `PUT`, and `GET /teams/<name>` must all return the responsibility text unchanged.

Clean up only this run:

```sh
REEF_VERIFY_RUN=<id> $H cleanup
```
