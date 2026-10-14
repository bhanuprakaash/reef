# Full team

A user applies `teams/full.json` (lead, architect, developer, tester) and asks the lead for a
feature. Only a role with `write` changes files. Each feature works in its own git worktree on
`feature/<slug>`, and one write turn at a time runs on a tree. Before each write turn the
server takes a checkpoint, and the user can roll the tree back to it. A failed turn can be
retried. Everything that waits on the user (an approval, a pause, a failed turn, an agent's
last word) shows in one inbox, live, in the server and in the window. Contract:
`docs/server.md` (role, turn, Channels live, Inbox) and `docs/memory.md` (Teams).

## Sub-features

One line per acceptance line of the full-team slice and its sub-issues.

- `write-claude-refused` a tester without `write`, asked to create a file with Write and then Bash, is refused both; the tree is unchanged; its `claude` runs with `--permission-mode default` and without `Edit` and `Write` in `--allowedTools`.
- `write-codex-readonly` on codex (test fixture), a role without `write` starts its thread with `"sandbox":"read-only"`, even when its `permissionMode` is `workspace-write`; a role with `write` gets `workspace-write`.
- `write-claude-allowed` a developer with `write`, asked the same, creates the file.
- `write-default` a team file with no `write` field loads with `write: false` for every role; the seeded `solo` lead has `write`.
- `full-apply` `reef team apply teams/full.json`, then `reef team list` shows 4 roles, lead starred.
- `full-rest` `GET /teams/full` shows developer `write: true`, the others `false`, and no `gates` key.
- `tree-two-features` one wake on feature `a` and one on `b` make two worktrees on `feature/a` and `feature/b`; each teammate's claude session has its own tree as `cwd`.
- `tree-reuse` a second channel of feature `a` runs in the same tree: `git worktree list` is unchanged and a file put there before stays.
- `tree-quick` a quick session's teammate runs in the project path.
- `tree-isolated` a file a developer wrote in `a`'s tree is absent from `b`'s and from the main checkout.
- `tree-not-git` a feature of a project that is not a git repo: one `failed` turn with git's error, a log line, and `GET /health` still 200.
- `tree-promote-dirty` a quick session promoted while the main checkout has an untracked file: `reef` posts where the feature works, the next wake resumes the same session in the worktree, and the main checkout is still dirty.
- `writer-serial` two channels of one feature both wake the developer: the second turn starts after the first ends.
- `writer-reader` a tester wake in channel 2 runs while channel 1's developer turn runs.
- `writer-parallel` developers of two features run at the same time.
- `checkpoint-untracked` a developer turn has `checkpoint` `refs/reef/checkpoints/<turn id>`; `git show` of it holds an untracked file present before the turn, and its second parent holds what was staged.
- `checkpoint-untouched` HEAD, `git status` and the index are the same before and after a developer turn that uses no tools.
- `checkpoint-reader` a tester turn has `"checkpoint":null`.
- `checkpoint-upgrade` an old database opens and its turns have no checkpoint (`store/upgrade_test.go`).
- `rollback-restores` a developer edits a file, creates one and commits; rollback brings back HEAD, `git status`, `git log`, the index and every file of the checkpoint.
- `rollback-busy` rollback while the channel runs a turn is 409, and while another channel's write turn holds the tree is 409; nothing changes.
- `rollback-note` `reef` posts `Rolled back to before turn N.`, and the next wake of the developer starts with `reef wrote: Rolled back to before turn N.`; a turn with no checkpoint is 400, an unknown one 404.
- `retry-restart` a developer turn failed by a server crash (`server restarted`) is retried: a new developer turn ends `done` and the failed one is `retried`.
- `retry-twice` retrying it again, or retrying a `done` turn, is 409.
- `inbox-kinds` `GET /inbox` lists an `approval`, a `paused`, a `failed` and a `waiting` item.
- `inbox-clear` approve, continue, retry and send each clear their item, and the channel runs on.
- `inbox-stream` `GET /inbox/events` sends a frame when an item appears and when it clears, across four channels.
- `inbox-restart` after a restart `GET /inbox` lists the same items, byte for byte.
- `window-retry` Retry on a failed turn row runs the turn again (`$H ui retry`).
- `window-rollback` Rollback with confirm restores the tree (`$H ui rollback`).
- `window-inbox` a new approval in a channel that is not open shows in the inbox and fires one notification; Approve from the inbox runs the wake and the item goes; clicking the item or the notification opens its channel (`$H ui inbox`).
- `google-login` the spec's flow on a throwaway Go app with `teams/full.json`: lead, then architect, architect to developer held, approve, developer commits in the worktree, developer to tester held, approve, tester runs `go test` and reports; the main checkout is untouched and the inbox lists each approval.

