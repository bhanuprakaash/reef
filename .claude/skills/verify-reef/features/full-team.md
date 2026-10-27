# Full team

A user applies `teams/full.json` (lead, architect, developer, tester) and asks the lead for a
work item. Only a role with `write` changes files. Each work item works in its own git worktree on
`work-item/<id>`, and one write turn at a time runs on a tree. Before each write turn the
server takes a checkpoint, and the user can roll the tree back to it. A failed turn can be
retried. Everything that waits on the user (a pause, a failed turn, an agent's
last word) shows in one inbox, live, in the server and in the window. Contract:
`docs/server.md` (role, turn, Spaces live, Inbox) and `docs/memory.md` (Teams).

## Sub-features

One line per acceptance line of the full-team slice and its sub-issues.

- `write-claude-refused` a tester without `write`, asked to create a file with Write and then Bash, is refused both; the tree is unchanged; its `claude` runs with `--permission-mode default`, without `Edit` and `Write` in `--allowedTools` and with `--disallowedTools Edit,Write,MultiEdit,NotebookEdit`; the developer's `claude` has no `--disallowedTools`.
- `write-codex-readonly` on codex (test fixture), a role without `write` starts its thread with `"sandbox":"read-only"`, even when its `permissionMode` is `workspace-write`; a role with `write` gets `workspace-write`.
- `write-claude-allowed` a developer with `write`, asked the same, creates the file.
- `write-default` a team file with no `write` field loads with `write: false` for every role; the seeded `solo` lead has `write`.
- `full-apply` `reef team apply teams/full.json`, then `reef team list` shows 4 roles, lead starred.
- `full-rest` `GET /teams/full` shows developer `write: true`, the others `false`, and no `gates` key.
- `tree-two-features` one wake on work item `1` and one on `2` make two worktrees on `work-item/1` and `work-item/2`; each teammate's claude session has its own tree as `cwd`.
- `tree-reuse` a second turn in work item `a`'s Space runs in the same tree: `git worktree list` is unchanged and a file put there before stays.
- `tree-quick` a quick session's teammate runs in the project path.
- `tree-isolated` a file a developer wrote in `a`'s tree is absent from `b`'s and from the main checkout.
- `tree-not-git` a work item of a project that is not a git repo: one `failed` turn whose error starts `work items need the project folder to be a git repo with a commit: $S/plain: `, a log line, and `GET /health` still 200.
- `tree-attach-dirty` a free Space attached to a work item while the main checkout has an untracked file: `reef` posts where the work item works, the next wake resumes the same session in the worktree, and the main checkout is still dirty.
- `writer-parallel` developers of two work items run at the same time.
- `checkpoint-untracked` a developer turn has `checkpoint` `refs/reef/checkpoints/<D>/<turn id>`; `git show` of it holds an untracked file present before the turn, and its second parent holds what was staged.
- `checkpoint-untouched` HEAD, `git status` and the index are the same before and after a developer turn that uses no tools.
- `checkpoint-reader` a tester turn has `"checkpoint":null`.
- `checkpoint-upgrade` an old database opens and its turns have no checkpoint (`store/upgrade_test.go`).
- `rollback-restores` a developer edits a file, creates one and commits; rollback brings back HEAD, `git status`, `git log`, the index and every file of the checkpoint.
- `rollback-busy` rollback while the Space runs a turn is 409; nothing changes.
- `rollback-note` `reef` posts `Rolled back to before turn N.`, and the prompt of the developer's next wake starts with `reef wrote: Rolled back to before turn N.`; a turn with no checkpoint is 400, an unknown one 404.
- `retry-restart` a developer turn failed by a server crash (`server restarted`) is retried: a new developer turn ends `done` and the failed one is `retried`.
- `retry-twice` retrying it again, or retrying a `done` turn, is 409.
- `inbox-kinds` `GET /inbox` lists a `paused`, a `failed` and a `waiting` item.
- `inbox-clear` continue, retry and send each clear their item, and the Space runs on.
- `inbox-stream` `GET /inbox/events` sends a frame when an item appears and when it clears, across three Spaces.
- `inbox-restart` after a restart `GET /inbox` lists the same items, byte for byte.
- `window-retry` Retry on a failed turn row runs the turn again (`$H ui retry`).
- `window-rollback` Rollback with confirm restores the tree (`$H ui rollback`).
- `google-login` the spec's flow on a throwaway Go app with `teams/full.json`: lead, architect, developer, tester and lead run automatically; the developer commits in the worktree, the tester runs `go test`, and the main checkout is untouched.
- `handoff-no-approval` a lead reply that mentions `@developer` runs the developer automatically and stores no approval row.
- `note-retry` a developer turn failed by a crash is an inbox item; rollback, then retry: the retried prompt has the restart and rollback notes; the wake after it has none.
- `retry-superseded` a failed turn followed by a `done` turn of the same role: retry 409, not in `?retryable=1` nor the inbox.
- `retry-wrap` a failed wrap-up turn keeps no wake: retry 409, not in `?retryable=1` nor the inbox.
- `rollback-moved` a free-Space checkpoint, then attach to a work item: rollback of that turn is 409 and the work item worktree is unchanged.
- `rollback-branch` a developer that switched branch after the checkpoint: rollback leaves `work-item/<id>` at the checkpoint's HEAD and checked out.
- `rollback-chat` a Space open in `reef chat`: rollback is 409.
- `checkpoint-unborn` a project folder with `git init` and no commit: a quick session's lead turn is `done` with no checkpoint.
- `tree-recreate` a work item's worktree folder deleted by hand: the next wake makes it again with no error.
- `tree-branch-taken` the main checkout on `work-item/<id>` of work item `X`: a wake of work item `X` fails, naming the branch and the main checkout.
- `attach-clean` attach on a clean main checkout: a `reef` note names the worktree path and branch, and nothing about uncommitted changes.
- `space-active-one` a second Space for a work item that has an active Space is 409 `conflict: work item N already has an active Space`.
- `space-attach-list` a free Space attached to a work item shows in `GET /spaces?workItem=` with kind `work_item`.
- `space-members-frozen` editing a Team's roster does not change the members of a Space made from it: its member keeps the tools and `write` it was copied with, and a Space made after the edit has the new ones.
- `write-settings` a project `.claude/settings.json` that allows `Edit`: the tester's edit is still refused and the file unchanged.

