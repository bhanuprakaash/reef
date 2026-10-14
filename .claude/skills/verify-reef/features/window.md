# Window

A user opens the Reef window in a browser at the address `reef serve` prints. They start a
channel on a feature, chat and watch the agent's tool calls and text arrive live. A new
channel of the same feature starts from the brief. A quick session can be promoted to a
feature, and a channel that hits a cap pauses and says why. A channel on a team names the running teammate, opens replies in a Thread pane and asks Approve or Reject for a gated wake. Contract: `docs/server.md`
(Web UI, Channels live, Wrap-up and promotion, Caps).

## Sub-features

- `window-live` a claude channel on a feature: a tool-using prompt shows tool rows while the turn runs; when it ends the reply is one stored `lead` message and the live view is gone.
- `window-brief` a new channel on a feature whose brief exists answers the goal from that brief.
- `window-promote` a quick session promoted with a slug and name ends the turn with a proposal panel; the sidebar moves the channel under the new feature; the quick-session turns' checkpoints, taken in the project folder, no longer show a Rollback button; Approve makes brief version 1.
- `window-cap` a channel whose token budget is used shows a banner with the reason and a disabled composer, keeps it after a reload, and the next message runs after Continue.
- `window-pages` a bad project path shows the server's error; a feature's stage survives a reload; a task added on the tasks page is marked done and shows under the done filter.
- `window-stop` Stop mid-turn shows `Failed: interrupted` and the partial reply once; the next message resumes the session.
- `window-wrap` Wrap up shows the proposal panel, Reject leaves the brief unchanged; `/wrap` then Approve makes brief version 1.
- `window-teams` the team picker makes a `duo` channel; the live turn names the lead, then the architect; a reply in the Thread pane is answered by the architect and the root shows `2 replies`; in a `gated` channel Approve runs the architect and the row goes (the recipe is in `teams.md`).
- `window-retry` a failed turn shows a row with Retry; Retry runs the same wake as a new turn that ends `done`, and the row goes.
- `window-rollback` a channel with checkpointed turns lists them under Checkpoints, newest first, each with Rollback; Rollback asks to confirm, then the feature's worktree has the same HEAD, `git status`, `git log` and files as before the turn, `reef` posts `Rolled back to before turn N.`, and the next turn's prompt starts with that note.
- `window-inbox` the Inbox link and `/inbox` list what waits on the user, live from `/inbox/events`: a new approval in a channel that is not open shows with Approve and Reject and fires one desktop notification; Approve from the list runs the wake and the item goes; clicking an item, or its notification, opens its channel scrolled to and marking the item (the approval row, the failed turn row, the pause banner or the newest message); an item of the channel open and visible fires no notification.
- `window-diff` a feature's Diff section shows `GET /features/{id}/diff`: a turn that writes a file in the worktree, uncommitted, shows there as the file with its added line.
- `window-memory` the home page shows a project memory editor, empty for a new project; saving it stores the body (`GET /projects/{id}/memory`), and it survives a reload.
- `window-inspector` a channel's Inspector panel shows `GET /channels/{id}/inspector`: one block per role that ran with its turn count, tokens and cost from the API; a role block expands to its turns, newest first; a developer turn expands to its prompt and a tool call with its name, input and result.

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
- `ui inbox` needs `gated` too, and reads the badge as `1`, so it fails on a server that already lists other items: run it first, or on its own folder (`$H reef --data-name inbox -- team apply $E/gated.json`, then `serve-start --data-name inbox`).
- `ui inspector` needs the team `full` applied to the `ui` folder: `$H reef --data-name ui -- team apply "$PWD/teams/full.json"` before `serve-start`.

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
REEF_VERIFY_RUN=<id> $H ui rollback
REEF_VERIFY_RUN=<id> $H ui inbox
REEF_VERIFY_RUN=<id> $H ui diff
REEF_VERIFY_RUN=<id> $H ui memory
REEF_VERIFY_RUN=<id> $H ui inspector
```

- **live.** Prints a `PASS live:` line for the tool rows seen mid-turn (the first row's tool name and input), the one stored `lead` message (with the message rows) and the cleared live view. Screenshots `live-midturn.png` (tool rows under `LEAD is working...`; the label is the running role in capitals, `ARCHITECT is working...` in a team) and `live-final.png`.
- **brief.** PUTs a brief (`Goal: build the login page with magic links.`), starts a channel, asks the goal, prints the answer and `PASS brief:` when it mentions magic links. Screenshot `brief-answer.png`.
- **promote.** Makes the run's folder a git repo with one commit when it is not one. Two exchanges in a quick session (two rows under Checkpoints), Promote with slug `search` and name `Search`, waits for the proposal panel and prints its text, checks the sidebar (channel under Search, no Quick sessions heading in that project) and that the two quick-session turns have no row under Checkpoints, and that `GET /turns?channel=N&rollbackable=1` omits them, clicks Approve, then reads `Version 1 by` on the feature page and one `brief` row for the feature. Screenshots `promote-before-promote`, `promote-form`, `promote-proposal`, `promote-checkpoints-after-promote`, `promote-approved`, `promote-feature-page`.
- **cap.** One exchange, then `update channel set token_budget=1` through `sqlite3` on the run's data folder, then a send. Prints the banner text (`Paused: token budget of 1 used: N tokens`), checks the composer is disabled and the banner survives a reload, clicks Continue, sends again and checks a second `lead` message and `token_budget` 1000001 with `paused` empty. Screenshots `cap-paused`, `cap-continued`, `cap-ran-after-continue`.
- **pages.** Adds a project with path `/nonexistent-reef-ui` and reads the `not a directory` error, adds a project and feature, sets the stage to Live testing, saves and reloads, then on the tasks page adds a task to that feature, filters by project and feature, clicks Mark done and filters by done. Screenshots `pages-bad-path`, `pages-tasks-done`.
- **stop.** Asks for a long story with a secret word, clicks Stop after a few seconds, waits for `Failed: interrupted`, checks the first 40 characters of the stored `lead` message show once on the page, then asks for the word and checks the answer. Screenshots `stop-stopped`, `stop-resumed`.
- **wrap.** One exchange with a decision, Wrap up, Reject (`rejected`, no brief row), then `/wrap` in the composer, Approve, and `Version 1 by` on the feature page. Screenshots `wrap-proposal`, `wrap-approved`.
- **teams.** Adds a project and feature, starts a channel on `duo` from the form (Team select, Start), checks `GET /channels/N` has team `duo`, sends `DESIGN a short plan for a login form.`, and sees `LEAD is working...` then `ARCHITECT is working...` and the architect's root with a Reply button. Opens the Thread pane, replies `Which field would you validate first? Answer in this thread.`, sees the architect answer in the pane (root, user, architect), closes it and reads `2 replies` on the root. Then a `gated` channel: the `lead → architect` row with Approve and Reject, no architect turn yet, Approve, `ARCHITECT is working...`, the row gone and the `approval` row `approved`. Screenshots `teams-team-picked`, `teams-lead-working`, `teams-architect-working`, `teams-architect-posted`, `teams-thread-open`, `teams-thread-architect-working`, `teams-thread-answered`, `teams-main-list`, `teams-gate-approval`, `teams-gate-approved`, `teams-gate-architect-posted`.
- **retry.** Sends a long prompt, clicks Stop, waits for the row `Turn N by lead failed: interrupted` and its Retry button, clicks Retry, sees `LEAD is working...`, waits for it to end and the row to go, then reads the `turn` rows: the first `failed` with `retried` 1, the second `done`. Screenshots `retry-failed-row`, `retry-retrying`, `retry-retried`.
- **rollback.** Makes the run's folder a git repo with one commit when it is not one (a project with features needs it), adds a project and feature and a solo channel (its lead has `write`). One exchange makes the worktree and checkpoint 1; the scenario adds an untracked `mine.txt` and records HEAD, `git status --porcelain`, `git log --oneline` and `git ls-files` of the worktree. A second turn creates `new.txt`; the scenario commits it. It waits for `Before turn N by lead` under Checkpoints (two Rollback buttons), clicks that Rollback and accepts the confirm (`Roll back to before turn N?`), waits for `Rolled back to before turn N.` from reef, and compares the four records with the recorded ones, `new.txt` gone and `mine.txt` back. A third turn is sent, and the prompt claude got for it (its session file under `~/.claude/projects`) must have `reef wrote: Rolled back to before turn N.` after the header; `turn.wake_text` keeps the wake without it, since `reef` notes are added when the prompt is sent. Screenshots `rollback-checkpoints`, `rollback-rolled-back`.

- **diff.** Adds a project and feature and a solo channel, sends `Use the Write tool to create new.txt in the current directory containing the word hi. Then reply done.`, then opens the feature page and reads the Diff section: `new.txt` listed once as `added` with the line `hi`, and `GET /features/{id}/diff` returns the same file, status and line over REST. Screenshot `diff-diff.png`.

- **inspector.** Needs the team `full` (see above). Adds a project and feature, starts a `full` channel, sends `Say ok in one word.` to the lead, then `@developer Use the Write tool to create the file d.txt containing the word yes. Then reply done.` to wake the developer directly. Opens the Inspector panel and reads `GET /channels/{id}/inspector`: checks the lead and developer blocks show the turn count and input tokens the API reports, expands the developer block and its one turn, and reads its prompt and its Write tool call's name and result in the panel. Screenshots `inspector-panel.png`, `inspector-turn-open.png`.

- **inbox.** Needs the team `gated` (see `ui teams`). Grants the `notifications` permission and wraps `window.Notification` to record each `new Notification` (title and body). Adds a project and feature, starts a `gated` channel, sends `DESIGN a short plan for a login form.`, and clicks the Inbox link while the lead runs, so the channel is not open. Waits for the row `Approval: inbox room` (`lead wants to wake architect`) with Approve and Reject, reads the badge `1` and, after 3 s, exactly one notification titled `Approval: inbox room`; no architect turn yet. Calls the notification's `onclick` (opens `/channels/N#approval-M` with the row on screen and ringed), returns to the inbox and clicks the item (same), then Approve from the list: the row goes, the `approval` row is `approved` and the architect turn is `done`. The architect's answer then shows as a `Waiting: inbox room` item with Open and fires its own notification (two in all). Open on it lands on `#latest-message`, on screen and ringed. Two messages sent to that open channel through the API give new waiting items: the first, with the channel visible, fires no notification (still two); the second, with `document.visibilityState` forced to `hidden`, fires one (three). A fresh load of `/inbox` with the item listed fires none. Screenshots `inbox-approval-row`, `inbox-item-opened-channel`, `inbox-approved`, `inbox-after-wake`, `inbox-waiting-opened-channel`.

Each line is `PASS <scenario>: <what> | <what it saw>` or `FAIL ...`. The command exits 1 on any `FAIL`; an error inside a scenario is a `FAIL scenario aborted` line and an `error.png`.

Proof: the output is saved in `~/.reef-verify/<id>/ui-<scenario>.log`, screenshots in `~/.reef-verify/<id>/ui/<scenario>-<step>.png`. Open the screenshots and look at them. Add SQL rows with `$H sql "select id, author, substr(body,1,60) from message"`, `$H sql "select id, status from turn"`, `$H sql "select id, feature_id, author from brief"`, `$H sql "select id, channel_id, status from proposal"`.

## Gotchas

- Not run by `go test` or `npm test`. Runs on the fake `claude` by default (`SKILL.md`, Fake
  claude); real claude only with `REEF_VERIFY_CLAUDE=real` or `record`, and then every scenario
  takes minutes. Run each once.
- Scenarios assume the model answers short, tool-free prompts as asked. A `FAIL` on wording (`brief`) needs the screenshot before it is called a bug.
- `cap` writes to `reef.db` while the server runs; it is the one write the API cannot do (`tokenBudget` is set by the server only).
- Quote the PASS lines and say which screenshots you opened. A screenshot you did not open is not proof.