## How to get to it (user POV)

- `reef team apply teams/full.json`, then in the window pick the team `full` in the New channel form of a feature. The project must be a git repo with a commit.
- Approve or Reject a held wake in the channel, or from the Inbox link. Retry and Rollback are buttons on the failed turn row and under Checkpoints.
- REST: `POST /turns/{id}/retry`, `POST /turns/{id}/rollback`, `GET /inbox`, `GET /inbox/events`.

## Driving it with verify-reef

Preconditions:

- Baseline from the README. `ANTHROPIC_MODEL=sonnet` on every `serve-start`. Codex is not needed: the codex line uses the repo's test fixture, so `doctor` showing `FAIL codex-auth` does not matter here.
- In every Bash call set `H=.claude/skills/verify-reef/scripts/verify-reef`, `REEF_VERIFY_RUN=<id>`, `E=~/.reef-verify/<id>`, `S=$(cd /tmp && pwd -P)/reef-verify/<id>` and, once a server runs, `A=$(cat $S/serve.addr)`. Paste `wait_done` and `wait_for` from `teams.md`.
- `$H reef` runs in the scratch cwd, so a repo file is named by its full path: `$PWD/teams/full.json`.
- The scratch cwd is the project and must be a git repo with one commit:

```sh
git -C $S/cwd init -q -b main && echo one > $S/cwd/a.txt && git -C $S/cwd add a.txt && git -C $S/cwd -c user.name=v -c user.email=v@example.com commit -qm init
```