## How to get to it (user POV)

- `reef team apply teams/full.json`, then in the window pick the team `full` in the New Space form of a work item. The project must be a git repo with a commit.
- Retry and Rollback are buttons on the failed turn row and under Checkpoints.
- REST: `POST /turns/{id}/retry`, `POST /turns/{id}/rollback`, `GET /inbox`, `GET /inbox/events`.

## Driving it with verify-reef

`$H feature full-team` runs all of this and prints one PASS/FAIL line per sub-feature.

Preconditions:

- Baseline from the README. `ANTHROPIC_MODEL=sonnet` on every `serve-start`. Codex is not needed: the codex line uses the repo's test fixture, so `doctor` showing `FAIL codex-auth` does not matter here. Start a server that uses the fixture with `BASH_ENV=` set: a `BASH_ENV` that resets `PATH` hides the shim and the real codex runs.
- In every Bash call set `H=.claude/skills/verify-reef/scripts/verify-reef`, `REEF_VERIFY_RUN=<id>`, `E=~/.reef-verify/<id>`, `S=$(cd /tmp && pwd -P)/reef-verify/<id>` and, once a server runs, `A=$(cat $S/serve.addr)`. Paste `wait_done` and `wait_for` from `teams.md`.
- `$H reef` runs in the scratch cwd, so a repo file is named by its full path: `$PWD/teams/full.json`.
- The scratch cwd is the project and must be a git repo with one commit:

```sh
git -C $S/cwd init -q -b main && echo one > $S/cwd/a.txt && git -C $S/cwd add a.txt && git -C $S/cwd -c user.name=v -c user.email=v@example.com commit -qm init
```

- Team files in `$E`. `crew` has a developer with `write` and a tester without it whose `tools` list `Edit` and `Write` and whose mode is `acceptEdits` (both must be dropped); `cx` puts both on codex; `relay` hands lead work to tester:

```sh
cat > $E/crew.json <<'EOF'
{
  "name": "crew",
  "roles": [
    {"name": "lead", "lead": true, "backend": "claude",
     "persona": "You are the lead. Answer in one short sentence. Never @mention anyone."},
    {"name": "developer", "write": true, "backend": "claude", "permissionMode": "acceptEdits",
     "persona": "You are the developer. Do what the user asks with as few tool calls as you can, then answer in one short sentence. Never @mention anyone.",
     "tools": ["Read", "Edit", "Write", "Bash"]},
    {"name": "tester", "backend": "claude", "permissionMode": "acceptEdits",
     "persona": "You are the tester. Do what the user asks, then answer in one short sentence. Never @mention anyone.",
     "tools": ["Read", "Edit", "Write", "Bash(ls:*)"]}
  ]
}
EOF
cat > $E/cx.json <<'EOF'
{
  "name": "cx",
  "roles": [
    {"name": "lead", "lead": true, "backend": "claude"},
    {"name": "developer", "write": true, "backend": "codex"},
    {"name": "tester", "backend": "codex", "permissionMode": "workspace-write"}
  ]
}
EOF
cat > $E/relay.json <<'EOF'
{
  "name": "relay",
  "roles": [
    {"name": "lead", "lead": true, "backend": "claude",
     "persona": "You are the lead. Whatever you are told, reply with exactly one line: @tester check this"},
    {"name": "tester", "backend": "claude",
     "persona": "You are the tester. Answer in one short sentence. Never @mention anyone."}
  ]
}
EOF
```

