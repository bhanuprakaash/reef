# Window

A user opens the Reef window in a browser at the address `reef serve` prints. They start a
channel on a feature, chat and watch the agent's tool calls and text arrive live. A new
channel of the same feature starts from the brief. A quick session can be promoted to a
feature, and a channel that hits a cap pauses and says why. A channel on a team names the running teammate, opens replies in a Thread pane and asks Approve or Reject for a gated wake. Contract: `docs/server.md`
(Web UI, Channels live, Wrap-up and promotion, Caps).

## Sub-features

- `window-live` a claude channel on a feature: a tool-using prompt shows tool rows while the turn runs; when it ends the reply is one stored `lead` message and the live view is gone.
- `window-brief` a new channel on a feature whose brief exists answers the goal from that brief.
- `window-promote` a quick session promoted with a slug and name ends the turn with a proposal panel; the sidebar moves the channel under the new feature; Approve makes brief version 1.
- `window-cap` a channel whose token budget is used shows a banner with the reason and a disabled composer, keeps it after a reload, and the next message runs after Continue.
- `window-pages` a bad project path shows the server's error; a feature's stage survives a reload; a task added on the tasks page is marked done and shows under the done filter.
- `window-stop` Stop mid-turn shows `Failed: interrupted`; the next message resumes the session.
- `window-wrap` Wrap up shows the proposal panel, Reject leaves the brief unchanged; `/wrap` then Approve makes brief version 1.
- `window-teams` the team picker makes a `duo` channel; the live turn names the lead, then the architect; a reply in the Thread pane is answered by the architect and the root shows `2 replies`; in a `gated` channel Approve runs the architect and the row goes (the recipe is in `teams.md`).
- `window-retry` a failed turn shows a row with Retry; Retry runs the same wake as a new turn that ends `done`, and the row goes.

Acceptance lines of the slice: "start a channel, chat, see streamed tool calls and text live" is
`window-live`; "a new channel of the same feature starts from the brief" is `window-brief`;
"promote a quick session" is `window-promote`; "hitting a cap pauses the channel and shows why" is
`window-cap`. Not driven here: codex channels in the window and reload mid-turn (drive them by hand
in the browser).

## How to get to it (user POV)

- `cd web && npm ci && npm run build`, `go build -o reef ./cmd/reef`, `./reef serve`, open `http://127.0.0.1:7700`.
- Add a project, add a feature, click New channel (or New quick session), type, Enter.

## Driving it with verify-reef

Preconditions:

- Baseline from the README, plus `node` and a Chrome: `/Applications/Google Chrome.app` on macOS; on Linux Playwright's Chromium with `REEF_VERIFY_CHROME` set (see `SKILL.md`, Window). Check it like `doctor` would: `"$REEF_VERIFY_CHROME" --version` prints a version. `error while loading shared libraries` means the system libraries are missing: `npx playwright@1.64.0 install-deps chromium`.
- The web app is built before `$H build`: `(cd web && npm ci && npm run build)`. The binary embeds `web/dist`; without it every page is 503 `UI not built`.
- The server starts with the model set, because it starts the `claude` agents: `ANTHROPIC_MODEL=sonnet REEF_VERIFY_RUN=<id> $H serve-start --data-name ui`.
- `ui teams` also needs the teams `duo` and `gated` applied to the `ui` folder: `$H reef --data-name ui -- team apply ...` before `serve-start` (files and commands in `teams.md`, Window).