- Team files in `$E`. `crew` has a developer with `write` and a tester without it whose `tools` list `Edit` and `Write` and whose mode is `acceptEdits` (both must be dropped); `cx` puts both on codex; `relay` gates lead to tester:

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
  ],
  "gates": [{"from": "lead", "to": "tester", "on": true}]
}
EOF
```

Teams, no server needed (data folder `full`):

- **Apply full.** `$H reef --data-name full -- team apply $PWD/teams/full.json` prints `applied team full: lead*(claude) architect(claude) developer(claude) tester(claude)`. Then `$H reef -- team apply $E/crew.json`, `$H reef -- team apply $E/cx.json`, `$H reef -- team apply $E/relay.json`. `$H reef -- team list` has the line `full  lead*(claude) architect(claude) developer(claude) tester(claude)` (`full-apply`).
- **Write column.** `$H sql "select team.name, role.name, lead, write from role join team on team.id = team_id order by team.name, role.id"` has `crew|lead|1|0`, `crew|developer|0|1`, `crew|tester|0|0`, `full|lead|1|0`, `full|architect|0|0`, `full|developer|0|1`, `full|tester|0|0`, `relay|lead|1|0`, `relay|tester|0|0` and `solo|lead|1|1`: files with no `write` field load `0` (`write-default`).

Server (the same folder): `ANTHROPIC_MODEL=sonnet $H serve-start --data-name full`, set `A`.

- **REST.** `$H api GET /teams/full` is 200; its roles have `"write":false` for lead, architect and tester and `"write":true` for developer, and the body has no `gates` key (`full-rest`).
- **Setup.** `mkdir -p $S/plain`. `$H api POST /projects '{"name":"full","path":"'$S'/cwd"}'` (id 1), `$H api POST /projects '{"name":"plain","path":"'$S'/plain"}'` (id 2, not a git repo). Features: `{"projectId":1,"slug":"a","name":"A"}` (1), `{"projectId":1,"slug":"b","name":"B"}` (2), `{"projectId":2,"slug":"ng","name":"No git"}` (3). Channels: `{"projectId":1,"featureId":1,"name":"a1","team":"crew"}` (1), `{"projectId":1,"featureId":1,"name":"a2","team":"crew"}` (2), `{"projectId":1,"featureId":2,"name":"b1","team":"crew"}` (3), `{"projectId":1,"name":"quick","team":"crew"}` (4, a quick session), `{"projectId":2,"featureId":3,"name":"nogit"}` (5, `solo`). In every channel a first message `@developer ...` wakes only the developer: the lead never starts.

Worktrees and writers. `T=$S/full/worktrees/1/a`. A claude session's folder is `~/.claude/projects/*/<session id>.jsonl`; `grep -ho '"cwd":"[^"]*"' <that file> | sort -u` shows where it ran.

- **Two features at once.** `$H api POST /channels/1/send '{"text":"@developer Write the numbers from 1 to 300 separated by spaces. Use no tools."}'` and the same to `/channels/3/send`, both 202, then `wait_done 1 1; wait_done 3 1`. `git -C $S/cwd worktree list` has three lines: `$S/cwd [main]`, `$S/full/worktrees/1/a [feature/a]`, `$S/full/worktrees/1/b [feature/b]`; `git -C $S/cwd status --porcelain` is empty (`tree-two-features`). `$H sql "select id, channel_id, role, started_at, ended_at, checkpoint from turn order by id"`: turns 1 (channel 1) and 2 (channel 3) are `developer`, each started before the other ended (`writer-parallel`), with checkpoints `refs/reef/checkpoints/1` and `/2`. The session of turn 1 has only `"cwd":"$S/full/worktrees/1/a"`, of turn 2 only `.../1/b`.
- **Same feature, two channels.** `echo mark > $T/marker.txt`. Send `@developer Write the numbers from 1 to 600 separated by spaces. Use no tools.` to channel 1 and at once to channel 2, then `wait_done 1 2; wait_done 2 1`. Of the two new developer turns, the second's `started_at` is at or after the first's `ended_at` (`writer-serial`). `git -C $S/cwd worktree list` is unchanged and `cat $T/marker.txt` prints `mark` (`tree-reuse`).
- **Tester refused, while the developer runs.** `git -C $T status --porcelain > $E/a-status-before.txt`. Send the 600 prompt to channel 1, `sleep 1`, then `$H api POST /channels/2/send '{"text":"@tester Use the Write tool to create the file t.txt containing the word nope. If that is refused, try once with Bash. Say what happened."}'`. After `sleep 4`, `for p in $(pgrep -P $(cat $S/serve.pid) -x claude); do ps -ww -o args= -p $p; done` shows the developer with `--permission-mode acceptEdits ... --allowedTools Read,Edit,Write,Bash,Task,mcp__reef__*` and the tester with `--permission-mode default ... --allowedTools Read,Bash(ls:*),mcp__reef__*`. `wait_done 1 3; wait_done 2 2`. The tester turn started before the developer turn ended and ended before it (`writer-reader`). `ls $T/t.txt` fails, `git -C $T status --porcelain | diff $E/a-status-before.txt -` prints nothing, the tester's message says both were refused, and its session file has 2 `"is_error":true` tool results (`write-claude-refused`). `$H api GET /turns/<tester turn>` has `"checkpoint":null` (`checkpoint-reader`).
- **Checkpoint.** `echo before > $T/pre.txt`, `echo staged > $T/st.txt && git -C $T add st.txt`. Save `git -C $T rev-parse HEAD`, `git -C $T status --porcelain` and `git -C $T ls-files --stage` to `$E/a-before.txt`. `$H api POST /channels/1/send '{"text":"@developer Reply with one word. Use no tools."}'`, `wait_done 1 4`, `N=$($H sql "select max(id) from turn where channel_id = 1" | tail -1)`. `$H api GET /turns/$N` has `"checkpoint":"refs/reef/checkpoints/<N>"`. The same three commands into `$E/a-after.txt`, and `diff $E/a-before.txt $E/a-after.txt` prints nothing (`checkpoint-untouched`). `git -C $T show refs/reef/checkpoints/$N:pre.txt` prints `before` and `git -C $T show refs/reef/checkpoints/$N^2:st.txt` prints `staged` (`checkpoint-untracked`).
- **Developer writes.** `$H api POST /channels/1/send '{"text":"@developer Use the Write tool to create the file d.txt containing the word yes."}'`, `wait_done 1 5`. `cat $T/d.txt` prints `yes` (`write-claude-allowed`).
- **Old database.** `go test ./store -run Upgrade -v` passes: `TestOpen_UpgradesVersion1Database/turn` expects a turn with no checkpoint (`checkpoint-upgrade`).

Rollback, channel 1. `snap() { git -C $T rev-parse HEAD; git -C $T status --porcelain; git -C $T log --oneline; git -C $T ls-files --stage; for f in $(ls $T); do echo "$f: $(cat $T/$f)"; done; }`.

- **Restores.** `snap > $E/rb-before.txt` (HEAD is the first commit; `A  st.txt`, `?? d.txt`, `?? marker.txt`, `?? pre.txt`). `$H api POST /channels/1/send '{"text":"@developer Append a line with the word two to a.txt, create the file c.txt containing the word sea, then run the shell command: git add -A && git commit -qm dev-change"}'`, `wait_done 1 6`, `N=...` as above. `snap` now shows a new HEAD, the commit `dev-change`, `c.txt: sea`, and `a.txt` with two lines. `$H api POST /turns/$N/rollback` is 204. `snap > $E/rb-after.txt; diff $E/rb-before.txt $E/rb-after.txt` prints nothing: HEAD, status (with `st.txt` staged again), log, index and file contents are back, and `c.txt` is gone (`rollback-restores`).
- **Note and errors.** `$H api GET '/messages?channel=1'` ends with a `reef` message `Rolled back to before turn <N>.`. `$H api POST /turns/<tester turn>/rollback` is 400 `invalid: turn <id> has no checkpoint`; `$H api POST /turns/999/rollback` is 404 `turn 999: not found`.
- **Busy.** Save `git -C $T rev-parse HEAD; git -C $T status --porcelain` to `$E/busy-before.txt`. Send the 600 prompt to channel 1, `sleep 2`, then `$H api POST /turns/$N/rollback` is 409 `channel 1 has a turn running: conflict`, and the rollback of channel 2's developer turn (`$H sql "select id from turn where channel_id = 2 and role = 'developer' limit 1"`) is 409 `.../worktrees/1/a has a write turn running: conflict` (`rollback-busy`). After `wait_done 1 7` the same two commands match `$E/busy-before.txt`, and channel 1 still has one `reef` message.
- **Next wake.** `$H sql "select wake_text from turn where id = <that last turn>"` is `[channel 1, message M]`, `reef wrote: Rolled back to before turn <N>.`, a blank line, then the user's text; the developer's session file has that line too (`rollback-note`).

Quick session, file isolation and a project without git:

- **Quick session.** `$H api POST /channels/4/send '{"text":"@tester Reply with one word."}'`, `wait_done 4 1`. The tester's session has only `"cwd":"$S/cwd"` (`tree-quick`).
- **Isolated.** `ls $S/full/worktrees/1/a/d.txt $S/full/worktrees/1/b/d.txt $S/cwd/d.txt` lists only the first; `git -C $S/cwd status --porcelain` is empty (`tree-isolated`).
- **Not git.** `$H api POST /channels/5/send '{"text":"Reply with one word."}'` is 202. `wait_for '/turns?channel=5&status=failed' lead`. `$H api GET '/turns?channel=5'` is one `lead` turn, `failed`, `"error":"git rev-parse --verify --quiet refs/heads/feature/ng in $S/plain: exit status 128: fatal: not a git repository ..."`. `grep 'turn of lead in channel 5 failed' $E/serve.log` prints that error, and `$H api GET /health` is 200 (`tree-not-git`).

Promote with a dirty main checkout (channel 6, `solo`, whose lead has `write`):

- **Note and move.** `$H api POST /channels '{"projectId":1,"name":"promo"}'` (id 6). `echo dirty > $S/cwd/dirty.txt`. `$H api POST /channels/6/send '{"text":"Reply with one word."}'`, `wait_done 6 1`. `$H api POST /channels/6/promote '{"slug":"c","name":"C"}'` is 202, `wait_done 6 2`. `$H api GET '/messages?channel=6'` has, after the user's `/promote`, a `reef` message `Uncommitted changes stay on the main checkout. This feature works in $S/full/worktrees/1/c on feature/c.`. `$H api POST /channels/6/send '{"text":"Reply with one word."}'`, `wait_done 6 3`. `$H sql "select id, role, status, session_id, checkpoint from turn where channel_id = 6"`: three `lead` turns, all one session id. That session's file has `"cwd":"$S/cwd"` lines and, for the last turn, `"cwd":"$S/full/worktrees/1/c"`: the session resumed in the worktree. `git -C $S/cwd worktree list` now has `.../1/c [feature/c]`, and `git -C $S/cwd status --porcelain` is still `?? dirty.txt` (`tree-promote-dirty`).

Retry after a crash, channel 3:

- **Retry.** `$H api POST /channels/3/send '{"text":"@developer Write the numbers from 1 to 600 separated by spaces. Use no tools."}'`, `sleep 3`, `$H serve-stop --kill`, `ANTHROPIC_MODEL=sonnet $H serve-start --data-name full`, set `A`. `F=$($H sql "select max(id) from turn where channel_id = 3" | tail -1)`. `$H api GET /turns/$F` is `failed`, `"error":"server restarted"`, with its `wakeText` and `"retried":false`; channel 3 has a `reef` message `Turn <F> by developer failed: server restarted. The worktree may be dirty; retry or roll back.`. `$H api POST /turns/$F/retry` is 202, `wait_done 3 2`. `$H sql "select id, role, status, retried from turn where channel_id = 3 order by id"` ends `<F>|developer|failed|1` and a new `developer|done|0` (`retry-restart`).
- **Twice.** `$H api POST /turns/$F/retry` again is 409 `turn <F> is not failed, or retried is already true: conflict`, and so is `$H api POST /turns/1/retry` (a `done` turn) (`retry-twice`).

Inbox (channels 7 to 9 are quick sessions, so they make no worktree):

- **Stream.** Start it first: `curl -sN --max-time 400 http://$A/inbox/events > $E/inbox-events.txt &`.
- **Four kinds.** `$H api POST /channels '{"projectId":1,"name":"gate","team":"relay"}'` (7), `... "name":"budget","team":"crew"` (8), `... "name":"stopped","team":"crew"` (9). `$H api POST /channels/7/send '{"text":"go"}'`, `$H api POST /channels/8/send '{"text":"Reply with one word."}'`, `$H api POST /channels/9/send '{"text":"Write the numbers from 1 to 600 separated by spaces."}'`, `sleep 3`, `$H api POST /channels/9/stop` (204), `wait_done 7 1; wait_done 8 1`. `$H sql --write "update channel set token_budget = 1 where id = 8"`, then `$H api POST /channels/8/send '{"text":"Reply with one word."}'` is 409 `channel 8 is paused: token budget of 1 used: N tokens: conflict`. `$H api GET /inbox` has `{"kind":"failed","channelId":9,...,"text":"lead failed: interrupted",...,"turnId":<id>}`, `{"kind":"approval","channelId":7,...,"text":"lead wants to wake tester",...,"approvalId":1}`, `{"kind":"paused","channelId":8,...,"text":"token budget of 1 used: N tokens"}` and `waiting` items of the channels that ended on an agent's message, such as 4 (`inbox-kinds`).
- **Clear.** `$H api POST /channels/7/approvals/1 '{"approve":true}'`, `$H api POST /channels/8/continue`, `$H api POST /turns/<channel 9's failed turn>/retry` and `$H api POST /channels/4/send '{"text":"Reply with one other word."}'` (4 is `waiting`) are 204, 204, 202 and 202. At once `$H api GET /inbox` lists none of those four items. `wait_done 7 2; wait_done 9 1; wait_done 4 2`, then `$H api POST /channels/8/send '{"text":"Reply with one word."}'` is 202 and `wait_done 8 2`. Channel 7 has a `tester` turn `done`, 9 a new `lead` turn `done` (the failed one `retried` 1), 4 and 8 a new `lead` turn `done`: each channel ran on. Each of the four now lists a new `waiting` item (`inbox-clear`).
- **Frames.** After the stream ends, `grep '^data:' $E/inbox-events.txt | while read -r l; do echo "$l" | grep -o '"kind":"[a-z]*","channelId":[4789],' | tr '\n' ' '; echo; done` prints one line per frame: the `approval` of 7, the `failed` of 9 and the `paused` of 8 each appear in a new frame; later frames drop them one by one (8 turns `waiting` after `continue`), 4's `waiting` goes while its turn runs and comes back, and 7 and 9 come back as `waiting` (`inbox-stream`).
- **Restart.** Make the four kinds again: `$H api POST /channels/7/send '{"text":"go"}'` and `wait_for /inbox '"kind":"approval"'`; `$H sql --write "update channel set token_budget = 1 where id = 8"` and the send to 8 (409, paused); channel 5's `failed` turn is still listed. `$H api GET /inbox > $E/inbox-before.txt`, `$H serve-stop`, `ANTHROPIC_MODEL=sonnet $H serve-start --data-name full`, `$H api GET /inbox > $E/inbox-after.txt`. `diff $E/inbox-before.txt $E/inbox-after.txt` prints nothing, and `grep -o '"kind":"[a-z]*","channelId":[0-9]*' $E/inbox-after.txt` has `approval` 7, `paused` 8, `failed` 5 and `waiting` lines (`inbox-restart`).

Codex on the test fixture (`write-codex-readonly`), folder `cx`. A shim logs what the server sends to the fixture `agent/codex/testdata/codex`:

- **Setup.** `$H serve-stop`. `mkdir -p $S/spy && printf '#!/bin/sh\ntee -a %s/codex-in.log | %s/agent/codex/testdata/codex "$@"\n' "$E" "$PWD" > $S/spy/codex && chmod +x $S/spy/codex`. `$H reef --data-name cx -- team apply $E/cx.json`. `FAKE_CODEX=replay PATH=$S/spy:$PATH ANTHROPIC_MODEL=sonnet $H serve-start --data-name cx`, set `A`. `$H api POST /projects '{"name":"cx","path":"'$S'/cwd"}'`, `$H api POST /channels '{"projectId":1,"name":"cx room","team":"cx"}'`.
- **Sandbox.** `$H api POST /channels/1/send '{"text":"@tester hi"}'`, `wait_done 1 1`, `$H api POST /channels/1/send '{"text":"@developer hi"}'`, `wait_done 1 2`. `$H sql "select id, role, backend, status from turn"` is `1|tester|codex|done`, `2|developer|codex|done`. `grep thread/start $E/codex-in.log | grep -o '"sandbox":"[a-z-]*"'` prints `"sandbox":"read-only"` (the tester, whose file says `workspace-write`), then `"sandbox":"workspace-write"` (the developer).

Window (`$H ui`, with `REEF_VERIFY_CHROME` on Linux, see `window.md`). Apply `gated` (from `teams.md`) to each folder first:

- `$H serve-stop`, `$H reef --data-name inbox -- team apply $E/gated.json`, `ANTHROPIC_MODEL=sonnet $H serve-start --data-name inbox`, `$H ui inbox`: every line `PASS` (`window-inbox`). It reads the badge as `1`, so it needs a folder with no other items.
- `$H serve-stop`, `ANTHROPIC_MODEL=sonnet $H serve-start --data-name ui2`, `$H ui rollback`, then `$H ui retry` on the same server: every line `PASS` (`window-rollback`, `window-retry`). Open the screenshots `rollback-checkpoints`, `rollback-rolled-back`, `retry-failed-row`, `retry-retried`, `inbox-approval-row`, `inbox-approved`.

Google login end to end (`google-login`), folder `google`, real claude on sonnet. A throwaway Go app with an email login page and one test is the project:

- **App.** In `$S/google-app`: `go.mod` (`module example.com/shop`, `go 1.22`), `main.go` with `routes()` serving `GET /login` (a form with email, password and `Sign in`) and `POST /login`, and `main_test.go` with `TestLoginPage`. `(cd $S/google-app && go test ./...)` passes. `git -C $S/google-app init -q -b main && git -C $S/google-app add -A && git -C $S/google-app -c user.name=v -c user.email=v@example.com commit -qm "shop with email login"`.
- **Setup.** `$H serve-stop`, `$H reef --data-name google -- team apply $PWD/teams/full.json`, `ANTHROPIC_MODEL=sonnet $H serve-start --data-name google`, set `A`. `$H api POST /projects '{"name":"shop","path":"'$S'/google-app"}'`, `$H api POST /features '{"projectId":1,"slug":"google-login","name":"Google login"}'`, `$H api POST /channels '{"projectId":1,"featureId":1,"name":"google","team":"full"}'`. Record both streams: `curl -sN --max-time 1800 http://$A/inbox/events > $E/google-inbox.txt &` and `curl -sN --max-time 1800 http://$A/channels/1/events > $E/google-events.txt &`. `idle() { for i in $(seq 120); do [ "$(curl -s "http://$A/turns?channel=1&status=running")" = "[]" ] && { sleep 4; [ "$(curl -s "http://$A/turns?channel=1&status=running")" = "[]" ] && return; }; sleep 5; done; echo "timeout"; }`.
- **Ask.** `$H api POST /channels/1/send '{"text":"Add Google login to this app: a Sign in with Google link on the login page that goes to /auth/google, which redirects to the Google OAuth URL with the client id from the GOOGLE_CLIENT_ID env var. Keep it small and covered by a test."}'`, `idle`. Turns: `lead`, `architect`, both `done`, no checkpoint. `$H sql "select id, from_role, to_role, kind, status from approval"` is `1|architect|developer|gate|pending`, and `$H api GET /inbox` is that one `approval` item, `architect wants to wake developer`.
- **Approve the developer.** `$H api POST /channels/1/approvals/1 '{"approve":true}'`, `idle`. A `developer` turn `done` with `checkpoint` `refs/reef/checkpoints/3`; `git -C $S/google-app log --oneline main..feature/google-login` has one commit by the developer; `git -C $S/google-app worktree list` has `$S/google/worktrees/1/google-login [feature/google-login]`; `git -C $S/google-app status --porcelain` is empty and `main` is still the first commit. Approval 2 is `developer|tester|gate|pending` and the inbox lists it alone.
- **Approve the tester.** `$H api POST /channels/1/approvals/2 '{"approve":true}'`, `idle`. A `tester` turn `done` (no checkpoint) whose message reports `go test` results to `@lead`, then a `lead` turn `done` that reports to the user and mentions no one. `$H sql "select paused from channel"` is empty. `$H api GET /inbox` is one `waiting` item, the lead's report.
- **Proof.** `$H api GET '/messages?channel=1'`, `$H api GET '/turns?channel=1'`, `git -C $S/google-app log --stat main..feature/google-login`. `grep '^data:' $E/google-inbox.txt | cut -c1-120` shows `[]`, the first approval, `[]`, the second approval, `[]`, the `waiting` item. `grep -A1 '^event: approval' $E/google-events.txt` has each approval `pending`, then `approved`.

When done: `$H cleanup`, then remove claude's state for the worktrees and for `google-app` (claude keeps a `memory` folder under the key of a worktree's main checkout): `rm -rf ~/.claude/projects/$(printf %s "$S" | sed 's/[^A-Za-z0-9]/-/g')-*`.

## Gotchas

- The model decides the words, so check structure: `turn.role`, `turn.checkpoint`, `approval` rows, git state, files. The `crew` personas keep turns short and tool-free unless asked.
- Every `@name` in an agent's message wakes that teammate, and a gate holds only its own edge. In the Google run a lead that wrote "I'll hand it to @developer" woke the developer at once, past the architect-to-developer gate. `teams/full.json` tells each role to mention only the teammate it hands work to; if a run still takes another path, start again on a fresh repo and folder.
- The Google flow uses exactly 4 agent hops (architect, developer, tester, lead); approving a gate does not reset the hop count. One extra mention (an architect asking `@lead` to confirm) reaches the hop cap and the channel pauses with the holds in `approval` (kind `hold`).
- The single-writer lock and the checkpoints are per process. `pgrep -P $(cat $S/serve.pid) -x claude` lists only this server's agents.
- `serve-stop --kill` is a real crash: the running claude is a child in its own process group and is ended by `cleanup` if it outlives the server.
- A plain `Bash` in a role's `tools` lets a role without `write` change files through the shell (`echo x > f` works). The `crew` tester lists only `Bash(ls:*)`.
- A background `curl` inside a command whose output is piped keeps the pipe open until `--max-time`; run the stream in its own call or redirect its stderr too.
- Turn, message and approval ids are global to a data folder; the steps name them by query when other channels wrote before.