Teams, no server needed (data folder `full`):

- **Apply full.** `$H reef --data-name full -- team apply $PWD/teams/full.json` prints `applied team full: lead*(claude) architect(claude) developer(claude) tester(claude)`. Then `$H reef -- team apply $E/crew.json`, `$H reef -- team apply $E/cx.json`, `$H reef -- team apply $E/relay.json`. `$H reef -- team list` has the line `full  lead*(claude) architect(claude) developer(claude) tester(claude)` (`full-apply`).
- **Write column.** `$H sql "select team.name, member.name, lead, write from team_member member join team on team.id = team_id order by team.name, member.id"` has `crew|lead|1|0`, `crew|developer|0|1`, `crew|tester|0|0`, `full|lead|1|0`, `full|architect|0|0`, `full|developer|0|1`, `full|tester|0|0`, `relay|lead|1|0`, `relay|tester|0|0` and `solo|lead|1|1`: files with no `write` field load `0` (`write-default`).

Server (the same folder): `ANTHROPIC_MODEL=sonnet $H serve-start --data-name full`, set `A`.

- **REST.** `$H api GET /teams/full` is 200; its roles have `"write":false` for lead, architect and tester and `"write":true` for developer, and the body has no `gates` key (`full-rest`).
- **Setup.** `mkdir -p $S/plain`. `$H api POST /projects '{"name":"full","path":"'$S'/cwd"}'` (id 1), `$H api POST /projects '{"name":"plain","path":"'$S'/plain"}'` (id 2, not a git repo). Features: `{"projectId":1,"title":"A"}` (1), `{"projectId":1,"title":"B"}` (2), `{"projectId":2,"title":"No git"}` (3). Spaces: `{"projectId":1,"workItemId":1,"name":"a1","team":"crew"}` (1), `{"projectId":1,"name":"a2","team":"crew"}` (2, a free Space: a work item has one active Space), `{"projectId":1,"workItemId":2,"name":"b1","team":"crew"}` (3), `{"projectId":1,"name":"quick","team":"crew"}` (4, a quick session), `{"projectId":2,"workItemId":3,"name":"nogit"}` (5, `solo`). In every Space a first message `@developer ...` wakes only the developer: the lead never starts.

Worktrees and writers. `T=$S/full/worktrees/1/1`. A claude session's folder is `~/.claude/projects/*/<session id>.jsonl`; `grep -ho '"cwd":"[^"]*"' <that file> | sort -u` shows where it ran.