Each scenario adds its own project through the form (`ui-<scenario>-<suffix>`, folder = the run's scratch cwd), so they run in any order on one server.

```sh
REEF_VERIFY_RUN=<id> $H ui live
REEF_VERIFY_RUN=<id> $H ui brief
REEF_VERIFY_RUN=<id> $H ui promote
REEF_VERIFY_RUN=<id> $H ui cap
REEF_VERIFY_RUN=<id> $H ui pages
REEF_VERIFY_RUN=<id> $H ui stop
REEF_VERIFY_RUN=<id> $H ui wrap
REEF_VERIFY_RUN=<id> $H ui teams
REEF_VERIFY_RUN=<id> $H ui retry
```

- **live.** Prints a `PASS live:` line for the tool rows seen mid-turn (the first row's tool name and input), the one stored `lead` message (with the message rows) and the cleared live view. Screenshots `live-midturn.png` (tool rows under `LEAD is working...`; the label is the running role in capitals, `ARCHITECT is working...` in a team) and `live-final.png`.
- **brief.** PUTs a brief (`Goal: build the login page with magic links.`), starts a channel, asks the goal, prints the answer and `PASS brief:` when it mentions magic links. Screenshot `brief-answer.png`.
- **promote.** Two exchanges in a quick session, Promote with slug `search` and name `Search`, waits for the proposal panel and prints its text, checks the sidebar (channel under Search, no Quick sessions heading in that project), clicks Approve, then reads `Version 1 by` on the feature page and one `brief` row for the feature. Screenshots `promote-before-promote`, `promote-form`, `promote-proposal`, `promote-approved`, `promote-feature-page`.
- **cap.** One exchange, then `update channel set token_budget=1` through `sqlite3` on the run's data folder, then a send. Prints the banner text (`Paused: token budget of 1 used: N tokens`), checks the composer is disabled and the banner survives a reload, clicks Continue, sends again and checks a second `lead` message and `token_budget` 1000001 with `paused` empty. Screenshots `cap-paused`, `cap-continued`, `cap-ran-after-continue`.
- **pages.** Adds a project with path `/nonexistent-reef-ui` and reads the `not a directory` error, adds a project and feature, sets the stage to Live testing, saves and reloads, then on the tasks page adds a task to that feature, filters by project and feature, clicks Mark done and filters by done. Screenshots `pages-bad-path`, `pages-tasks-done`.
- **stop.** Asks for a long story with a secret word, clicks Stop after a few seconds, waits for `Failed: interrupted`, then asks for the word and checks the answer. Screenshots `stop-stopped`, `stop-resumed`.
- **wrap.** One exchange with a decision, Wrap up, Reject (`rejected`, no brief row), then `/wrap` in the composer, Approve, and `Version 1 by` on the feature page. Screenshots `wrap-proposal`, `wrap-approved`.
- **teams.** Adds a project and feature, starts a channel on `duo` from the form (Team select, Start), checks `GET /channels/N` has team `duo`, sends `DESIGN a short plan for a login form.`, and sees `LEAD is working...` then `ARCHITECT is working...` and the architect's root with a Reply button. Opens the Thread pane, replies `Which field would you validate first? Answer in this thread.`, sees the architect answer in the pane (root, user, architect), closes it and reads `2 replies` on the root. Then a `gated` channel: the `lead → architect` row with Approve and Reject, no architect turn yet, Approve, `ARCHITECT is working...`, the row gone and the `approval` row `approved`. Screenshots `teams-team-picked`, `teams-lead-working`, `teams-architect-working`, `teams-architect-posted`, `teams-thread-open`, `teams-thread-architect-working`, `teams-thread-answered`, `teams-main-list`, `teams-gate-approval`, `teams-gate-approved`, `teams-gate-architect-posted`.
- **retry.** Sends a long prompt, clicks Stop, waits for the row `Turn N by lead failed: interrupted` and its Retry button, clicks Retry, sees `LEAD is working...`, waits for it to end and the row to go, then reads the `turn` rows: the first `failed` with `retried` 1, the second `done`. Screenshots `retry-failed-row`, `retry-retrying`, `retry-retried`.

Each line is `PASS <scenario>: <what> | <what it saw>` or `FAIL ...`. The command exits 1 on any `FAIL`; an error inside a scenario is a `FAIL scenario aborted` line and an `error.png`.

Proof: the output is saved in `~/.reef-verify/<id>/ui-<scenario>.log`, screenshots in `~/.reef-verify/<id>/ui/<scenario>-<step>.png`. Open the screenshots and look at them. Add SQL rows with `$H sql "select id, author, substr(body,1,60) from message"`, `$H sql "select id, status from turn"`, `$H sql "select id, feature_id, author from brief"`, `$H sql "select id, channel_id, status from proposal"`.

## Gotchas

- Not run by `go test` or `npm test`; every scenario starts a real `claude` and takes minutes. Run each once.
- Scenarios assume the model answers short, tool-free prompts as asked. A `FAIL` on wording (`brief`) needs the screenshot before it is called a bug.
- `cap` writes to `reef.db` while the server runs; it is the one write the API cannot do (`tokenBudget` is set by the server only).
- Quote the PASS lines and say which screenshots you opened. A screenshot you did not open is not proof.
