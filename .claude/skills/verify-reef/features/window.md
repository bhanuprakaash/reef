# Window

A user opens the Reef window in a browser at the address `reef serve` prints. They start a
channel on a feature, chat and watch the agent's tool calls and text arrive live. A new
channel of the same feature starts from the brief. A quick session can be promoted to a
feature, and a channel that hits a cap pauses and says why. Contract: `docs/server.md`
(Web UI, Channels live, Wrap-up and promotion, Caps).

## Sub-features

- `window-live` a claude channel on a feature: a tool-using prompt shows tool rows while the turn runs; when it ends the reply is one stored `lead` message and the live view is gone.
- `window-brief` a new channel on a feature whose brief exists answers the goal from that brief.
- `window-promote` a quick session promoted with a slug and name ends the turn with a proposal panel; the sidebar moves the channel under the new feature; Approve makes brief version 1.
- `window-cap` a channel whose token budget is used shows a banner with the reason and a disabled composer, keeps it after a reload, and the next message runs after Continue.

Acceptance lines of the slice: "start a channel, chat, see streamed tool calls and text live" is
`window-live`; "a new channel of the same feature starts from the brief" is `window-brief`;
"promote a quick session" is `window-promote`; "hitting a cap pauses the channel and shows why" is
`window-cap`. Not driven here: codex channels in the window, reload mid-turn, Stop, and Wrap up
(they are in the sub-issue acceptance lines; drive them by hand in the browser).

## How to get to it (user POV)

- `cd web && npm ci && npm run build`, `go build -o reef ./cmd/reef`, `./reef serve`, open `http://127.0.0.1:7700`.
- Add a project, add a feature, click New channel (or New quick session), type, Enter.

## Driving it with verify-reef

Preconditions:

- Baseline from the README, plus Chrome at `/Applications/Google Chrome.app` and `node`.
- The web app is built before `$H build`: `(cd web && npm ci && npm run build)`. The binary embeds `web/dist`; without it every page is 503 `UI not built`.
- The server starts with the model set, because it starts the `claude` agents: `ANTHROPIC_MODEL=sonnet REEF_VERIFY_RUN=<id> $H serve-start --data-name ui`.

Each scenario adds its own project through the form (`ui-<scenario>-<suffix>`, folder = the run's scratch cwd), so the four run in any order on one server.

```sh
REEF_VERIFY_RUN=<id> $H ui live
REEF_VERIFY_RUN=<id> $H ui brief
REEF_VERIFY_RUN=<id> $H ui promote
REEF_VERIFY_RUN=<id> $H ui cap
```

- **live.** Prints a `PASS live:` line for the tool rows seen mid-turn (the first row's tool name and input), the one stored `lead` message (with the message rows) and the cleared live view. Screenshots `live-midturn.png` (tool rows under `LEAD is working...`) and `live-final.png`.
- **brief.** PUTs a brief (`Goal: build the login page with magic links.`), starts a channel, asks the goal, prints the answer and `PASS brief:` when it mentions magic links. Screenshot `brief-answer.png`.
- **promote.** Two exchanges in a quick session, Promote with slug `search` and name `Search`, waits for the proposal panel and prints its text, checks the sidebar (channel under Search, no Quick sessions heading in that project), clicks Approve, then reads `Version 1 by` on the feature page and one `brief` row for the feature. Screenshots `promote-before-promote`, `promote-form`, `promote-proposal`, `promote-approved`, `promote-feature-page`.
- **cap.** One exchange, then `update channel set token_budget=1` through `sqlite3` on the run's data folder, then a send. Prints the banner text (`Paused: token budget of 1 used: N tokens`), checks the composer is disabled and the banner survives a reload, clicks Continue, sends again and checks a second `lead` message and `token_budget` 1000001 with `paused` empty. Screenshots `cap-paused`, `cap-continued`, `cap-ran-after-continue`.

Each line is `PASS <scenario>: <what> | <what it saw>` or `FAIL ...`. The command exits 1 on any `FAIL`; an error inside a scenario is a `FAIL scenario aborted` line and an `error.png`.

Proof: the output is saved in `~/.reef-verify/<id>/ui-<scenario>.log`, screenshots in `~/.reef-verify/<id>/ui/<scenario>-<step>.png`. Open the screenshots and look at them. Add SQL rows with `$H sql "select id, author, substr(body,1,60) from message"`, `$H sql "select id, status from turn"`, `$H sql "select id, feature_id, author from brief"`, `$H sql "select id, channel_id, status from proposal"`.

## Gotchas

- Not run by `go test` or `npm test`; every scenario starts a real `claude` and takes minutes. Run each once.
- Scenarios assume the model answers short, tool-free prompts as asked. A `FAIL` on wording (`brief`) needs the screenshot before it is called a bug.
- `cap` writes to `reef.db` while the server runs; it is the one write the API cannot do (`tokenBudget` is set by the server only).
- Quote the PASS lines and say which screenshots you opened. A screenshot you did not open is not proof.