- **Two work items at once.** `$H api POST /spaces/1/send '{"text":"@developer Write the numbers from 1 to 300 separated by spaces. Use no tools."}'` and the same to `/spaces/3/send`, both 202, then `wait_done 1 1; wait_done 3 1`. `git -C $S/cwd worktree list` has three lines: `$S/cwd [main]`, `$S/full/worktrees/1/1 [work-item/1]`, `$S/full/worktrees/1/2 [work-item/2]`; `git -C $S/cwd status --porcelain` is empty (`tree-two-features`). `$H sql "select id, space_id, role, started_at, ended_at, checkpoint from turn order by id"`: turns 1 (Space 1) and 2 (Space 3) are `developer`, each started before the other ended (`writer-parallel`), with checkpoints `refs/reef/checkpoints/<D>/1` and `<D>/2`. The session of turn 1 has only `"cwd":"$S/full/worktrees/1/1"`, of turn 2 only `.../1/b`.
- **Same work item, a second turn.** `echo mark > $T/marker.txt`. Send `@developer Write the numbers from 1 to 600 separated by spaces. Use no tools.` to Space 1, then `wait_done 1 2`. `git -C $S/cwd worktree list` is unchanged and `cat $T/marker.txt` prints `mark` (`tree-reuse`).
- **Tester refused.** `git -C $T status --porcelain > $E/a-status-before.txt`. Send the 600 prompt to Space 1, `sleep 3`, and list the claude processes of the server: `for p in $(pgrep -P $(cat $S/serve.pid) -x claude); do ps -ww -o args= -p $p; done` shows the developer with `--permission-mode acceptEdits ... --allowedTools Read,Edit,Write,Bash,Task,mcp__reef__*` and no `--disallowedTools`. `wait_done 1 3`, then `$H api POST /spaces/1/send '{"text":"@tester Use the Write tool to create the file t.txt containing the word nope. If that is refused, try once with Bash. Say what happened."}'`, `sleep 4`, and list again: the tester has `--permission-mode default`, `--allowedTools Read,Bash(ls:*),mcp__reef__*` and `--disallowedTools Edit,Write,MultiEdit,NotebookEdit`. `wait_done 1 4`. `$T/t.txt` does not exist, `git -C $T status --porcelain` matches `$E/a-status-before.txt`, and the tester's session file has at least two `"is_error":true` results: the Write and the Bash were both refused (`write-claude-refused`). The tester turn has `"checkpoint":null` (`checkpoint-reader`).
- **Checkpoint.** `echo before > $T/pre.txt`, `echo staged > $T/st.txt && git -C $T add st.txt`. Save `git -C $T rev-parse HEAD`, `git -C $T status --porcelain` and `git -C $T ls-files --stage` to `$E/a-before.txt`. `$H api POST /spaces/1/send '{"text":"@developer Reply with one word. Use no tools."}'`, `wait_done 1 5`, `N=$($H sql "select max(id) from turn where space_id = 1" | tail -1)`. `$H api GET /turns/$N` has `"checkpoint":"refs/reef/checkpoints/<D>/<N>"` (`<D>` is 12 hex digits); `C=$($H sql "select checkpoint from turn where id = $N" | tail -1)`. The same three commands into `$E/a-after.txt`, and `diff $E/a-before.txt $E/a-after.txt` prints nothing (`checkpoint-untouched`). `git -C $T show $C:pre.txt` prints `before` and `git -C $T show $C^2:st.txt` prints `staged` (`checkpoint-untracked`).
- **Developer writes.** `$H api POST /spaces/1/send '{"text":"@developer Use the Write tool to create the file d.txt containing the word yes."}'`, `wait_done 1 6`. `cat $T/d.txt` prints `yes` (`write-claude-allowed`).
- **Old database.** `go test ./store -run Upgrade -v` passes: `TestOpen_UpgradesVersion1Database/turn` expects a turn with no checkpoint (`checkpoint-upgrade`).

Rollback, Space 1. `snap() { git -C $T rev-parse HEAD; git -C $T status --porcelain; git -C $T log --oneline; git -C $T ls-files --stage; for f in $(ls $T); do echo "$f: $(cat $T/$f)"; done; }`.

- **Restores.** `snap > $E/rb-before.txt` (HEAD is the first commit; `A  st.txt`, `?? d.txt`, `?? marker.txt`, `?? pre.txt`). `$H api POST /spaces/1/send '{"text":"@developer Append a line with the word two to a.txt, create the file c.txt containing the word sea, then run the shell command: git add -A && git commit -qm dev-change"}'`, `wait_done 1 7`, `N=...` as above. `snap` now shows a new HEAD, the commit `dev-change`, `c.txt: sea`, and `a.txt` with two lines. `$H api POST /turns/$N/rollback` is 204. `snap > $E/rb-after.txt; diff $E/rb-before.txt $E/rb-after.txt` prints nothing: HEAD, status (with `st.txt` staged again), log, index and file contents are back, and `c.txt` is gone (`rollback-restores`).
- **Note and errors.** `$H api GET '/messages?space=1'` ends with a `reef` message `Rolled back to before turn <N>.`. `$H api POST /turns/<tester turn>/rollback` is 400 `invalid: turn <id> has no checkpoint`; `$H api POST /turns/999/rollback` is 404 `turn 999: not found`.
- **Busy.** Save `git -C $T rev-parse HEAD; git -C $T status --porcelain` to `$E/busy-before.txt`. Send the 600 prompt to Space 1, `sleep 2`, then `$H api POST /turns/$N/rollback` is 409 `space 1 has a turn running: conflict` (`rollback-busy`). After `wait_done 1 8` the same two commands match `$E/busy-before.txt`, and Space 1 still has one `reef` message.
- **Next wake.** `$H sql "select wake_text, session_id from turn where id = <that last turn>"`: the wake is `[Space 1, message M]` and the user's text, with no `reef` line, since `reef` notes are added when the prompt is sent. In that session's file the last prompt is `[Space 1, message M]`, `reef wrote: Rolled back to before turn <N>.`, a blank line, then the user's text: `grep -o 'reef wrote: Rolled back to before turn <N>[.]' <session file> | head -1` prints it (`rollback-note`).

Quick session, file isolation and a project without git:

