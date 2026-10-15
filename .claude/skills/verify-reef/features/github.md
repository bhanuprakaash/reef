# GitHub poll

`reef serve` polls `gh pr view` for each feature with a worktree, every `REEF_GH_INTERVAL`,
and posts a `reef` note in the feature's most recent channel when the PR appears, changes
state, or a check run finishes. A check run that finishes failed also wakes the `developer`
role in that channel, once per run, so it can look at the failure without the user pasting
in logs. Contract: `docs/server.md` (GitHub poll) and `docs/product.md` (decision 48).

## Sub-features

- `pr-note` a fake `gh` reporting an open PR: the feature's channel gets one `PR #N open: <url>` note.
- `pr-repeat` the same PR answer on a later poll posts nothing more.
- `check-success` a finished successful check posts one note naming the check.
- `check-failed-wake` a finished failed check posts a note and wakes the `developer` role: the turn's `wake_text` names the PR and the check, plus the tail of `gh run view <run id> --log-failed`.
- `restart-no-repeat` a restart of `reef serve` does not repeat the note or the wake.
- `check-rerun-wakes-again` the same check name finishing failed again, with a new run id, posts a new note and wakes the developer again.
- `solo-note-only` a feature on the `solo` team (no `developer` role) gets the failed-check note and no turn runs.
- `two-runs-same-name-both-noted` a rollup with two check runs under the same name (a matrix job, two workflows) notes both, once each, across repeat polls.
- `wake-retries-after-busy` a failed check on a paused channel still posts its note; the busy cause is logged once, and the wake runs once the channel is no longer paused.
- `no-gh-logged-once` with no `gh` on `PATH`, the server keeps running, posts nothing, and logs the cause once, not every poll.

## How to get to it (user POV)

- `reef team apply teams/full.json`, add a project that is a git repo, add a feature, make
  its worktree, open a channel on team `full`. Once the PR exists on GitHub, its status and a
  failed check show up as `reef` notes; a failed check also starts the developer, as if the
  user had pasted in the failure and asked it to look.
- REST: nothing new. The poll uses `GET /features`, `GET /projects/{id}` and posts through
  the existing message and turn paths.

## Driving it with verify-reef

`$H feature github` runs all of this and prints one PASS/FAIL line per sub-feature.

Preconditions:

- Baseline from the README. `ANTHROPIC_MODEL=sonnet` on every `serve-start`; the default
  fake `claude` is enough, since the developer's turn is proven by its stored `wake_text`,
  not by what it replies.
- In every Bash call set `H=.claude/skills/verify-reef/scripts/verify-reef`,
  `REEF_VERIFY_RUN=<id>`, `E=~/.reef-verify/<id>`, `S=$(cd /tmp && pwd -P)/reef-verify/<id>`
  and, once a server runs, `A=$(cat $S/serve.addr)`.
- The scratch cwd is the project and must be a git repo with one commit (see `full-team.md`,
  Preconditions).
- A fake `gh` goes first on `PATH`, built by the script into `$E/gh/bin/gh`: `pr view`
  answers with the content of `$GH_FAKE_RESPONSE` (or the content of `$GH_FAKE_ERR` on
  stderr, exit 1, when that file is non-empty), `run view <id> --log-failed` answers with the
  content of `$GH_FAKE_LOG`. The script switches these files between steps and relies on
  `REEF_GH_INTERVAL=1s` so a poll runs within a couple of seconds.
- `no-gh-logged-once` needs a `reef serve` with no `gh` anywhere on `PATH`, even though this
  sandbox has a real one at `/usr/bin/gh`. The script builds `$E/no-gh-bin`, a symlink farm of
  every file in `/usr/local/sbin`, `/usr/local/bin`, `/usr/sbin`, `/usr/bin`, `/sbin` and
  `/bin` except `gh`, and starts that server with `PATH` set to it alone.

```sh
ANTHROPIC_MODEL=sonnet REEF_VERIFY_RUN=<id> $H feature github
```

## Gotchas

- `gh pr view`'s `statusCheckRollup` carries `detailsUrl`/`completedAt` on a `CheckRun` and
  `targetUrl`/`startedAt` on a `StatusContext`; the poll keys a check's note and wake on
  those, not just its name, so the fake must change one of them for `check-rerun-wakes-again`
  to count as a new run, not a repeat of the first.
- The fake `gh` ignores which branch `pr view` was asked about, so every feature in the run
  sees the same PR and check answers; `solo-note-only` relies on this to get the same failed
  check as `check-failed-wake` without a second fake PR.
- `serve.log` is one file for the whole run, appended to across every `serve-start` in it, so
  `no-gh-logged-once` counts the line across the run, after sections that always find `gh`.
