# Teams

A user defines a team of roles in a JSON file with `reef team apply`, opens a Space on that
team, and talks to its lead. The lead hands work to a specialist by an `@mention`. The server
wakes the specialist in its own agent session, which stops when its turn ends, and the reply
lands in a thread. The user replies in that thread and the thread's owner answers. Agents that
keep waking each other hit the hop cap and the Space pauses. Contract: `docs/server.md`
(team, role, Spaces live, Caps) and `docs/memory.md` (Teams, reef mcp, the context prompt).

## Sub-features

- `team-apply` `reef team apply FILE.json` creates a team; `reef team list` shows each team with its roles and backends, the lead starred. The same name again replaces its roles: a changed persona updates the one team.
- `team-errors` a file with no lead or two leads, a role named `user` or with a space, a bad backend, an unknown field, a missing file, and no subcommand each fail with `reef: ...`, exit 1, and change nothing.
- `team-rest` `GET /teams` lists teams by name (`solo` always exists); `GET /teams/{name}` returns one with its roles, 404 for an unknown name; the routes take no write; a role has no `mcpServers` key.
- `team-space` `POST /spaces` with `team` stores it and fills `role` (the lead's name), `backend` and `permissionMode` from the lead role; with no team the Space is in `solo`; an unknown team and a `role` other than `lead` are 400.
- `team-reapply-live` a team applied again with a new role while the lead is live: `@tester` runs a tester turn.
- `thread-send` `POST /spaces/{id}/send` with `parent` stores the reply under that top-level message; a parent that is not a top-level message of the Space is 400; `role` is no longer a field (400).
- `thread-read` the `read_thread` MCP tool with `root` returns the root and its replies in order; a reply id as `root` is `not found`; in a quick session it reads the token's own Space.
- `lazy-lead` a first message `@architect ...` in a new Space starts only the architect: no lead turn, no lead token, one `claude` child while it runs and none after.
- `start-fails` a role whose agent fails to start (a `codex` that runs `exit 1` first on `PATH`) leaves one `failed` turn of that role with the start error in `GET /turns?space=ID`, and the server log has a line with it.
- `session-per-teammate` the lead's `claude` stays after its turn; a specialist's is gone when its wake ends; each has its own token, its own session id, the role's persona and skills and team line in its first prompt, and the role's `tools` as `--allowedTools`; a second wake resumes the same session.
- `route-lead-architect` the user asks the lead; the lead's reply `@architect ...` wakes the architect, which posts a top-level message it owns; the lead gets no turn after it.
- `route-thread-owner` a user reply with `parent` on the architect's message goes to the architect, not the lead, and the answer lands in the same thread.
- `route-default-lead` a user message with no mention and no thread goes to the lead.
- `route-space-role` on a Space whose `role` is `developer` in the `solo` team, `@lead hi` runs one `developer` turn and starts no `lead` session.
- `post-to-space` a message the lead posts with the `post_to_space` tool wakes the architect after the lead's turn ends, even when the lead's reply names no one.
- `route-mentions` one message that mentions two roles wakes both; a turn that posts `@architect` and replies naming `@architect` runs one architect turn; an agent's reply in a thread, owned by the user or the lead, wakes nobody unless it mentions a role.
- `wake-ids` a wake's prompt starts with `[Space N, message N]` for a top-level message and `[Space N, message N, thread N]` for a thread reply, and `post_to_space` with `parent` set to the header's thread id stores the post in the thread. A resumed session's wake may have a `### Changed since your last turn` or `### New brief` part right after the header (`wake-delta`).
- `wake-delta` a role's wake in a resumed session on a feature has, right after its header line, what changed since its session's previous sent turn, read from `turn.prompt` (`$H sql "select prompt from turn where id = N"`): one `kind N: first line (author, date)` line for each memory item accepted since in the role's scopes (`work_item`, `project`, the Space's `team` unless it is `solo`, and the role's own `teammate`), whoever its author (an item by the role's own name too), `decision N is retired, replaced by decision M.` (`decision N is retired.` with no replacement) for each retired one, and `### New brief` with the brief when a version is newer. An item added and retired since is named as retired only, another role's `teammate` item never shows, it comes before any `reef wrote:` lines, a repeat wake with no change has no such part, and the brief comes once. Add the changes with `REEF_TOKEN=<lead token> $H mcp-call remember ...`, `$H api POST /memory`, `$H api POST /memory/ID/retire` and `$H api PUT /work-items/ID/brief`; the fake `claude` matches on the message after the last blank line, so the part does not change which scenario runs.
- `token-role` a teammate's tool calls carry its own role: `remember` by the lead token has author and role `lead`, by the architect's session `architect`.
- `hop-cap` two teammates that keep mentioning each other run 6 hops, the 7th does not run, the Space is paused with `hop cap: 6 agent hops without a user message`; `continue` runs the held wake and counts from 0 again; a user send clears the pause, drops the held wake and counts from 0.
- `hop-work-resets` a developer whose turns call `Edit` and a tester whose turns call `Bash`, each handing back to the other, run 12 hops and the Space is not paused: a tool call other than Reef's own resets the count.
- `hop-talk-pauses` a lead and a developer whose turns only call `post_to_space` pause after 6 hops with `hop cap: 6 agent hops without a user message`; the work item goes to `needs_you` and `GET /inbox` has a `held` item for the Space.
- `budget-per-item` two Spaces of one work item count their turns against one budget, the work item's `tokenBudget`; `continue` raises that budget.
- `direct-order` a turn that sends a direct request to the developer and then hands off to `@tester` in its reply wakes the developer first, then the tester: the wakes of one turn run in the order their messages were stored.
- `direct-close-stops` closing a work item (or approving a wrap-up) gives each direct request still `waiting` in its Spaces, held by a pause for example, a `failed` response with body `stopped`, and the request's `state` is `stopped`.
- `direct-failed-named` after a direct request fails, the sender's next prompt has the line `reef wrote: Your direct request to architect failed: <first line of the request>`, and the one after has not.
- `handoff-runs` a lead mention wakes the architect automatically with no approval API or event.
- `wrap-wakes-nobody` a `/wrap` turn whose reply says `@architect` starts no architect turn.
- `restart-held` after a restart a hop-cap pause stays, and `continue` runs the held wake.
- `codex-teammate` a role on `codex` is woken and its reply is stored (driven with the codex test fixture, not a real codex).
- `provider-rest` `GET /providers` lists the detected `claude` and `codex` with `path` and `version`; `POST /providers` adds a second claude from another folder and it appears with its folder and version; a bad kind, a relative or missing folder, a folder with no executable of the kind, a bad name, a duplicate name and the name `claude` are rejected with an error; `DELETE` removes an added one and refuses `claude` and `codex`.
- `provider-role` a role with `"provider":"NAME"` runs that install's binary: with two wrapper folders that each log a marker and `exec` the fake `claude`, only the named install's marker appears and a role with no provider logs none. A team that names an unknown provider, or one of the other kind, is `invalid`; a removed provider, or a Space `backend` that differs from the install's kind, gives a `failed` turn with the reason, not a run of the `PATH` binary.
- `window-teams` the window picks a team, names the running teammate, opens a Thread pane from a root's Reply button, and shows `N replies in thread` on the root in the main list (`$H ui teams`, see Window below).