- **Quick session.** `$H api POST /spaces/4/send '{"text":"@tester Reply with one word."}'`, `wait_done 4 1`. The tester's session has only `"cwd":"$S/cwd"` (`tree-quick`).
- **Isolated.** `ls $S/full/worktrees/1/1/d.txt $S/full/worktrees/1/2/d.txt $S/cwd/d.txt` lists only the first; `git -C $S/cwd status --porcelain` is empty (`tree-isolated`).
- **Not git.** `$H api POST /spaces/5/send '{"text":"Reply with one word."}'` is 202. `wait_for '/turns?space=5&status=failed' lead`. `$H api GET '/turns?space=5'` is one `lead` turn, `failed`, `"error":"work items need the project folder to be a git repo with a commit: $S/plain: git rev-parse --verify HEAD in $S/plain: exit status 128: fatal: not a git repository ..."`. `grep 'turn of lead in space 5 failed' $E/serve.log` prints that error, and `$H api GET /health` is 200 (`tree-not-git`).

Attach to a work item with a dirty main checkout (Space 6, `solo`, whose lead has `write`):

- **Note and move.** `$H api POST /spaces '{"projectId":1,"name":"promo"}'` (id 6). `echo dirty > $S/cwd/dirty.txt`. `$H api POST /spaces/6/send '{"text":"Reply with one word."}'`, `wait_done 6 1`. `$H api POST /work-items '{"projectId":1,"title":"C"}'` (4), `$H api POST /spaces/6/attach '{"workItemId":4}'` is 200. `$H api GET '/messages?space=6'` has a `reef` message `This Space is now attached to work item "C" (4). It works in $S/full/worktrees/1/4 on work-item/4. Uncommitted changes stay on the main checkout.`. `$H api POST /spaces/6/send '{"text":"Reply with one word."}'`, `wait_done 6 2`. The Space has 2 lead turns that share one session; the session file has `"cwd":"$S/cwd"` and `"cwd":"$S/full/worktrees/1/4"`, `git -C $S/cwd worktree list` has `$S/full/worktrees/1/4 [work-item/4]`, and `git -C $S/cwd status --porcelain` is only `?? dirty.txt` (`tree-attach-dirty`).

Retry after a crash, Space 3:

- **Retry.** `$H api POST /spaces/3/send '{"text":"@developer Write the numbers from 1 to 600 separated by spaces. Use no tools."}'`, `sleep 3`, `$H serve-stop --kill`, `ANTHROPIC_MODEL=sonnet $H serve-start --data-name full`, set `A`. `F=$($H sql "select max(id) from turn where space_id = 3" | tail -1)`. `$H api GET /turns/$F` is `failed`, `"error":"server restarted"`, with its `wakeText` and `"retried":false`; Space 3 has a `reef` message `Turn <F> by developer failed: server restarted. The worktree may be dirty; retry or roll back.`. `$H api POST /turns/$F/retry` is 202, `wait_done 3 2`. `$H sql "select id, role, status, retried from turn where space_id = 3 order by id"` ends `<F>|developer|failed|1` and a new `developer|done|0` (`retry-restart`).
- **Twice.** `$H api POST /turns/$F/retry` again is 409 `turn <F> cannot be retried: conflict`, and so is `$H api POST /turns/1/retry` (a `done` turn) (`retry-twice`).

Inbox (Spaces 7 to 9 are quick sessions, so they make no worktree):

- **Stopped is not an item.** `$H api POST /spaces '{"projectId":1,"name":"relay","team":"relay"}'` (7), `... "name":"budget","team":"crew"` (8), `... "name":"crashed","team":"crew"` (9). `$H api POST /spaces/9/send '{"text":"Write the numbers from 1 to 600 separated by spaces."}'`, `sleep 3`, `$H api POST /spaces/9/stop` (204). Its turn is `failed`, `interrupted`, and `$H api GET /inbox` has no item of Space 9: a turn the user stopped is not one.
- **Failed by a crash.** The same send to 9 again, `sleep 3`, `$H serve-stop --kill`, `ANTHROPIC_MODEL=sonnet $H serve-start --data-name full`, set `A`. `C=$($H sql "select max(id) from turn where space_id = 9" | tail -1)`.
- **Stream.** Start it now: `curl -sN --max-time 400 http://$A/inbox/events > $E/inbox-events.txt &`.
- **Three kinds.** Send to Space 8, wait for it, set its token budget to 1, then send again. `$H api GET /inbox` has the crash-failed item for Space 9, the budget pause for Space 8, and `waiting` items such as Space 4 (`inbox-kinds`).
- **Clear.** Continue Space 8, retry Space 9's failed turn and send to waiting Space 4. Each item clears while its turn runs, then each Space ends with a new `waiting` item (`inbox-clear`).
- **Frames.** The inbox event stream shows failed Space 9, paused Space 8 and the waiting items disappear and return as work runs (`inbox-stream`).
- **Restart.** Re-create the budget pause, save `GET /inbox`, restart, and compare the response byte for byte. Paused, failed and waiting items survive (`inbox-restart`).

