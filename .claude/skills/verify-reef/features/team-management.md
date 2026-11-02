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

`REEF_VERIFY_RUN=<id> $H ui team-management` opens `/teams` (it must land on the Crews tab of `/bots`),
creates a Team from `full` through the window, edits its responsibility (saved on leaving the field,
no Save button), moves the star, removes and adds a Bot (each saves at once), runs a Space whose lead completes a scripted turn, checks deletion from the … menu,
and captures the Team editor at 1280x800 and 390x844.

For the deletion check, create a second Team through `POST /teams`, create a top-level Work
item, then call `DELETE /teams/<name>`. It must return 204 and keep the item: a Crew is a roster,
so deleting one never blocks on a Work item.
`POST`, `PUT`, and `GET /teams/<name>` must all return the responsibility text unchanged.

Clean up only this run:

```sh
REEF_VERIFY_RUN=<id> $H cleanup
```