## How to get to it (user POV)

- Write a team file (see `docs/memory.md`), `reef team apply team.json`, then in the window choose the team in the New Space form, or `POST /spaces` with `"team":"<name>"`.
- Talk to the lead with `POST /spaces/{id}/send`. Reply in a thread with `"parent": <root id>`. Watch `GET /spaces/{id}/events`.

## Driving it with verify-reef

Preconditions:

- Baseline from the README, with `$S/cwd` a git repo with one commit (the Spaces below are on a feature, so they run in the worktree `$S/teams/worktrees/1/login`). `ANTHROPIC_MODEL=sonnet` on every `serve-start`. The run is on `claude`; `codex` is not needed (the codex step uses the repo's test fixture), so `doctor` showing `FAIL codex-auth` does not matter here.
- In every Bash call set `H=.claude/skills/verify-reef/scripts/verify-reef`, `REEF_VERIFY_RUN=<id>`, `E=~/.reef-verify/<id>`, `S=$(cd /tmp && pwd -P)/reef-verify/<id>` and, once a server runs, `A=$(cat $S/serve.addr)` (the port changes after every `serve-start`).
- Two helpers to paste into a call that waits. `wait_done CH N` waits until Space CH has N done turns (N is the last turn of the chain, so a half-run chain does not match); `wait_for PATH TEXT` waits until `GET PATH` contains TEXT:

```sh
wait_done() { for i in $(seq 60); do [ "$(curl -s "http://$A/turns?space=$1&status=done" | grep -o '"id"' | wc -l)" = "$2" ] && { sleep 1; return; }; sleep 3; done; echo "timeout: Space $1 never had $2 done turns"; return 1; }
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
      "persona": "You are the lead. Answer in one short sentence. If the user's message contains the word DESIGN, reply instead with exactly one line: @architect followed by the user's request. If asked to wrap up the Space, call propose_wrap, then reply with exactly two lines: Proposed. and then @architect FYI"
    },
    {
      "name": "architect",
      "backend": "claude",
      "persona": "You are the architect. Answer in one short sentence, and end it with the word PLANNED. Use no tools unless asked to. Never @mention anyone.",
      "skills": ["short-plans"],
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
sed 's/short-plans/very-short-plans/' $E/duo.json > $E/duo2.json
sed -e 's/"name": "duo"/"name": "mixed"/' -e '/"name": "architect"/{n;s/"claude"/"codex", "permissionMode": "read-only"/}' $E/duo.json > $E/mixed.json
printf '{"name":"bad","roles":[{"name":"a","backend":"claude"}]}' > $E/nolead.json
printf '{"name":"bad","roles":[{"name":"a","lead":true,"backend":"claude"},{"name":"b","lead":true,"backend":"codex"}]}' > $E/twolead.json
printf '{"name":"bad","roles":[{"name":"a","lead":true,"backend":"gemini"}]}' > $E/badbackend.json
printf '{"name":"bad","roles":[{"name":"a","lead":true,"backend":"claude"},{"name":"user","backend":"claude"}]}' > $E/userrole.json
printf '{"name":"bad","roles":[{"name":"a","lead":true,"backend":"claude"},{"name":"two words","backend":"claude"}]}' > $E/spacerole.json
printf '{"name":"keys","roles":[{"name":"lead","lead":true,"backend":"claude","mcpServers":[{"command":"true","env":{"API_KEY":"sk-verify-5521"}}]}]}' > $E/keys.json
printf '{"name":"duo","roles":[{"name":"lead","lead":true,"backend":"claude","persona":"You are the lead. Answer in one short sentence."},{"name":"architect","backend":"claude"},{"name":"tester","backend":"claude","persona":"You are the tester. Answer in one short sentence, and end it with the word TESTED. Never @mention anyone."}]}' > $E/duo3.json
printf '{"name":"bad","roles":[{"name":"a","lead":true,"backend":"claude","color":"red"}]}' > $E/unknownfield.json
```

Teams, no server needed (data folder `teams`):

- **Apply and list.** `$H reef --data-name teams -- team apply $E/duo.json` prints `applied team duo: lead*(claude) architect(claude)`. `$H reef -- team list` prints `duo  lead*(claude) architect(claude)` and `solo  lead*(claude)`. `$H sql "select team.name, member.name, lead from team_member member join team on team.id = team_id order by team.name, member.id"` shows `duo|lead|1`, `duo|architect|0`, `solo|lead|1`.
- **Apply again.** `$H reef -- team apply $E/duo2.json` prints the same line. `$H sql "select count(*) from team"` is still `2`, and `$H sql "select skills from teammate where name = 'duo-architect'"` is `["very-short-plans"]`. Put the original back with `$H reef -- team apply $E/duo.json`.
- **Errors.** Each `$H reef -- team apply $E/<file>` ends `exit: 1` and `$H sql "select count(*) from team"` stays `2`:
  - `nolead.json`: `reef: invalid: team bad needs exactly one lead role, has 0`
  - `twolead.json`: `reef: invalid: team bad needs exactly one lead role, has 2`
  - `badbackend.json`: `reef: role a: invalid: backend must be claude or codex`
  - `userrole.json`: `reef: invalid: role name "user" must be letters, digits, _ or -, and not user or reef`
  - `spacerole.json`: `reef: invalid: role name "two words" must be letters, digits, _ or -, and not user or reef`
  - `unknownfield.json`: `reef: parse <path>: json: unknown field "color"`
  - `$H reef -- team apply /nonexistent.json`: `reef: read team file: open /nonexistent.json: no such file or directory`
  - `$H reef -- team`: `reef: usage: reef team apply FILE.json | list [--data DIR]`
- **Two more teams.** `$H reef -- team apply $E/pingpong.json` and `$H reef -- team apply $E/keys.json` print `applied team ...`. `$H sql "select count(*) from team_member where mcp_servers like '%sk-verify-5521%'"` is `1`: the env is stored for the agent.

Server (the same folder): `ANTHROPIC_MODEL=sonnet $H serve-start --data-name teams`, then `A=$(cat $S/serve.addr)`.

- **REST.** `$H api GET /teams` is 200 with `duo`, `keys`, `pingpong`, `solo` in that order; each role has `name`, `lead`, `write`, `backend`, `permissionMode`, `persona`, `skills`, and `tools`, with no `gates` key. `$H api GET /teams/keys` is 200 and has no `mcpServers` key; neither it nor `$H api GET /teams` contains `mcpServers`, `sk-verify-5521` or `API_KEY`. `$H api GET /teams/nope` is 404 `team nope: not found`. `$H api POST /teams '{}'` is 405.
- **Setup.** `$H api POST /projects '{"name":"teams","path":"'$S'/cwd"}'`, `$H api POST /work-items '{"projectId":1,"title":"Login"}'`.
- **Space on a team.** `$H api POST /spaces '{"projectId":1,"workItemId":1,"name":"plain","backend":"claude"}'` is 201 id 1 with `"team":"solo"`. `$H api POST /spaces '{"projectId":1,"workItemId":1,"name":"duo room","team":"duo"}'` is id 2 with `"team":"duo","role":"lead","backend":"claude"`: the backend came from the lead role, the request named none. Then create `lazy` on `duo` and `ping room` on `pingpong`. An unknown team is 400, as is a role other than lead.

Lazy lead, Space 3. `claude` processes of the server are `pgrep -P $(cat $S/serve.pid) -x claude`:

- **Only the architect starts.** `$H api POST /spaces/3/send '{"text":"@architect Write the numbers from 1 to 300 separated by spaces. Remember the codeword kiwi-77."}'` is 202. At once (the turn lasts several seconds): `$H sql "select role from token"` is `architect` and no other row; `pgrep -P $(cat $S/serve.pid) -x claude | wc -l` is `1`; `ps -ww -o args= -p $(pgrep -P $(cat $S/serve.pid) -x claude)` shows `--allowedTools Read,Skill(short-plans),mcp__reef__*` (the architect's `tools`, its picked skill and the `reef` server) and a `--disallowedTools` that names `Edit,Write,MultiEdit,NotebookEdit` and `Skill(name)` for each other claude skill on disk. After `wait_done 3 1`: the `claude` count is `0`, `$H sql "select count(*) from token"` is `0`, and `$H sql "select role from turn where space_id = 3"` is one `architect` row: the lead never started.

Space 2, lead and architect:

- **Lead process stays, specialist process goes.** `$H api POST /spaces/2/send '{"text":"Hello. Reply with one word."}'`, `wait_done 2 1`. The `claude` count is `1`, `$H sql "select role from token"` is `lead`. Then `$H api POST /spaces/2/send '{"text":"DESIGN a login form. Remember the codeword kiwi-77."}'`, `wait_done 2 3`. The `claude` count is `1` and the token is still `lead` only: the architect's process and token are gone. `$H sql "select id, role, status from turn where space_id = 2 order by id"` is `lead`, `lead`, `architect`, all `done`, and the last row is the architect: the lead got no turn after the architect posted.
- **Routing from the lead.** `$H api GET '/messages?space=2'` has, in order, user `Hello...`, lead (one word), user `DESIGN a login form...`, lead `@architect DESIGN a login form...`, and architect with `"parentId":null` and a body ending `PLANNED` (its persona). The architect ran automatically and no approval event was sent (`handoff-runs`).
- **Persona, skills and team line.** Take the architect's session id: `$H sql "select session_id from turn where role = 'architect' and space_id = 2"`. With `K=$(printf %s "$S/teams/worktrees/1/login" | sed 's/[^A-Za-z0-9]/-/g')`, `grep -c "You are the architect" ~/.claude/projects/$K/<session id>.jsonl`, `grep -c "Your skills:" ...` and `grep -c "Your team: @lead, @architect" ...` are each at least 1. For the lead's session id the file has `Do not summarize or repeat a specialist's post back to the user`. Grep before `cleanup`, which removes that folder.
- **Token role.** `T=$($H sql "select token from token where role = 'lead'" | tail -1)`, then `REEF_TOKEN=$T $H mcp-call remember '{"kind":"decision","body":"Lead check"}'` gives `"author":"lead"`, `"role":"lead"`, `"scope":"work_item:1"`.
- **Thread send and its errors.** The architect's message id is `ROOT=$($H sql "select id from message where space_id = 2 and author = 'architect'" | tail -1)`. `$H api POST /spaces/2/send '{"text":"x","parent":9999}'` is 400 `invalid: parent 9999 is not a top-level message of space 2`. A parent from another Space is 400 the same way: `$H api POST /spaces/2/send '{"text":"x","parent":1}'` (message 1 is in Space 3). `$H api POST /spaces/2/send '{"text":"x","role":"architect"}'` is 400 `unknown field "role"`. `$H sql "select count(*) from message where space_id = 2"` is still `5`.
- **Thread owner answers, in the thread, and resumes.** `$H api POST /spaces/2/send '{"text":"What codeword did I give you? Answer with the codeword only. Also call the reef remember tool once with kind decision and body Codeword is kiwi-77.","parent":'$ROOT'}'` is 202. `wait_done 2 4`. `$H api GET '/messages?space=2&after='$ROOT` has the user message and an architect message, both `"parentId":<ROOT>`, the answer `kiwi-77`. `$H sql "select id, role, session_id from turn where space_id = 2 order by id"`: the two `architect` turns have the same session id, so the second wake resumed it (it recalled the codeword), and the lead still has two turns. `$H sql "select id, author, body from memory"` has `1|lead|Lead check` and `2|architect|Codeword is kiwi-77`.
- **read_thread with a root.** `REEF_TOKEN=$T $H mcp-call read_thread '{"spaceId":2,"root":'$ROOT'}'` gives `isError=false` and three messages, oldest first: the architect's root (`"parentId":null`), the user reply and the architect reply (`"parentId":<ROOT>`); `tools:` lists `post_to_space` among 23 tools. A reply id as root, `'{"spaceId":2,"root":<reply id>}'`, gives `isError=true thread <id>: not found`. In a quick session `read_thread` reads only the token's own Space: with the lead token of a quick-session Space, `{"spaceId":<that space>,"root":<a top-level message id>}` gives `isError=false` and the message, and any other Space's id gives `isError=true this space has no feature`.
- **post_to_space wakes.** `$H api POST /spaces/2/send '{"text":"Use the reef post_to_space tool once to ask the architect, mentioning them, to state the codeword again. Then reply with just the word: posted"}'`, `wait_done 2 6`. Newest messages: user, lead `@architect ...` (posted by the tool, top level), lead `posted`, architect `...kiwi-77. PLANNED`. `$H sql "select id, role from turn where space_id = 2 order by id desc limit 2"` is the architect, then the lead. The lead's reply names no one, so the wake came from the tool's message. The lead's session shows the tool: `grep -c mcp__reef__post_to_space ~/.claude/projects/$K/<lead session id>.jsonl` is at least 1. The architect's session has the prompt line: `grep -c '\[Space 2, message' ~/.claude/projects/$K/<architect session id>.jsonl` is at least 1.
- **Wrap wakes nobody.** `$H sql "select role, count(*) from turn where space_id = 2 group by role"` is `architect|3`, `lead|3`. `$H api POST /spaces/2/send '{"text":"/wrap"}'`, `wait_done 2 7`. The same query is `architect|3`, `lead|4`. The newest lead message is `Proposed.\n@architect FYI`: its line-start mention woke no one. `$H api GET /spaces/2/proposal` is 200 `pending`.
- **No mention, no thread, goes to the lead.** The `Hello. Reply with one word.` message in the first step had none: its only turn is the lead's (`lead` turn 1 of Space 2, before any architect turn).
- **Post into the thread.** `$H api POST /spaces/2/send '{"text":"Call the reef post_to_space tool once with body: thread post, and parent set to the thread id in the header of this message. Then reply: ok","parent":'$ROOT'}'`, `wait_done 2 8`. `$H api GET '/messages?space=2&after=<newest id before the send>'` has the user message, an architect `thread post` and an architect `ok`, all three with `"parentId":<ROOT>`: the architect's header was `[Space 2, message N, thread <ROOT>]` and the post took the thread id. Its replies in the thread woke nobody: `$H sql "select role, count(*) from turn where space_id = 2 group by role"` is `architect|4`, `lead|4`.

Hop cap, Space 4 (`pingpong`: the lead always says `@architect ping`, the architect `@lead pong`):

- **Pause after 6 hops.** `$H api POST /spaces/4/send '{"text":"start"}'`, then `wait_for /spaces/4 '"paused":"hop cap'`. `$H sql "select role, status from turn where space_id = 4 order by id"` is seven `done` rows: `lead`, `architect`, `lead`, `architect`, `lead`, `architect`, `lead` (the lead's first turn answers the user; the next six are hops, and the 7th hop did not run). `$H api GET /spaces/4` has `"paused":"hop cap: 6 agent hops without a user message"`. `curl -sN --max-time 2 http://$A/spaces/4/events` prints `turn` idle, then `pause` with that reason.
- **Continue runs the held wake.** `curl -sN --max-time 150 http://$A/spaces/4/events > $E/ping-events.txt &`, then `$H api POST /spaces/4/continue` is 204. `wait_for /spaces/4 '"paused":"hop cap'` and `wait_done 4 13`: six more turns ran (`architect`, `lead`, `architect`, `lead`, `architect`, `lead`: the count started from 0 again) and the Space paused again. `grep -A1 -e 'event: pause' -e 'event: turn' $E/ping-events.txt` shows the two frames sent on connect (`turn` idle, `pause` with the reason), then `pause` `{"reason":""}`, then `turn` frames each with a `role` (`"role":"architect","status":"running"` and so on), and last `pause` with the hop cap reason.
- **A user message clears the pause.** The Space is paused again with thirteen turns. `$H api POST /spaces/4/send '{"text":"again"}'` is 202, and `wait_done 4 20`: seven more turns ran, `lead`, `architect`, `lead`, `architect`, `lead`, `architect`, `lead` (`$H sql "select role from turn where space_id = 4 order by id desc limit 7"`). The first answers `again`, so the held wake of the earlier pause was dropped, and the count started at 0: the 7th turn is the 6th hop. `$H api GET /spaces/4` has the hop cap reason again.
- `continue` while a chain runs is 409; the recipe cannot time that, so `TestContinue_WhileRunning` in `server/live_test.go` covers it.

Hop count and tool calls, a folder of its own (`--data-name hop`) with the team `full` (`$H reef --data-name hop -- team apply $PWD/teams/full.json`) and `$S/cwd` a repo with one commit. The fake scenarios are `work-lead`, `work-developer`, `work-tester`, `talk-lead` and `talk-developer`:

- **Setup.** `$H api POST /projects '{"name":"hop","path":"'$S'/cwd"}'`, then three work items `workpong`, `talkpong` and `budget` (ids 1 to 3). `$H sql --write "update work_item set status = 'in_progress' where id in (1, 2)"`. `$H api POST /spaces '{"projectId":1,"workItemId":N,"name":"sN","team":"full"}'` for N 1 and 2 (Space ids 1 and 2).
- **Work resets the count.** `$H api POST /spaces/1/send '{"text":"workpong"}'` (the lead answers `@developer work`, the developer calls `Edit` and answers `@tester check`, the tester calls `Bash` and answers `@developer again`, and so on). When `$H sql "select count(*) from turn where space_id = 1 and status = 'done'"` is 13, `$H api POST /spaces/1/stop`. `$H sql "select role, status from turn where space_id = 1 order by id"` is `lead`, then `developer` and `tester` alternating, 13 `done` rows (the lead's answer and 12 hops), and `$H sql "select paused from space where id = 1"` is empty. `$H sql "select turn.role, t.name, count(*) from tool_call t join turn on turn.id = t.turn_id where turn.space_id = 1 group by turn.role, t.name"` is `developer|Edit` and `tester|Bash` six times each.
- **Talk only pauses at 6.** `$H api POST /spaces/2/send '{"text":"talkpong"}'`, then `wait_for /spaces/2 '"paused":"hop cap'`. `$H sql "select role, status from turn where space_id = 2 order by id"` is seven `done` rows, `lead` and `developer` alternating from `lead`; the only tool in `tool_call` is `mcp__reef__post_to_space`. `$H sql "select status from work_item where id = 2"` is `needs_you`. `$H api GET /inbox` is one item `{"kind":"held","group":"needs_you","spaceId":2,"workItemId":2,"text":"hop cap: 6 agent hops without a user message (1 wake held)",...}`.
- **One budget per work item.** `$H api POST /spaces '{"projectId":1,"workItemId":3,"name":"first","team":"full"}'` (id 3), `$H sql --write "update space set closed_at = '2026-10-27T00:00:00Z' where id = 3"`, `$H api POST /spaces '{"projectId":1,"workItemId":3,"name":"second","team":"full"}'` (id 4). `$H sql --write "insert into turn (space_id, role, backend, status, input_tokens, started_at, ended_at) values (3, 'lead', 'claude', 'done', 700000, '2026-10-27T00:00:00Z', '2026-10-27T00:00:01Z'), (4, 'lead', 'claude', 'done', 400000, '2026-10-27T00:00:02Z', '2026-10-27T00:00:03Z')"`. Neither Space is over 1,000,000 alone. `$H api POST /spaces/4/send '{"text":"Say ok in one word"}'` is 409 `space 4 is paused: token budget of 1000000 used: 1100000 tokens: conflict`. `$H api POST /spaces/4/continue` is 204; `$H api GET /work-items/3` and `GET /spaces/4` have `"tokenBudget":2100000`, and a send runs a turn.

Direct requests, a folder of its own (`--data-name direct`) with the team `full` (`$H reef --data-name direct -- team apply $PWD/teams/full.json`) and `$S/cwd` a repo with one commit. The fake scenarios are `order-lead`, `order-developer`, `order-tester`, `order-noted`, `coordinate-send`, `direct-review`, `direct-noted`, `coordinate-fail`, `direct-fail`, `hop6-lead`, `hop6-confirm`, `hop6-noted`, `hop6-fail`, `wrap-propose` and `say-ok-one-word`:

- **Setup.** `$H serve-start --data-name direct`, `$H api POST /projects '{"name":"direct","path":"'$S'/cwd"}'`, then three work items `order`, `held` and `failed` (ids 1 to 3) and `$H sql --write "update work_item set status = 'in_progress' where id in (1, 2, 3)"`. One Space each: `$H api POST /spaces '{"projectId":1,"workItemId":N,"name":"sN","team":"full"}'` (ids 1 to 3). The token budget of a Space is its top-level work item's.
- **Order.** `$H api POST /spaces/1/send '{"text":"ORDER the build check."}'`, then `$H sql "select role, status from turn where space_id = 1 order by id"` is `lead`, `developer`, `tester`, `lead`, all `done`: the lead sent the developer a direct request and its reply handed off to `@tester`, and the request, stored first, ran first. `$H sql "select from_role, to_role, status, state from direct_message where space_id = 1"` is `lead|developer|sent|answered` and `developer|lead|done|`.
- **Close stops a held request.** `$H sql --write "update work_item set token_budget = 100 where id = 2"`, `$H api POST /spaces/2/send '{"text":"COORDINATE the login review with the architect."}'`. The lead's turn uses more than 100 tokens, so `$H sql "select paused from space where id = 2"` is `token budget of 100 used: 244 tokens`, `select status, direct_message_id from approval where space_id = 2` is `pending|<the request>` and the request's `state` is `waiting`; the architect did not run. `$H api POST /work-items/2/close` is 204, and `$H api GET /spaces/2/direct-messages` has the request with `"state":"stopped"` and a response from `architect` with `"status":"failed"`, `"body":"stopped"`.
- **A failed request is named once.** `$H api POST /spaces/3/send '{"text":"COORDINATE the failing review with the architect."}'`: the architect's turn is `failed` (`the review tool crashed`) and the request's `state` is `failed`. `$H api POST /spaces/3/send '{"text":"Say ok in one word."}'`, then `$H sql "select prompt from turn where space_id = 3 and role = 'lead' order by id desc limit 1"` is `[Space 3, message N]`, `reef wrote: Your direct request to architect failed: Please review the failing plan.`, a blank line and `Say ok in one word.`. After `Say fine in one word.` the newest lead prompt has no `Your direct request`.
- **Retry at the hop cap runs.** Two more work items `hop` and `wrap` (ids 4 and 5), in progress, each with a `full` Space (ids 4 and 5). `$H api POST /spaces/4/send '{"text":"RETRY the hop six check."}'`, `wait_done 4 6`. The lead sent three requests; the architect, developer and tester answered, and the lead's wakes from those answers are two `done` turns (`Noted.`) and one `failed` (`the hop six check crashed`), the tester's, the sixth hop: `$H sql "select id, role, status, direct_message_id from turn where space_id = 4 order by id"`. `$H api POST /turns/<the failed id>/retry` is 202 and a new `lead|failed|<same direct id>` turn appears: the retry ran (the scripted lead crashes on this wake every time), no hold approval was stored (`select count(*) from approval where space_id = 4` is `0`) and the Space is not paused.
- **Wrap-up says what it left out.** `$H api POST /spaces/5/send '{"text":"Say ok in one word."}'`, `wait_done 5 1`. 120 direct messages `$H sql --write "WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM n WHERE i<120) INSERT INTO direct_message (space_id, from_role, to_role, body, status, state, created_at) SELECT 5, 'developer', 'tester', 'pair note ' || i, 'sent', 'waiting', '2026-10-27T00:00:00Z' FROM n; UPDATE direct_message SET chain_id = id WHERE space_id = 5"`. `$H api POST /spaces/5/wrap` is 202. `$H sql "select prompt from turn where space_id = 5 and wake_text is null"` starts `Direct exchanges in this Space, private between two teammates:`, then `20 older exchanges left out (read_direct_messages)`, then `[direct N] developer -> tester: pair note 21` (the newest 100 follow).
- **A rejected wrap-up leaves the lead's session clean.** Continues Space 5. The wrap-up turn has its own `session_id` (a copy of the lead's session, `--fork-session`): `$H sql "select role, session_id from turn where space_id = 5 order by id"` is the lead's first turn, the wrap-up with another id. `$H api POST /proposals/<id>/reject '{"note":"redo"}'`, `$H api POST /spaces/5/send '{"text":"Say fine in one word."}'`: its turn has the first session id again. `grep -c 'pair note' ~/.claude/projects/*/<first session id>.jsonl` is `0` and in the wrap-up's session file `1`; `$H sql "select instr(prompt, 'pair note') from turn where id = <the new turn>"` is `0`.
- **Two queued wakes to one role.** `$H ui coordination` (see `window.md`) ends with a `full` Space in which the lead's and the developer's requests to the architect queue; each Coordination panel streams only its own turn (`window-coordination`).
- Approving a wrap-up closes its Space the same way; `TestClosingASpaceStopsItsWaitingDirectRequests` in `store/direct_message_test.go` covers both.

Re-apply while the lead is live, Space 6:

- **New role mid-space.** `$H api POST /spaces '{"projectId":1,"workItemId":1,"name":"reapply","team":"duo"}'` is id 6. `$H api POST /spaces/6/send '{"text":"Hello. Reply with one word."}'`, `wait_done 6 1`: the lead's `claude` is live. `$H reef -- team apply $E/duo3.json` prints `applied team duo: lead*(claude) architect(claude) tester(claude)`. `$H api POST /spaces/6/send '{"text":"@tester run the checks"}'`, `wait_done 6 2`. `$H sql "select role, status from turn where space_id = 6 order by id"` is `lead|done`, `tester|done`, and the newest message is by `tester` and ends `TESTED`.

Restart with paused Space 4:

- **Held state.** `$H serve-stop`, `ANTHROPIC_MODEL=sonnet $H serve-start --data-name teams`, then set `A` again. `$H sql "select paused from space where id = 4"` is still the hop cap reason. `$H sql "select kind, status from approval where space_id = 4 and kind = 'hold' and status = 'pending'"` has one row, the wake held by the hop cap. `$H api POST /spaces/4/continue` is 204; the held wake and 5 more hops run before the Space reaches the hop cap again.

Codex teammate on the test fixture (`mixed`: the architect is on `codex`). The fixture `agent/codex/testdata/codex` plays a codex that answers `hi`. This does not prove real codex; with a logged-in `codex`, leave the `PATH` and `FAKE_CODEX` part out and the architect answers for real.

- **Setup.** `$H serve-stop`. `mkdir -p $S/shim && ln -s "$PWD/agent/codex/testdata/codex" $S/shim/codex`. `$H reef --data-name mixed -- team apply $E/mixed.json` prints `applied team mixed: lead*(claude) architect(codex)`. `FAKE_CODEX=replay PATH=$S/shim:$PATH ANTHROPIC_MODEL=sonnet $H serve-start --data-name mixed`, set `A`. `$H api POST /projects '{"name":"teams","path":"'$S'/cwd"}'`, `$H api POST /spaces '{"projectId":1,"name":"mixed room","team":"mixed"}'` (a quick session, 201 with `"team":"mixed"`).
- **Wake.** `$H api POST /spaces/1/send '{"text":"DESIGN a form."}'`, `wait_done 1 2`. `$H sql "select id, role, backend, status, session_id from turn"` is `1|lead|claude|done|<uuid>` and `2|architect|codex|done|t1`. `$H api GET '/messages?space=1'` ends with an `architect` message `hi`.

Failed start, the same `mixed` folder (`start-fails`): a `codex` first on `PATH` that runs `exit 1`, so the architect's agent cannot start.

- **Setup.** `$H serve-stop`. `mkdir -p $S/fail && printf '#!/bin/sh\nexit 1\n' > $S/fail/codex && chmod +x $S/fail/codex`. `PATH=$S/fail:$PATH ANTHROPIC_MODEL=sonnet $H serve-start --data-name mixed`, set `A`.
- **Failed turn and log line.** `$H api POST /spaces/1/send '{"text":"DESIGN a form."}'` is 202. `wait_for '/turns?space=1&status=failed' architect`. `$H api GET '/turns?space=1&status=failed'` is one row: `"id":4,"spaceId":1,"role":"architect","backend":"codex","sessionId":""`, `"status":"failed"` and `"error":"initialize: write |1: broken pipe"` (the text after `initialize:` depends on when the shim exits). `grep 'turn of architect in space 1 failed' $E/serve.log` prints one line with the same error. Turn 3 is the lead's, `done`.

Space role as lead, a new folder `role` (`route-space-role`). `reef chat` makes a `solo` Space whose `role` is `developer`; the API only makes `lead` Spaces. Its feature has its own slug, `roles`: `feature/login` is already checked out in the `teams` folder's worktree, and `reef chat` fails with `branch feature/login is checked out at ...` otherwise.

- **Setup.** `$H serve-stop`. `$H reef --data-name role -- project add`, `$H reef -- feature add roles Roles`. `ANTHROPIC_MODEL=sonnet $H reef -- chat roles --role developer --backend claude` prints `space 1: resume with reef chat --space 1`, `exit: 0` (stdin is empty, so no turn runs). `$H sql "select space.id, space.role, team.name from space join team on team.id = team_id"` is `1|developer|solo`. `ANTHROPIC_MODEL=sonnet $H serve-start --data-name role`, set `A`.
- **`@lead` wakes the Space role.** `$H api POST /spaces/1/send '{"text":"@lead hi"}'` is 202, `wait_done 1 1`. `$H sql "select id, role, status from turn"` is one row, `1|developer|done`. `$H sql "select role from token"` is `developer` only: no `lead` token. `pgrep -P $(cat $S/serve.pid) -x claude | wc -l` is `1`, the developer's process, which stays like a lead's. The developer's reply may name `@lead` too; it wakes no one.

Window (`$H ui teams` drives New Space and Thread; the hop cap is by hand). Apply `duo` and `full` to the server's data folder, start the server, then run `$H ui teams` with `REEF_VERIFY_CHROME` on Linux (see `window.md`).

- **New space.** The New Space form has a team picker listing `duo`, `solo` and any other applied team. Pick `duo`, create the Space: the sidebar shows it, `GET /spaces/<id>` has `"team":"duo"`.
- **Thread.** Send `DESIGN a short plan for a login form.`. While each teammate runs, the live turn names it: `Lead is working...`, then `Architect is working...`. When it ends, the architect's message has a Reply button; a Thread pane opens on click; reply there. The architect (not the lead) answers in the pane, and the main list shows the root with an `N replies in thread` button; the replies appear only in the Thread pane.
- **Hop cap.** In a `pingpong` Space, after seven turns the cap banner shows `Paused: hop cap: 6 agent hops without a user message` and the composer stays enabled (other pauses disable it); sending a message clears the banner; Continue runs the held wake.

When done: `$H cleanup`.

Providers (data folder `providers`, `ANTHROPIC_MODEL=sonnet $H serve-start --data-name providers`, scratch cwd a git repo). Make two installs, each a folder with an executable `claude` that answers `--version` itself and otherwise logs a marker and runs the fake:

```sh
for m in A B; do mkdir -p $E/prov$m; cat > $E/prov$m/claude <<EOF
#!/bin/sh
if [ "\$1" = "--version" ]; then echo "$m-9.0.0 (Claude Code)"; exit 0; fi
echo "$m ran in \$(pwd)" >> $E/markers.log
exec $S/fake/claude "\$@"
EOF
chmod +x $E/prov$m/claude; done; : > $E/markers.log; mkdir -p $E/emptydir
printf '{"name":"provteam","roles":[{"name":"lead","lead":true,"backend":"claude","provider":"installB","persona":"You are the lead."}]}' > $E/provteam.json
printf '{"name":"plainteam","roles":[{"name":"lead","lead":true,"backend":"claude","persona":"You are the lead."}]}' > $E/plainteam.json
```

- **List.** `$H api GET /providers` is 200 with `claude` (`detected:true`, `path` the fake, `version` `0.0.0 (Claude Code) fake`) and `codex` with its path and version.
- **Add and reject.** `POST /providers` `{"name":"installA","kind":"claude","folder":"'$E'/provA"}` and the same for `installB` with `provB` are 201 with `version` `A-9.0.0 (Claude Code)` and `B-9.0.0 ...`; `GET /providers` lists both after the two detected. These are 400: `kind` `gemini` (`provider kind must be claude or codex`), folder `/nonexistent` (`not a directory`), folder `relative` (`not an absolute path`), `$E/emptydir` (`no claude binary in ...`), kind `codex` on `provA` (`no codex binary in ...`), name `my x`. These are 409: `installA` again, name `claude`.
- **Team checks.** `$H reef --data-name providers -- team apply` of a file whose role says `"provider":"nope"` is `reef: invalid: role lead: provider "nope" is not installed`, exit 1; `"backend":"codex","provider":"installB"` is `... provider "installB" is a claude install, the role runs on codex`. Apply `provteam.json` and `plainteam.json`; `GET /teams/provteam` shows `"provider":"installB"`.
- **Runs that binary.** `POST /projects` `{"name":"prov","path":"'$S'/cwd"}` and `POST /work-items` `{"projectId":1,"title":"Prov"}`. `POST /spaces` `{"projectId":1,"workItemId":1,"name":"on B","team":"provteam"}` (id 1), `POST /spaces/1/send` `{"text":"Say ok in one word"}`, wait for a done turn: `cat $E/markers.log` is one line, `B ran in <data>/worktrees/1/prov`. Space 2 on `plainteam` and the same send: `markers.log` is still one line and `fake-claude.log` has a second `replay` line.
- **Removed or mismatched.** `DELETE /providers/installA` and `installB` are 204, `claude` and `codex` are 400, `ghost` is 404. A new Space 3 on `provteam` and a send gives a `failed` turn whose `error` is `invalid: role lead: provider "installB" is not installed`, and `markers.log` is unchanged. Add `installB` again; Space 4 `{"team":"provteam","backend":"codex"}` and a send gives a `failed` turn with `provider "installB" is a claude install, the role runs on codex`.

## Gotchas

- The model decides the words, so check structure, not text: who ran (`turn.role`), who posted (`message.author`), `parentId` and cap-held wakes. The personas steer the model; if a run takes another path (the lead answers instead of mentioning, a lead that does not call `propose_wrap`), start the step again on a fresh Space.
- `wait_done` counts a Space's done turns. The row turns `done` a moment before the server routes its messages, so wait for the last turn of the chain (the `N` in each step), not for "no running turn".
- Message and hold ids are global to the data folder; the steps name them by query (`ROOT=...`), not by number, when other Spaces wrote before.
- A raw `insert into space` must set `team_id` (`1` is `solo`): a Space with no team makes `GET /spaces/{id}` fail. Rows made by the API or by the migration always have one.
- `ps` check: a claude agent is a direct child of `reef serve`, so `pgrep -P $(cat $S/serve.pid) -x claude` counts them. `reef mcp` children belong to the claude process.
- `curl -sN --max-time N` on the event stream exits 28 when the time is up; that is expected.
- The hop cap counts talk-only turns woken by an agent, not the turn that answers the user, so a pause happens after `1 + 6` turns. A turn that calls `Edit`, `Bash` or any tool other than Reef's own (`mcp__reef__*`) resets the count, so recordings of working roles never reach the cap.
- The hop count lives in the server's memory; the pause and the wakes it holds are stored.
- `reef chat` is always the lead of a `solo` Space and routes nothing; its recipes are in `chat.md`.