Codex on the test fixture (`write-codex-readonly`), folder `cx`. A shim logs what the server sends to the fixture `agent/codex/testdata/codex`:

- **Setup.** `$H serve-stop`. `mkdir -p $S/spy && printf '#!/bin/sh\ntee -a %s/codex-in.log | %s/agent/codex/testdata/codex "$@"\n' "$E" "$PWD" > $S/spy/codex && chmod +x $S/spy/codex`. `$H reef --data-name cx -- team apply $E/cx.json`. `FAKE_CODEX=replay PATH=$S/spy:$PATH ANTHROPIC_MODEL=sonnet $H serve-start --data-name cx`, set `A`. `$H api POST /projects '{"name":"cx","path":"'$S'/cwd"}'`, `$H api POST /spaces '{"projectId":1,"name":"cx room","team":"cx"}'`.
- **Sandbox.** `$H api POST /spaces/1/send '{"text":"@tester hi"}'`, `wait_done 1 1`, `$H api POST /spaces/1/send '{"text":"@developer hi"}'`, `wait_done 1 2`. `$H sql "select id, role, backend, status from turn"` is `1|tester|codex|done`, `2|developer|codex|done`. `grep thread/start $E/codex-in.log | grep -o '"sandbox":"[a-z-]*"'` prints `"sandbox":"read-only"` (the tester, whose file says `workspace-write`), then `"sandbox":"workspace-write"` (the developer).

Window (`$H ui`, with `REEF_VERIFY_CHROME` on Linux, see `window.md`):

- `$H serve-stop`, `ANTHROPIC_MODEL=sonnet $H serve-start --data-name ui2`, `$H ui rollback`, then `$H ui retry` on the same server: every line `PASS` (`window-rollback`, `window-retry`). Open the screenshots `rollback-checkpoints`, `rollback-rolled-back`, `retry-failed-row`, `retry-retried`.

Google login end to end (`google-login`), folder `google`, real claude on sonnet. A throwaway Go app with an email login page and one test is the project:

- **App.** In `$S/google-app`: `go.mod` (`module example.com/shop`, `go 1.22`), `main.go` with `routes()` serving `GET /login` (a form with email, password and `Sign in`) and `POST /login`, and `main_test.go` with `TestLoginPage`. `(cd $S/google-app && go test ./...)` passes. `git -C $S/google-app init -q -b main && git -C $S/google-app add -A && git -C $S/google-app -c user.name=v -c user.email=v@example.com commit -qm "shop with email login"`.
- **Setup.** `$H serve-stop`, `$H reef --data-name google -- team apply $PWD/teams/full.json`, `ANTHROPIC_MODEL=sonnet $H serve-start --data-name google`, set `A`. `$H api POST /projects '{"name":"shop","path":"'$S'/google-app"}'`, `$H api POST /work-items '{"projectId":1,"title":"Google login"}'`, `$H api POST /spaces '{"projectId":1,"workItemId":1,"name":"google","team":"full"}'`. Record both streams: `curl -sN --max-time 1800 http://$A/inbox/events > $E/google-inbox.txt &` and `curl -sN --max-time 1800 http://$A/spaces/1/events > $E/google-events.txt &`. `idle() { for i in $(seq 120); do [ "$(curl -s "http://$A/turns?space=1&status=running")" = "[]" ] && { sleep 4; [ "$(curl -s "http://$A/turns?space=1&status=running")" = "[]" ] && return; }; sleep 5; done; echo "timeout"; }`.
- **Ask.** Send the Google login request, then `wait_done 1 5`. `$H sql "select id, role, status from turn where space_id=1 order by id"` is lead, architect, developer, tester, lead, all done. The approval table has no rows.
- **Proof.** The developer turn has a checkpoint and one commit on `work-item/1`; the tester turn is done; the main checkout is clean and unchanged. The inbox ends with one `waiting` item, and the Space event stream has no approval event.

Handoffs, notes, retry and rollback rules, folder `fu`, on the same scratch cwd:

