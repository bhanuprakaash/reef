# Teams

A user defines a team of roles in a JSON file with `reef team apply`, opens a channel on that
team, and talks to its lead. The lead hands work to a specialist by an `@mention`. The server
wakes the specialist in its own agent session, which stops when its turn ends, and the reply
lands in a thread. The user replies in that thread and the thread's owner answers. Agents that
keep waking each other hit the hop cap and the channel pauses. A wake across a gate waits for
the user's Approve or Reject. Contract: `docs/server.md` (team, role, gate, approval, Channels
live, Caps) and `docs/memory.md` (Teams, reef mcp, the context prompt).

## Sub-features

- `team-apply` `reef team apply FILE.json` creates a team; `reef team list` shows each team with its roles and backends, the lead starred. The same name again replaces its roles: a changed persona updates the one team.
- `team-errors` a file with no lead or two leads, a role named `user` or with a space, a bad backend, an unknown field, a gate that names a missing role, a missing file, and no subcommand each fail with `reef: ...`, exit 1, and change nothing.
- `team-rest` `GET /teams` lists teams by name (`solo` always exists); `GET /teams/{name}` returns one with its roles and gates, 404 for an unknown name; the routes take no write; a role has no `mcpServers` key.
- `team-channel` `POST /channels` with `team` stores it and fills `role` (the lead's name), `backend` and `permissionMode` from the lead role; with no team the channel is in `solo`; an unknown team and a `role` other than `lead` are 400.
- `team-reapply-live` a team applied again with a new role while the lead is live: `@tester` runs a tester turn.
- `thread-send` `POST /channels/{id}/send` with `parent` stores the reply under that top-level message; a parent that is not a top-level message of the channel is 400; `role` is no longer a field (400).
- `thread-read` the `read_thread` MCP tool with `root` returns the root and its replies in order; a reply id as `root` is `not found`; in a quick session it reads the token's own channel.
- `lazy-lead` a first message `@architect ...` in a new channel starts only the architect: no lead turn, no lead token, one `claude` child while it runs and none after.
- `session-per-teammate` the lead's `claude` stays after its turn; a specialist's is gone when its wake ends; each has its own token, its own session id, the role's persona and skills and team line in its first prompt, and the role's `tools` as `--allowedTools`; a second wake resumes the same session.
- `route-lead-architect` the user asks the lead; the lead's reply `@architect ...` wakes the architect, which posts a top-level message it owns; the lead gets no turn after it.
- `route-thread-owner` a user reply with `parent` on the architect's message goes to the architect, not the lead, and the answer lands in the same thread.
- `route-default-lead` a user message with no mention and no thread goes to the lead.
- `post-to-channel` a message the lead posts with the `post_to_channel` tool wakes the architect after the lead's turn ends, even when the lead's reply names no one.
- `route-mentions` one message that mentions two roles wakes both; a turn that posts `@architect` and replies naming `@architect` runs one architect turn; an agent's reply in a thread, owned by the user or the lead, wakes nobody unless it mentions a role.
- `wake-ids` a wake's prompt starts with `[channel N, message N]` for a top-level message and `[channel N, message N, thread N]` for a thread reply, and `post_to_channel` with `parent` set to the header's thread id stores the post in the thread.
- `token-role` a teammate's tool calls carry its own role: `record_decision` by the lead token has author `lead`, by the architect's session author `architect`.
- `hop-cap` two teammates that keep mentioning each other run 4 hops, the 5th does not run, the channel is paused with `hop cap: 4 agent hops without a user message`; `continue` runs the held wake and counts from 0 again; a user send clears the pause, drops the held wake and counts from 0.
- `gate-default` the default team has no gate on lead to architect: the wake runs and `GET /channels/{id}/approvals` is `[]`.
- `gate-approve-reject` with the lead to architect gate on, the mention stores a pending approval and no architect turn; Approve runs the wake, Reject drops it, a decided approval is 409, and Approve during a turn is 409 and stays pending. The event stream carries `approval` frames.
- `wrap-wakes-nobody` a `/wrap` turn whose reply says `@architect` starts no architect turn.
- `restart-held` after a restart a pending approval is still pending and can be approved; a hop-cap pause stays, and `continue` clears it and runs nothing.
- `codex-teammate` a role on `codex` is woken and its reply is stored (driven with the codex test fixture, not a real codex).
- `window-teams` the window picks a team, folds replies under their root, opens a thread pane, names the running teammate and shows Approve and Reject (by hand, see below).

## How to get to it (user POV)

- Write a team file (see `docs/memory.md`), `reef team apply team.json`, then in the window choose the team in the New channel form, or `POST /channels` with `"team":"<name>"`.
- Talk to the lead with `POST /channels/{id}/send`. Reply in a thread with `"parent": <root id>`. Watch `GET /channels/{id}/events`. Approve a held wake with `POST /channels/{id}/approvals/{aid}` and `{"approve":true}`.

## Driving it with verify-reef

Preconditions:

- Baseline from the README. `ANTHROPIC_MODEL=sonnet` on every `serve-start`. The run is on `claude`; `codex` is not needed (the codex step uses the repo's test fixture), so `doctor` showing `FAIL codex-auth` does not matter here.
- In every Bash call set `H=.claude/skills/verify-reef/scripts/verify-reef`, `REEF_VERIFY_RUN=<id>`, `E=~/.reef-verify/<id>`, `S=$(cd /tmp && pwd -P)/reef-verify/<id>` and, once a server runs, `A=$(cat $S/serve.addr)` (the port changes after every `serve-start`).
- Two helpers to paste into a call that waits. `wait_done CH N` waits until channel CH has N done turns (N is the last turn of the chain, so a half-run chain does not match); `wait_for PATH TEXT` waits until `GET PATH` contains TEXT:

```sh
wait_done() { for i in $(seq 60); do [ "$(curl -s "http://$A/turns?channel=$1&status=done" | grep -o '"id"' | wc -l)" = "$2" ] && { sleep 1; return; }; sleep 3; done; echo "timeout: channel $1 never had $2 done turns"; return 1; }
wait_for() { for i in $(seq 60); do curl -s "http://$A$1" | grep -q "$2" && return; sleep 2; done; echo "timeout: $1 never had $2"; return 1; }
```

- Team files in `$E`. `duo.json` (a lead that mentions the architect only when the message says DESIGN, and an architect with a persona, a skill and one tool):

```sh
cat > $E/duo.json <<'EOF'
{
  "name": "duo",
  "roles": [
    {
      "name": "lead",
      "lead": true,
      "backend": "claude",
      "persona": "You are the lead. Answer in one short sentence. If the user's message contains the word DESIGN, reply instead with exactly one line: @architect followed by the user's request. If asked to wrap up the channel, call propose_wrap, then reply with exactly: Proposed. @architect FYI"
    },
    {
      "name": "architect",
      "backend": "claude",
      "persona": "You are the architect. Answer in one short sentence, and end it with the word PLANNED. Use no tools unless asked to. Never @mention anyone.",
      "skills": "Writing short plans.",
      "tools": ["Read"]
    }
  ]
}
EOF
cat > $E/pingpong.json <<'EOF'
{
  "name": "pingpong",
  "roles": [
    {"name": "lead", "lead": true, "backend": "claude",
     "persona": "You are the lead. Whatever you are told, reply with exactly one line: @architect ping"},
    {"name": "architect", "backend": "claude",
     "persona": "You are the architect. Whatever you are told, reply with exactly one line: @lead pong"}
  ]
}
EOF
sed 's/Writing short plans./Writing very short plans./' $E/duo.json > $E/duo2.json
sed 's/"name": "duo",/"name": "gated", "gates": [{"from": "lead", "to": "architect", "on": true}],/' $E/duo.json > $E/gated.json
sed -e 's/"name": "duo"/"name": "mixed"/' -e '/"name": "architect"/{n;s/"claude"/"codex", "permissionMode": "read-only"/}' $E/duo.json > $E/mixed.json
printf '{"name":"bad","roles":[{"name":"a","backend":"claude"}]}' > $E/nolead.json
printf '{"name":"bad","roles":[{"name":"a","lead":true,"backend":"claude"},{"name":"b","lead":true,"backend":"codex"}]}' > $E/twolead.json
printf '{"name":"bad","roles":[{"name":"a","lead":true,"backend":"gemini"}]}' > $E/badbackend.json
printf '{"name":"bad","roles":[{"name":"a","lead":true,"backend":"claude"},{"name":"user","backend":"claude"}]}' > $E/userrole.json
printf '{"name":"bad","roles":[{"name":"a","lead":true,"backend":"claude"},{"name":"two words","backend":"claude"}]}' > $E/spacerole.json
printf '{"name":"keys","roles":[{"name":"lead","lead":true,"backend":"claude","mcpServers":[{"command":"true","env":{"API_KEY":"sk-verify-5521"}}]}]}' > $E/keys.json
printf '{"name":"duo","roles":[{"name":"lead","lead":true,"backend":"claude","persona":"You are the lead. Answer in one short sentence."},{"name":"architect","backend":"claude"},{"name":"tester","backend":"claude","persona":"You are the tester. Answer in one short sentence, and end it with the word TESTED. Never @mention anyone."}]}' > $E/duo3.json
printf '{"name":"bad","roles":[{"name":"a","lead":true,"backend":"claude","color":"red"}]}' > $E/unknownfield.json
printf '{"name":"bad","roles":[{"name":"a","lead":true,"backend":"claude"}],"gates":[{"from":"a","to":"ghost","on":true}]}' > $E/badgate.json
```

Teams, no server needed (data folder `teams`):

- **Apply and list.** `$H reef --data-name teams -- team apply $E/duo.json` prints `applied team duo: lead*(claude) architect(claude)`. `$H reef -- team list` prints `duo  lead*(claude) architect(claude)` and `solo  lead*(claude)`. `$H sql "select team.name, role.name, lead from role join team on team.id = team_id order by team.name, role.id"` shows `duo|lead|1`, `duo|architect|0`, `solo|lead|1`.
- **Apply again.** `$H reef -- team apply $E/duo2.json` prints the same line. `$H sql "select count(*) from team"` is still `2`, and `$H sql "select skills from role where name = 'architect'"` is `Writing very short plans.`. Put the original back with `$H reef -- team apply $E/duo.json`.
- **Errors.** Each `$H reef -- team apply $E/<file>` ends `exit: 1` and `$H sql "select count(*) from team"` stays `2`:
  - `nolead.json`: `reef: invalid: team bad needs exactly one lead role, has 0`
  - `twolead.json`: `reef: invalid: team bad needs exactly one lead role, has 2`
  - `badbackend.json`: `reef: role a: invalid: backend must be claude or codex`
  - `userrole.json`: `reef: invalid: role name "user" must be letters, digits, _ or -, and not user`
  - `spacerole.json`: `reef: invalid: role name "two words" must be letters, digits, _ or -, and not user`
  - `unknownfield.json`: `reef: parse <path>: json: unknown field "color"`
  - `badgate.json`: `reef: invalid: gate a to ghost names a role team bad does not have`
  - `$H reef -- team apply /nonexistent.json`: `reef: read team file: open /nonexistent.json: no such file or directory`
  - `$H reef -- team`: `reef: usage: reef team apply FILE.json | list [--data DIR]`
- **Three more teams.** `$H reef -- team apply $E/pingpong.json`, `$H reef -- team apply $E/gated.json` and `$H reef -- team apply $E/keys.json` print `applied team ...`. `$H sql "select count(*) from role where mcp_servers like '%sk-verify-5521%'"` is `1`: the env is stored for the agent.

Server (the same folder): `ANTHROPIC_MODEL=sonnet $H serve-start --data-name teams`, then `A=$(cat $S/serve.addr)`.

- **REST.** `$H api GET /teams` is 200 with `duo`, `gated`, `keys`, `pingpong`, `solo` in that order; each role has `name`, `lead`, `backend`, `permissionMode`, `persona`, `skills`, and `tools`, and only `gated` has `"gates":[{"from":"lead","to":"architect","on":true}]`. `$H api GET /teams/gated` is 200 with that team. `$H api GET /teams/keys` is 200 and has no `mcpServers` key; neither it nor `$H api GET /teams` contains `mcpServers`, `sk-verify-5521` or `API_KEY`. `$H api GET /teams/nope` is 404 `team nope: not found`. `$H api POST /teams '{}'` is 405.
- **Setup.** `$H api POST /projects '{"name":"teams","path":"'$S'/cwd"}'`, `$H api POST /features '{"projectId":1,"slug":"login","name":"Login"}'`.
- **Channel on a team.** `$H api POST /channels '{"projectId":1,"featureId":1,"name":"plain","backend":"claude"}'` is 201 id 1 with `"team":"solo"`. `$H api POST /channels '{"projectId":1,"featureId":1,"name":"duo room","team":"duo"}'` is id 2 with `"team":"duo","role":"lead","backend":"claude"`: the backend came from the lead role, the request named none. Then three more, ids 3 to 5: `{"projectId":1,"featureId":1,"name":"lazy","team":"duo"}`, `{"projectId":1,"featureId":1,"name":"ping room","team":"pingpong"}`, `{"projectId":1,"featureId":1,"name":"gate room","team":"gated"}`. `... "team":"nope"` is 400 `invalid: unknown team nope`. `... "team":"duo","role":"architect"` is 400 `invalid: role must be lead`. `$H sql "select channel.id, channel.name, team.name from channel join team on team.id = team_id"` shows `1|plain|solo`, `2|duo room|duo`, `3|lazy|duo`, `4|ping room|pingpong`, `5|gate room|gated`.

Lazy lead, channel 3. `claude` processes of the server are `pgrep -P $(cat $S/serve.pid) -x claude`:

- **Only the architect starts.** `$H api POST /channels/3/send '{"text":"@architect Write the numbers from 1 to 300 separated by spaces. Remember the codeword kiwi-77."}'` is 202. At once (the turn lasts several seconds): `$H sql "select role from token"` is `architect` and no other row; `pgrep -P $(cat $S/serve.pid) -x claude | wc -l` is `1`; `ps -ww -o args= -p $(pgrep -P $(cat $S/serve.pid) -x claude)` shows `--allowedTools Read,mcp__reef__*` (the architect's `tools` plus the `reef` server). After `wait_done 3 1`: the `claude` count is `0`, `$H sql "select count(*) from token"` is `0`, and `$H sql "select role from turn where channel_id = 3"` is one `architect` row: the lead never started.

Channel 2, lead and architect:

- **Lead process stays, specialist process goes.** `$H api POST /channels/2/send '{"text":"Hello. Reply with one word."}'`, `wait_done 2 1`. The `claude` count is `1`, `$H sql "select role from token"` is `lead`. Then `$H api POST /channels/2/send '{"text":"DESIGN a login form. Remember the codeword kiwi-77."}'`, `wait_done 2 3`. The `claude` count is `1` and the token is still `lead` only: the architect's process and token are gone. `$H sql "select id, role, status from turn where channel_id = 2 order by id"` is `lead`, `lead`, `architect`, all `done`, and the last row is the architect: the lead got no turn after the architect posted.
- **Routing from the lead.** `$H api GET '/messages?channel=2'` has, in order, user `Hello...`, lead (one word), user `DESIGN a login form...`, lead `@architect DESIGN a login form...`, and architect with `"parentId":null` and a body ending `PLANNED` (its persona). `$H api GET /channels/2/approvals` is `[]` (`gate-default`).
- **Persona, skills and team line.** Take the architect's session id: `$H sql "select session_id from turn where role = 'architect' and channel_id = 2"`. With `K=$(printf %s "$S/cwd" | sed 's/[^A-Za-z0-9]/-/g')`, `grep -c "You are the architect" ~/.claude/projects/$K/<session id>.jsonl`, `grep -c "Your skills:" ...` and `grep -c "Your team: @lead, @architect" ...` are each at least 1. For the lead's session id the file has `Do not summarize or repeat a specialist's post back to the user`. Grep before `cleanup`, which removes that folder.
- **Token role.** `T=$($H sql "select token from token where role = 'lead'" | tail -1)`, then `REEF_TOKEN=$T $H mcp-call record_decision '{"body":"Lead check"}'` gives `"author":"lead"`.
- **Thread send and its errors.** The architect's message id is `ROOT=$($H sql "select id from message where channel_id = 2 and author = 'architect'" | tail -1)`. `$H api POST /channels/2/send '{"text":"x","parent":9999}'` is 400 `invalid: parent 9999 is not a top-level message of channel 2`. A parent from another channel is 400 the same way: `$H api POST /channels/2/send '{"text":"x","parent":1}'` (message 1 is in channel 3). `$H api POST /channels/2/send '{"text":"x","role":"architect"}'` is 400 `unknown field "role"`. `$H sql "select count(*) from message where channel_id = 2"` is still `5`.
- **Thread owner answers, in the thread, and resumes.** `$H api POST /channels/2/send '{"text":"What codeword did I give you? Answer with the codeword only. Also call the reef record_decision tool once with body Codeword is kiwi-77.","parent":'$ROOT'}'` is 202. `wait_done 2 4`. `$H api GET '/messages?channel=2&after='$ROOT` has the user message and an architect message, both `"parentId":<ROOT>`, the answer `kiwi-77`. `$H sql "select id, role, session_id from turn where channel_id = 2 order by id"`: the two `architect` turns have the same session id, so the second wake resumed it (it recalled the codeword), and the lead still has two turns. `$H sql "select id, author, body from decision"` has `1|lead|Lead check` and `2|architect|Codeword is kiwi-77`.
- **read_thread with a root.** `REEF_TOKEN=$T $H mcp-call read_thread '{"channelId":2,"root":'$ROOT'}'` gives `isError=false` and three messages, oldest first: the architect's root (`"parentId":null`), the user reply and the architect reply (`"parentId":<ROOT>`); `tools:` lists `post_to_channel` among eight tools. A reply id as root, `'{"channelId":2,"root":<reply id>}'`, gives `isError=true thread <id>: not found`.
- **post_to_channel wakes.** `$H api POST /channels/2/send '{"text":"Use the reef post_to_channel tool once to ask the architect, mentioning them, to state the codeword again. Then reply with just the word: posted"}'`, `wait_done 2 6`. Newest messages: user, lead `@architect ...` (posted by the tool, top level), lead `posted`, architect `...kiwi-77. PLANNED`. `$H sql "select id, role from turn where channel_id = 2 order by id desc limit 2"` is the architect, then the lead. The lead's reply names no one, so the wake came from the tool's message. The lead's session shows the tool: `grep -c mcp__reef__post_to_channel ~/.claude/projects/$K/<lead session id>.jsonl` is at least 1. The architect's session has the prompt line: `grep -c '\[channel 2, message' ~/.claude/projects/$K/<architect session id>.jsonl` is at least 1.
- **Wrap wakes nobody.** `$H sql "select role, count(*) from turn where channel_id = 2 group by role"` is `architect|3`, `lead|3`. `$H api POST /channels/2/send '{"text":"/wrap"}'`, `wait_done 2 7`. The same query is `architect|3`, `lead|4`. The newest lead message is `Proposed. @architect FYI`: its mention woke no one. `$H api GET /channels/2/proposal` is 200 `pending`.
- **No mention, no thread, goes to the lead.** The `Hello. Reply with one word.` message in the first step had none: its only turn is the lead's (`lead` turn 1 of channel 2, before any architect turn).

Hop cap, channel 4 (`pingpong`: the lead always says `@architect ping`, the architect `@lead pong`):

- **Pause after 4 hops.** `$H api POST /channels/4/send '{"text":"start"}'`, then `wait_for /channels/4 '"paused":"hop cap'`. `$H sql "select role, status from turn where channel_id = 4 order by id"` is five `done` rows: `lead`, `architect`, `lead`, `architect`, `lead` (the lead's first turn answers the user; the next four are hops, and the 5th hop did not run). `$H api GET /channels/4` has `"paused":"hop cap: 4 agent hops without a user message"`. `curl -sN --max-time 2 http://$A/channels/4/events` prints `turn` idle, then `pause` with that reason.
- **Continue runs the held wake.** `curl -sN --max-time 150 http://$A/channels/4/events > $E/ping-events.txt &`, then `$H api POST /channels/4/continue` is 204. `wait_for /channels/4 '"paused":"hop cap'` and `wait_done 4 9`: four more turns ran (`architect`, `lead`, `architect`, `lead`: the count started from 0 again) and the channel paused again. `grep -A1 -e 'event: pause' -e 'event: turn' $E/ping-events.txt` shows the two frames sent on connect (`turn` idle, `pause` with the reason), then `pause` `{"reason":""}`, then `turn` frames each with a `role` (`"role":"architect","status":"running"` and so on), and last `pause` with the hop cap reason.
- **A user message clears the pause.** The channel is paused again with nine turns. `$H api POST /channels/4/send '{"text":"again"}'` is 202, and `wait_done 4 14`: five more turns ran, `lead`, `architect`, `lead`, `architect`, `lead` (`$H sql "select role from turn where channel_id = 4 order by id desc limit 5"`). The first answers `again`, so the held wake of the earlier pause was dropped, and the count started at 0: the 5th turn is the 4th hop. `$H api GET /channels/4` has the hop cap reason again.
- `continue` while a chain runs is 409; the recipe cannot time that, so `TestContinue_WhileRunning` in `server/live_test.go` covers it.

Gate, channel 5 (`gated`: lead to architect is on). Start the stream first: `curl -sN --max-time 300 http://$A/channels/5/events > $E/gate-events.txt &`.

- **Held, not run.** `$H api POST /channels/5/send '{"text":"DESIGN a signup form."}'`, `wait_for /channels/5/approvals '"status":"pending"'`, `wait_done 5 1`. `$H api GET /channels/5/approvals` is one row: `"id":1`, `"from":"lead"`, `"to":"architect"`, `"text":"lead wrote: @architect design a signup form."` (the model's wording may differ), `"status":"pending"`. `$H sql "select role from turn where channel_id = 5"` is only `lead`.
- **Approve runs it.** `$H api POST /channels/5/approvals/1 '{}'` is 400 `approve is required`. `$H api POST /channels/2/approvals/1 '{"approve":true}'` is 404 (not this channel's). `$H api POST /channels/5/approvals/1 '{"approve":true}'` is 204. `wait_done 5 2`: the second turn is `architect`. `$H sql "select id, from_role, to_role, status from approval"` is `1|lead|architect|approved`. `$H api POST /channels/5/approvals/1 '{"approve":false}'` is 409 `approval 1 is approved: conflict`.
- **Reject drops it.** `$H api POST /channels/5/send '{"text":"DESIGN a password reset."}'`, `wait_for /channels/5/approvals '"id":2'`, `wait_done 5 3`. `$H api POST /channels/5/approvals/2 '{"approve":false}'` is 204. `$H api GET /channels/5/approvals` is `[]`; the approval row is `rejected`; the channel still has three turns (`lead`, `architect`, `lead`) and no new architect turn.
- **Approve while busy.** `$H api POST /channels/5/send '{"text":"DESIGN a profile page."}'`, `wait_for /channels/5/approvals '"id":3'`, `wait_done 5 4`. Then `$H api POST /channels/5/send '{"text":"Write the numbers 1 to 400 separated by spaces."}'` is 202, and at once `$H api POST /channels/5/approvals/3 '{"approve":true}'` is 409 `channel 5 has a turn running: conflict`. `wait_done 5 5`. `$H api GET /channels/5/approvals` still lists approval 3 as `pending`.
- **Frames.** `grep -v -e '^$' -e '"kind"' -e 'event: agent' $E/gate-events.txt` shows `event: approval` with `"status":"pending"` while the lead's turn is still running (before its `turn` done), `event: approval` with `"status":"approved"` after the approve (and `rejected` after the reject), and `turn` frames with a `role`.

Re-apply while the lead is live, channel 6:

- **New role mid-channel.** `$H api POST /channels '{"projectId":1,"featureId":1,"name":"reapply","team":"duo"}'` is id 6. `$H api POST /channels/6/send '{"text":"Hello. Reply with one word."}'`, `wait_done 6 1`: the lead's `claude` is live. `$H reef -- team apply $E/duo3.json` prints `applied team duo: lead*(claude) architect(claude) tester(claude)`. `$H api POST /channels/6/send '{"text":"@tester run the checks"}'`, `wait_done 6 2`. `$H sql "select role, status from turn where channel_id = 6 order by id"` is `lead|done`, `tester|done`, and the newest message is by `tester` and ends `TESTED`.

Restart (still gate room, pending approval 3, and the paused channel 4):

- **Held state.** `$H serve-stop`, `ANTHROPIC_MODEL=sonnet $H serve-start --data-name teams`, then set `A` again. `$H api GET /channels/5/approvals` still has approval 3 `pending`. `$H sql "select paused from channel where id = 4"` is still the hop cap reason. `$H api POST /channels/4/continue` is 204, `$H sql "select paused from channel where id = 4"` is empty and `$H sql "select count(*) from turn where channel_id = 4"` is still `14`: the held wake was in memory and is gone. `$H api POST /channels/5/approvals/3 '{"approve":true}'` is 204; `wait_done 5 6` and the new turn is `architect`.

Codex teammate on the test fixture (`mixed`: the architect is on `codex`). The fixture `agent/codex/testdata/codex` plays a codex that answers `hi`. This does not prove real codex; with a logged-in `codex`, leave the `PATH` and `FAKE_CODEX` part out and the architect answers for real.

- **Setup.** `$H serve-stop`. `mkdir -p $S/shim && ln -s "$PWD/agent/codex/testdata/codex" $S/shim/codex`. `$H reef --data-name mixed -- team apply $E/mixed.json` prints `applied team mixed: lead*(claude) architect(codex)`. `FAKE_CODEX=replay PATH=$S/shim:$PATH ANTHROPIC_MODEL=sonnet $H serve-start --data-name mixed`, set `A`. `$H api POST /projects '{"name":"teams","path":"'$S'/cwd"}'`, `$H api POST /channels '{"projectId":1,"name":"mixed room","team":"mixed"}'` (a quick session, 201 with `"team":"mixed"`).
- **Wake.** `$H api POST /channels/1/send '{"text":"DESIGN a form."}'`, `wait_done 1 2`. `$H sql "select id, role, backend, status, session_id from turn"` is `1|lead|claude|done|<uuid>` and `2|architect|codex|done|t1`. `$H api GET '/messages?channel=1'` ends with an `architect` message `hi`.

Window (by hand, in a browser; no `ui` scenario drives this yet). With a server on the `teams` data folder:

- **New channel.** The New channel form has a team picker with `duo`, `gated`, `keys`, `pingpong`, `solo`. Pick `duo`, create the channel: the sidebar shows it, `GET /channels/<id>` has `"team":"duo"`.
- **Thread.** Send `DESIGN a login form.`. While the architect runs, the live turn names it. When it ends, the architect's message has a reply count and a Thread pane opens on click; reply there. The architect (not the lead) answers in the pane, and the main list folds the replies under their root with the count.
- **Gate.** In a `gated` channel, after the lead mentions the architect, a row `lead to architect: ...` shows Approve and Reject. Approve runs the architect; Reject removes the row.
- **Hop cap.** In a `pingpong` channel, after five turns the cap banner shows `hop cap: 4 agent hops without a user message` and the composer stays enabled (other pauses disable it); sending a message clears the banner; Continue runs the held wake.

When done: `$H cleanup`.

## Gotchas

- The model decides the words, so check structure, not text: who ran (`turn.role`), who posted (`message.author`), `parentId`, rows in `approval`. The personas steer the model; if a run takes another path (the lead answers instead of mentioning, a lead that does not call `propose_wrap`), start the step again on a fresh channel.
- `wait_done` counts a channel's done turns. The row turns `done` a moment before the server routes its messages, so wait for the last turn of the chain (the `N` in each step), not for "no running turn".
- Message and approval ids are global to the data folder; the steps name them by query (`ROOT=...`), not by number, when other channels wrote before.
- A raw `insert into channel` must set `team_id` (`1` is `solo`): a channel with no team makes `GET /channels/{id}` fail. Rows made by the API or by the migration always have one.
- `ps` check: a claude agent is a direct child of `reef serve`, so `pgrep -P $(cat $S/serve.pid) -x claude` counts them. `reef mcp` children belong to the claude process.
- `curl -sN --max-time N` on the event stream exits 28 when the time is up; that is expected.
- The hop cap counts turns woken by an agent, not the turn that answers the user, so a pause happens after `1 + 4` turns.
- The held wake and the hop count live in the server's memory; only the pause and the approvals are stored.
- `reef chat` is always the lead of a `solo` channel and routes nothing; its recipes are in `chat.md`.