- **Setup.** Apply `full` and `pair`, where the pair lead follows an instruction to write a handoff mention. Create work items and Spaces `fa1`, `fb1` and `fc1`, and a free Space `fa2`. `P` asks the lead to output `@developer reply with one word, use no tools.`
- **Automatic handoff.** Run one developer turn in Space 1, then send `$P`. `wait_done 1 3`; the turns are developer, lead, developer, all done, and the approval table has no rows (`handoff-no-approval`).
- **Note on a retry, once.** `$H api POST /spaces/1/send '{"text":"@developer Write the numbers from 1 to 600 separated by spaces. Use no tools."}'`, `sleep 4`, `$H serve-stop --kill`, `ANTHROPIC_MODEL=sonnet $H serve-start --data-name fu`, set `A`, `F=<newest turn of space 1>`. `$H api GET /inbox` has `"kind":"failed","spaceId":1,...,"text":"developer failed: server restarted",...,"turnId":<F>`. `$H api POST /turns/$F/rollback` 204, `$H api POST /turns/$F/retry` 202, `wait_done 1 4`. `prompt <session>` has `reef wrote: Turn <F> by developer failed: server restarted. The worktree may be dirty; retry or roll back.\nreef wrote: Rolled back to before turn <F>.` before the text. Then `$H api POST /spaces/1/send '{"text":"@developer Reply with one word. Use no tools."}'`, `wait_done 1 5`: its `prompt` has no `reef wrote:` (`note-retry`).
- **Superseded.** The 600 send to Space 1 again, `sleep 4`, `$H serve-stop --kill`, restart, set `A`, `G=<newest turn of space 1>`. Send `@developer Reply with one word. Use no tools.`, `wait_done 1 6`. `$H api POST /turns/$G/retry` is 409 `turn <G> cannot be retried: conflict`, `$H api GET '/turns?space=1&retryable=1'` is `[]` and `$H api GET /inbox` has no `failed` item of Space 1 (`retry-superseded`).
- **Failed wrap-up.** `$H api POST /spaces/1/wrap` 202, `sleep 4`, `$H serve-stop --kill`, restart, set `A`. `$H sql "select role, status, error, wake_text is null from turn where id = <newest of space 1>"` is `lead|failed|server restarted|1`. Its retry is 409 `turn <id> cannot be retried: conflict`; `?retryable=1` is `[]` and the inbox has no item of Space 1 (`retry-wrap`).
- **Moved tree.** `$H api POST /spaces '{"projectId":1,"name":"q"}'` (5), send `Reply with one word.`, `wait_done 5 1`, `Q=<that turn>`, whose `checkpoint` is `refs/reef/checkpoints/<D>/<Q>`. `$H api POST /work-items '{"projectId":1,"title":"FQ"}'` (4), `$H api POST /spaces/5/attach '{"workItemId":4}'`, send `Reply with one word.` again, `wait_done 5 2`. Save `git -C $S/fu/worktrees/1/4 rev-parse HEAD; git -C $S/fu/worktrees/1/4 status --porcelain; ls -a $S/fu/worktrees/1/4`. `$H api POST /turns/$Q/rollback` is 409 `turn <Q> was checkpointed in $S/cwd, not $S/fu/worktrees/1/4: conflict`, and the same three commands match (`rollback-moved`).
- **Branch switch.** `H0=$(git -C $WA rev-parse HEAD)`. `$H api POST /spaces/1/send '{"text":"@developer Run this one shell command and nothing else: git checkout -q -b side && echo s > s.txt && git add s.txt && git commit -qm side"}'`, `wait_done 1 7`, `D=<that turn>`; `git -C $WA branch --show-current` is `side`. `$H api POST /turns/$D/rollback` 204: `git -C $WA branch --show-current` is `work-item/1` and `git -C $WA rev-parse HEAD work-item/1` prints `$H0` twice (`rollback-branch`).
- **Open in reef chat.** `$H api POST /work-items '{"projectId":1,"title":"FD"}'`, `$H api POST /spaces '{"projectId":1,"workItemId":<FD>,"name":"solo fa"}'` (`solo`), send `Reply with one word.`, `wait_done <that Space> 1`, `C=<that turn>` (it has a checkpoint). The server holds a Space it runs, so `$H api POST /spaces/<that Space>/stop` first. `(sleep 30 | ANTHROPIC_MODEL=sonnet $S/bin/reef chat --space <that Space> --data $S/fu > $E/chat-open.txt 2>&1 &)`, `sleep 8`, then `$H api POST /turns/$C/rollback` is 409 `space <that Space> is open elsewhere: conflict`. The chat ends by itself when its stdin closes (`rollback-chat`).
- **No commit yet.** `mkdir -p $S/fresh && git -C $S/fresh init -q -b main`. Project `{"name":"fresh","path":"'$S'/fresh"}` (2), Space `{"projectId":2,"name":"fresh q"}` (7), send `Reply with one word.`, `wait_done 7 1`: `$H api GET '/turns?space=7'` has `"status":"done"` and `"checkpoint":null` (`checkpoint-unborn`).
- **Worktree deleted.** `rm -rf $S/fu/worktrees/1/2`. `$H api POST /spaces/3/send '{"text":"@developer Reply with one word. Use no tools."}'`, `wait_done 3 2`: the `developer` turn is `done` with a checkpoint, `ls $S/fu/worktrees/1/2` lists `a.txt`, and `git -C $S/cwd worktree list` has `.../fu/worktrees/1/2 [work-item/2]` (`tree-recreate`).
- **Branch taken.** `$S/p157`: `git init -q -b main`, one commit, . Project `p157` (3), work item `{"projectId":3,"title":"X"}` (5), then `git -C $S/p157 checkout -q -b work-item/5`, Space `{"projectId":3,"workItemId":5,"name":"x1"}` (8), send `Reply with one word.`, `wait_for '/turns?space=8&status=failed' lead`: the error is `branch work-item/x is checked out at $S/p157; switch that checkout to another branch` (`tree-branch-taken`).
- **Attach on a clean main.** `$S/p157c`: `git init -q -b main` and one commit. Project `p157c`, Space `{"projectId":<p157c>,"name":"clean q"}`, send `Reply with one word.`, `wait_done <that Space> 1`; `git -C $S/p157c status --porcelain` is empty. Create work item `PC` in that project and `$H api POST /spaces/<that Space>/attach '{"workItemId":<PC>}'`. The `reef` message is exactly `This Space is now attached to work item "PC" (<PC>). It works in $S/fu/worktrees/<p157c>/<PC> on work-item/<PC>.` (`attach-clean`).
- **Settings allow Edit.** `$S/p153`: `git init -q -b main`, `a.txt` with `one`, one commit, then an untracked `.claude/settings.json` with `{"permissions":{"allow":["Edit","Write","Edit(*)","Write(*)"]}}`. Project `p153` (5), Space `{"projectId":5,"name":"settings q","team":"pair"}` (10). `$H api POST /spaces/10/send '{"text":"@tester Use the Edit tool to replace the word one with nope in a.txt. Say what happened."}'`, `wait_done 10 1`. `cat $S/p153/a.txt` is `one`, `git -C $S/p153 status --porcelain` is only `?? .claude/`, and the tester's session file has an `"is_error":true` tool result (`write-settings`).
- **One active Space.** Work item `K` with Space `k1` (team `pair`), then `$H api POST /spaces '{"projectId":1,"workItemId":<K>,"name":"k2"}'` is 409 `{"error":"conflict: work item <K> already has an active Space"}`, and `$H sql "select count(*) from space where work_item_id = <K>"` is `1` (`space-active-one`).
- **Attach shows on the item.** A free Space `free k` and work item `K2`: `$H api POST /spaces/<free k>/attach '{"workItemId":<K2>}'` is 200, and `$H api GET '/spaces?workItem=<K2>'` has the Space with `"kind":"work_item"` (`space-attach-list`).
- **Members are copied.** `$H api PUT /teams/frozen '{"name":"frozen","roles":[{"name":"lead","lead":true,"backend":"claude"},{"name":"dev","backend":"claude","write":true,"tools":["Read","Edit"]}]}'`, a Space `frozen room` of team `frozen`, then the same PUT with `dev` having no `write` and `"tools":["Read"]`. `$H api GET /spaces/<frozen room>/members` still has `dev` with `"write":true` and `"tools":["Read","Edit"]`; a Space `later room` made after the edit has `"write":false` and `"tools":["Read"]` (`space-members-frozen`).

When done: `$H cleanup`, then remove claude's state for the worktrees and for `google-app` (claude keeps a `memory` folder under the key of a worktree's main checkout): `rm -rf ~/.claude/projects/$(printf %s "$S" | sed 's/[^A-Za-z0-9]/-/g')-*`.

## Gotchas

- The model decides the words, so check structure: `turn.role`, `turn.checkpoint`, git state and files. The `crew` personas keep turns short and tool-free unless asked.
- Only an `@name` at the start of a message or of a line wakes that teammate (decision 80); "I'll hand it to @developer" in a sentence wakes nobody. `teams/full.json` tells each role to start a line with `@` only for the teammate it hands work to; if a run still takes another path, start again on a fresh repo and folder.
- The Google flow uses 4 agent hops (architect, developer, tester, lead) of the 6 the cap allows. A longer back and forth reaches the hop cap and pauses the Space with hold rows.
- The single-writer lock and the checkpoints are per process. `pgrep -P $(cat $S/serve.pid) -x claude` lists only this server's agents.
- `serve-stop --kill` is a real crash: the running claude is a child in its own process group and is ended by `cleanup` if it outlives the server.
- A plain `Bash` in a role's `tools` lets a role change files through the shell (`echo x > f` works), so `reef team apply` rejects it for a claude role without `write`. The `crew` tester lists only `Bash(ls:*)`.
- A background `curl` inside a command whose output is piped keeps the pipe open until `--max-time`; run the stream in its own call or redirect its stderr too.
- Turn and message ids are global to a data folder; the steps name them by query when other Spaces wrote before.
