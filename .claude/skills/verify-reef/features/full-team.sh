#!/usr/bin/env bash
# Drives every line of full-team.md and prints one PASS/FAIL line per sub-feature id.
# Called by `verify-reef feature full-team`, which exports H, REEF_VERIFY_RUN, E, S and
# runs from the repo root. See ../SKILL.md and full-team.md (the recipe).
set -u
: "${H:?}" "${REEF_VERIFY_RUN:?}" "${E:?}" "${S:?}"
export REEF_VERIFY_RUN

# Sections to run: the names given as arguments, else all.
all_sections="full cx ui2 google fu"
sections="$*"
for sec in $sections; do
	case " $all_sections " in *" $sec "*) ;; *) echo "full-team: unknown section '$sec'; sections: $all_sections" >&2; exit 2 ;; esac
done
sections=${sections:-$all_sections}

failed=0
check() { # check ID OK DETAIL
	if [ "$2" = 1 ]; then echo "PASS $1: $3"; else echo "FAIL $1: $3"; failed=1; fi
}
not_run() { echo "FAIL $1: not run: $2"; failed=1; }

wait_done() { for i in $(seq 60); do [ "$(curl -s "http://$A/turns?space=$1&status=done" | grep -o '"id"' | wc -l | tr -d ' ')" = "$2" ] && { sleep 1; return 0; }; sleep 3; done; echo "timeout: Space $1 never had $2 done turns" >&2; return 1; }
wait_for() { for i in $(seq 60); do curl -s "http://$A$1" | grep -q "$2" && return 0; sleep 2; done; echo "timeout: $1 never had $2" >&2; return 1; }

# api_call METHOD PATH [JSON]: sets STATUS and BODY from one $H api call.
api_call() {
	local out
	out=$("$H" api "$@")
	STATUS=$(printf '%s\n' "$out" | head -1 | cut -d' ' -f2)
	BODY=$(printf '%s\n' "$out" | tail -n +2)
}

# session_file SESSION_ID: path of claude's session jsonl, wherever its project folder is.
session_file() { ls "$HOME"/.claude/projects/*/"$1".jsonl 2>/dev/null | head -1; }

turn_col() { "$H" sql "select $2 from turn where id = $1" | sed 1d | tail -1; } # turn_col ID COLUMN(S)

run_ui() { # run_ui ID SCENARIO
	local id=$1 scenario=$2 out rc
	out=$("$H" ui "$scenario" 2>&1)
	rc=$?
	if echo "$out" | grep -qiE 'Chromium distribution|executable doesn.t exist|could not find chrome|ENOENT'; then
		not_run "$id" "no Chrome/Chromium for \`\$H ui $scenario\`: $(echo "$out" | tail -3 | tr '\n' ' ')"
	else
		check "$id" "$([ "$rc" = 0 ] && echo 1 || echo 0)" "$(echo "$out" | grep -E '^(PASS|FAIL)' | tr '\n' '; ')"
	fi
}

### Team files in $E

cat >"$E/crew.json" <<'EOF'
{
  "name": "crew",
  "roles": [
    {"name": "lead", "lead": true, "backend": "claude",
     "description": "You are the lead. Answer in one short sentence. Never @mention anyone."},
    {"name": "developer", "write": true, "backend": "claude", "permissionMode": "acceptEdits",
     "description": "You are the developer. Do what the user asks with as few tool calls as you can, then answer in one short sentence. Never @mention anyone.",
     "tools": ["Read", "Edit", "Write", "Bash"]},
    {"name": "tester", "backend": "claude", "permissionMode": "acceptEdits",
     "description": "You are the tester. Do what the user asks, then answer in one short sentence. Never @mention anyone.",
     "tools": ["Read", "Edit", "Write", "Bash(ls:*)"]}
  ]
}
EOF
cat >"$E/cx.json" <<'EOF'
{
  "name": "cx",
  "roles": [
    {"name": "lead", "lead": true, "backend": "claude"},
    {"name": "developer", "write": true, "backend": "codex"},
    {"name": "tester", "backend": "codex", "permissionMode": "workspace-write"}
  ]
}
EOF
cat >"$E/relay.json" <<'EOF'
{
  "name": "relay",
  "roles": [
    {"name": "lead", "lead": true},
    {"name": "tester"}
  ]
}
EOF

section_full() {
### Teams (data folder full). Bots are global, so each file's roles update the Bot of that name:
### crew's lead, developer and tester replace full's, and relay names two of them.

ANTHROPIC_MODEL=sonnet "$H" serve-start --data-name full || exit 1
A=$(cat "$S/serve.addr")
seed_write=$("$H" sql "select write from bot where name = 'lead'" | tail -1)
apply_full=$("$H" team-apply "$PWD/teams/full.json")
"$H" team-apply "$E/crew.json" >/dev/null
"$H" team-apply "$E/relay.json" >/dev/null
team_list=$("$H" reef -- team list)

apply_ok=1
case $apply_full in *"applied team full: lead*(claude) architect(claude) developer(claude) tester(claude)"*) ;; *) apply_ok=0 ;; esac
case $team_list in *"full  lead*(claude) architect(claude) developer(claude) tester(claude)"*) ;; *) apply_ok=0 ;; esac
check full-apply "$apply_ok" "team apply full printed the roster; team list has the full line"

wc_out=$("$H" sql "select team.name, bot.name, lead, write from team_member member join team on team.id = team_id join bot on bot.id = bot_id order by team.name, member.id")
wc_ok=1
[ "$seed_write" = 1 ] || wc_ok=0
for row in "crew|lead|1|0" "crew|developer|0|1" "crew|tester|0|0" "full|lead|1|0" "full|architect|0|0" "full|developer|0|1" "full|tester|0|0" "relay|lead|1|0" "relay|tester|0|0"; do
	echo "$wc_out" | grep -qF "$row" || wc_ok=0
done
check write-default "$wc_ok" "seeded lead Bot write=$seed_write; team/Bot write column: $(echo "$wc_out" | tr '\n' ' ')"

api_call GET /teams/full
rest_ok=1
[ "$STATUS" = 200 ] || rest_ok=0
echo "$BODY" | grep -q '"name":"lead","lead":true,"write":false' || rest_ok=0
echo "$BODY" | grep -q '"name":"architect","lead":false,"write":false' || rest_ok=0
echo "$BODY" | grep -q '"name":"developer","lead":false,"write":true' || rest_ok=0
echo "$BODY" | grep -q '"name":"tester","lead":false,"write":false' || rest_ok=0
echo "$BODY" | grep -q '"gates"' && rest_ok=0
check full-rest "$rest_ok" "GET /teams/full: $BODY"

mkdir -p "$S/plain"
api_call POST /projects '{"name":"full","path":"'"$S"'/cwd"}'
api_call POST /projects '{"name":"plain","path":"'"$S"'/plain"}'
api_call POST /work-items '{"projectId":1,"title":"A"}'
api_call POST /work-items '{"projectId":1,"title":"B"}'
api_call POST /work-items '{"projectId":2,"title":"No git"}'
api_call POST /spaces '{"projectId":1,"workItemId":1,"name":"a1","team":"crew"}'
api_call POST /spaces '{"projectId":1,"name":"a2","team":"crew"}'
api_call POST /spaces '{"projectId":1,"workItemId":2,"name":"b1","team":"crew"}'
api_call POST /spaces '{"projectId":1,"name":"quick","team":"crew"}'
api_call POST /spaces '{"projectId":2,"workItemId":3,"name":"nogit"}'

T="$S/full/worktrees/1/1"

### Worktrees and writers

api_call POST /spaces/1/send '{"text":"@developer Write the numbers from 1 to 300 separated by spaces. Use no tools."}'
send1=$STATUS
api_call POST /spaces/3/send '{"text":"@developer Write the numbers from 1 to 300 separated by spaces. Use no tools."}'
send3=$STATUS
wait_done 1 1
wait_done 3 1

wt_list=$(git -C "$S/cwd" worktree list)
two_ok=1
[ "$send1" = 202 ] && [ "$send3" = 202 ] || two_ok=0
echo "$wt_list" | grep -q "^$S/cwd .*\[main\]" || two_ok=0
echo "$wt_list" | grep -q "^$S/full/worktrees/1/1 .*\[work-item/1\]" || two_ok=0
echo "$wt_list" | grep -q "^$S/full/worktrees/1/2 .*\[work-item/2\]" || two_ok=0
[ -z "$(git -C "$S/cwd" status --porcelain)" ] || two_ok=0
[ "$(turn_col 1 role)" = developer ] && [ "$(turn_col 1 space_id)" = 1 ] || two_ok=0
[ "$(turn_col 2 role)" = developer ] && [ "$(turn_col 2 space_id)" = 3 ] || two_ok=0
[[ "$(turn_col 1 checkpoint)" =~ ^refs/reef/checkpoints/[0-9a-f]{12}/1$ ]] || two_ok=0
[[ "$(turn_col 2 checkpoint)" =~ ^refs/reef/checkpoints/[0-9a-f]{12}/2$ ]] || two_ok=0
sess1=$(turn_col 1 session_id) sess2=$(turn_col 2 session_id)
f1=$(session_file "$sess1") f2=$(session_file "$sess2")
[ "$(grep -ho '"cwd":"[^"]*"' "$f1" 2>/dev/null | sort -u)" = "\"cwd\":\"$S/full/worktrees/1/1\"" ] || two_ok=0
[ "$(grep -ho '"cwd":"[^"]*"' "$f2" 2>/dev/null | sort -u)" = "\"cwd\":\"$S/full/worktrees/1/2\"" ] || two_ok=0
check tree-two-work-items "$two_ok" "worktree list: $(echo "$wt_list" | tr '\n' ' '); status clean; each session's cwd is its own tree"

s1=$(turn_col 1 started_at) e1=$(turn_col 1 ended_at)
s2=$(turn_col 2 started_at) e2=$(turn_col 2 ended_at)
par_ok=1
[[ "$s1" < "$e2" ]] || par_ok=0
[[ "$s2" < "$e1" ]] || par_ok=0
check writer-parallel "$par_ok" "turn 1 $s1..$e1, turn 2 $s2..$e2: each started before the other ended"

echo mark >"$T/marker.txt"
api_call POST /spaces/1/send '{"text":"@developer Write the numbers from 1 to 600 separated by spaces. Use no tools."}'
wait_done 1 2

wt_list2=$(git -C "$S/cwd" worktree list)
reuse_ok=1
[ "$wt_list2" = "$wt_list" ] || reuse_ok=0
[ "$(cat "$T/marker.txt")" = mark ] || reuse_ok=0
check tree-reuse "$reuse_ok" "worktree list unchanged, marker.txt still mark"

git -C "$T" status --porcelain >"$E/a-status-before.txt"
api_call POST /spaces/1/send '{"text":"@developer Write the numbers from 1 to 600 separated by spaces. Use no tools."}'
sleep 3

dev_line="" tester_line=""
claude_lines() {
	local p line
	for p in $(pgrep -P "$(cat "$S/serve.pid")" -x claude); do
		line=$(ps -ww -o args= -p "$p")
		case $line in
		*--disallowedTools*) tester_line=$line ;;
		*) dev_line=$line ;;
		esac
	done
}
claude_lines
wait_done 1 3
api_call POST /spaces/1/send '{"text":"@tester Use the Write tool to create the file t.txt containing the word nope. If that is refused, try once with Bash. Say what happened."}'
sleep 4
claude_lines
wait_done 1 4

t_turn=$("$H" sql "select id from turn where space_id = 1 and role = 'tester' order by id desc limit 1" | tail -1)

refused_ok=1
case $dev_line in *"--permission-mode acceptEdits"*) ;; *) refused_ok=0 ;; esac
case $dev_line in *"--allowedTools Read,Edit,Write,Bash,Task,mcp__reef__*"*) ;; *) refused_ok=0 ;; esac
case $dev_line in *--disallowedTools*) refused_ok=0 ;; esac
case $tester_line in *"--permission-mode default"*) ;; *) refused_ok=0 ;; esac
case $tester_line in *"--allowedTools Read,Bash(ls:*),mcp__reef__*"*) ;; *) refused_ok=0 ;; esac
case $tester_line in *"--disallowedTools Edit,Write,MultiEdit,NotebookEdit"*) ;; *) refused_ok=0 ;; esac
[ ! -e "$T/t.txt" ] || refused_ok=0
diff <(git -C "$T" status --porcelain) "$E/a-status-before.txt" >/dev/null || refused_ok=0
t_sess=$(turn_col "$t_turn" session_id)
t_file=$(session_file "$t_sess")
errs=0
[ -n "$t_file" ] && errs=$(grep -c '"is_error":true' "$t_file")
[ "${errs:-0}" -ge 2 ] 2>/dev/null || refused_ok=0
check write-claude-refused "$refused_ok" "dev=[$dev_line] tester=[$tester_line] t.txt absent=$([ -e "$T/t.txt" ] && echo no || echo yes) errs=$errs"

api_call GET "/turns/$t_turn"
cr_ok=1
echo "$BODY" | grep -q '"checkpoint":null' || cr_ok=0
check checkpoint-reader "$cr_ok" "turn $t_turn: $BODY"

echo before >"$T/pre.txt"
echo staged >"$T/st.txt" && git -C "$T" add st.txt
{
	git -C "$T" rev-parse HEAD
	git -C "$T" status --porcelain
	git -C "$T" ls-files --stage
} >"$E/a-before.txt"
api_call POST /spaces/1/send '{"text":"@developer Reply with one word. Use no tools."}'
wait_done 1 5
N=$("$H" sql "select max(id) from turn where space_id = 1" | tail -1)
api_call GET "/turns/$N"
cp_ok=1
echo "$BODY" | grep -Eq "\"checkpoint\":\"refs/reef/checkpoints/[0-9a-f]{12}/$N\"" || cp_ok=0
{
	git -C "$T" rev-parse HEAD
	git -C "$T" status --porcelain
	git -C "$T" ls-files --stage
} >"$E/a-after.txt"
diff "$E/a-before.txt" "$E/a-after.txt" >/dev/null || cp_ok=0
check checkpoint-untouched "$cp_ok" "turn $N checkpoint set; HEAD/status/index unchanged across the no-tool turn"

untracked_ok=1
[ "$(git -C "$T" show "$(turn_col "$N" checkpoint):pre.txt")" = before ] || untracked_ok=0
[ "$(git -C "$T" show "$(turn_col "$N" checkpoint)^2:st.txt")" = staged ] || untracked_ok=0
check checkpoint-untracked "$untracked_ok" "checkpoint $N holds pre.txt (untracked) and ^2 holds st.txt (staged)"

api_call POST /spaces/1/send '{"text":"@developer Use the Write tool to create the file d.txt containing the word yes."}'
wait_done 1 6
write_ok=1
[ "$(cat "$T/d.txt" 2>/dev/null)" = yes ] || write_ok=0
check write-claude-allowed "$write_ok" "d.txt=$(cat "$T/d.txt" 2>/dev/null)"

upgrade_out=$(go test ./store -run Upgrade -v 2>&1)
upgrade_rc=$?
check checkpoint-upgrade "$([ "$upgrade_rc" = 0 ] && echo 1 || echo 0)" "go test ./store -run Upgrade -v exit $upgrade_rc: $(echo "$upgrade_out" | tail -3 | tr '\n' ' ')"

### Rollback, space 1

snap() {
	git -C "$T" rev-parse HEAD
	git -C "$T" status --porcelain
	git -C "$T" log --oneline
	git -C "$T" ls-files --stage
	for f in $(ls "$T"); do echo "$f: $(cat "$T/$f" 2>/dev/null)"; done
}

snap >"$E/rb-before.txt"
api_call POST /spaces/1/send '{"text":"@developer Append a line with the word two to a.txt, create the file c.txt containing the word sea, then run the shell command: git add -A && git commit -qm dev-change"}'
wait_done 1 7
N=$("$H" sql "select max(id) from turn where space_id = 1" | tail -1)
api_call POST "/turns/$N/rollback"
rb_status=$STATUS
snap >"$E/rb-after.txt"
restore_ok=1
[ "$rb_status" = 204 ] || restore_ok=0
diff "$E/rb-before.txt" "$E/rb-after.txt" >/dev/null || restore_ok=0
check rollback-restores "$restore_ok" "rollback of turn $N returned $rb_status; snap before/after match (c.txt gone, a.txt back to one line)"

rb_note_ok=1
api_call GET '/messages?space=1'
echo "$BODY" | grep -qF "\"author\":\"reef\",\"body\":\"Rolled back to before turn $N.\"" || rb_note_ok=0

api_call POST "/turns/$t_turn/rollback"
[ "$STATUS" = 400 ] || rb_note_ok=0
echo "$BODY" | grep -qF "invalid: turn $t_turn has no checkpoint" || rb_note_ok=0

api_call POST /turns/999/rollback
[ "$STATUS" = 404 ] || rb_note_ok=0
echo "$BODY" | grep -qF "turn 999: not found" || rb_note_ok=0

git -C "$T" rev-parse HEAD >"$E/busy-before.txt"
git -C "$T" status --porcelain >>"$E/busy-before.txt"
api_call POST /spaces/1/send '{"text":"@developer Write the numbers from 1 to 600 separated by spaces. Use no tools."}'
sleep 2
api_call POST "/turns/$N/rollback"
busy1_status=$STATUS busy1_body=$BODY
wait_done 1 8
git -C "$T" rev-parse HEAD >"$E/busy-after.txt"
git -C "$T" status --porcelain >>"$E/busy-after.txt"

busy_ok=1
[ "$busy1_status" = 409 ] || busy_ok=0
echo "$busy1_body" | grep -qF "space 1 has a turn running: conflict" || busy_ok=0
diff "$E/busy-before.txt" "$E/busy-after.txt" >/dev/null || busy_ok=0
msgcount=$("$H" sql "select count(*) from message where space_id = 1 and author = 'reef'" | tail -1)
[ "$msgcount" = 1 ] || busy_ok=0
check rollback-busy "$busy_ok" "busy=$busy1_status; tree unchanged across the busy window; $msgcount reef message(s) on Space 1"

last_turn=$("$H" sql "select id from turn where space_id = 1 order by id desc limit 1" | tail -1)
last_wake=$("$H" sql "select wake_text from turn where id = $last_turn" | tail -n +2)
echo "$last_wake" | grep -q '^\[Space 1, message [0-9]*\]' || rb_note_ok=0
echo "$last_wake" | grep -q 'reef wrote:' && rb_note_ok=0
last_sess=$(turn_col "$last_turn" session_id)
last_file=$(session_file "$last_sess")
grep -q "reef wrote: Rolled back to before turn $N\\." "$last_file" 2>/dev/null || rb_note_ok=0
check rollback-note "$rb_note_ok" "reef message and 400/404 as expected; next wake $last_turn's wake_text has no reef line, its session prompt does"

### Quick session, file isolation and a project without git

api_call POST /spaces/4/send '{"text":"@tester Reply with one word."}'
wait_done 4 1
q_sess=$("$H" sql "select session_id from turn where space_id = 4 order by id desc limit 1" | tail -1)
q_file=$(session_file "$q_sess")
quick_ok=1
[ "$(grep -ho '"cwd":"[^"]*"' "$q_file" 2>/dev/null | sort -u)" = "\"cwd\":\"$S/full/chats/4\"" ] || quick_ok=0
check tree-quick "$quick_ok" "quick session's tester session ran with cwd $S/full/chats/4 only, never the project folder"

iso_ok=1
[ -e "$S/full/worktrees/1/1/d.txt" ] || iso_ok=0
[ ! -e "$S/full/worktrees/1/2/d.txt" ] || iso_ok=0
[ ! -e "$S/cwd/d.txt" ] || iso_ok=0
[ -z "$(git -C "$S/cwd" status --porcelain)" ] || iso_ok=0
check tree-isolated "$iso_ok" "d.txt is only in worktrees/1/1; main checkout clean"

api_call POST /spaces/5/send '{"text":"Reply with one word."}'
notgit_send_status=$STATUS
wait_for '/turns?space=5&status=failed' lead
api_call GET '/turns?space=5'
notgit_ok=1
[ "$notgit_send_status" = 202 ] || notgit_ok=0
echo "$BODY" | grep -q '"role":"lead"' || notgit_ok=0
echo "$BODY" | grep -q '"status":"failed"' || notgit_ok=0
echo "$BODY" | grep -qF "work items need the project folder to be a git repo with a commit: $S/plain:" || notgit_ok=0
grep -q "turn of lead in Space 5 failed" "$E/serve.log" || notgit_ok=0
api_call GET /health
[ "$STATUS" = 200 ] || notgit_ok=0
check tree-not-git "$notgit_ok" "space 5 lead turn failed with the not-a-git-repo error; /health still 200"

### Attach to a work item with a dirty main checkout (space 6, solo)

api_call POST /spaces '{"projectId":1,"name":"promo"}'
echo dirty >"$S/cwd/dirty.txt"
api_call POST /spaces/6/send '{"text":"Reply with one word."}'
wait_done 6 1
api_call POST /work-items '{"projectId":1,"title":"C"}'
api_call POST /spaces/6/attach '{"workItemId":4}'
attach_status=$STATUS
api_call GET '/messages?space=6'
attach_msg_ok=1
attach_note='This Space is now attached to work item \"C\" (4). It works in '"$S"'/full/worktrees/1/4 on work-item/4. Uncommitted changes stay on the main checkout.'
echo "$BODY" | grep -qF "\"author\":\"reef\",\"body\":\"$attach_note\"" || attach_msg_ok=0
api_call POST /spaces/6/send '{"text":"Reply with one word."}'
wait_done 6 2

turns6=$("$H" sql "select id, role, status, session_id from turn where space_id = 6")
sess6=$(echo "$turns6" | tail -n +2 | awk -F'|' '{print $4}' | sort -u)
attach_ok=1
[ "$attach_status" = 200 ] || attach_ok=0
[ "$attach_msg_ok" = 1 ] || attach_ok=0
[ "$(echo "$turns6" | tail -n +2 | wc -l | tr -d ' ')" = 2 ] || attach_ok=0
[ "$(echo "$sess6" | wc -l | tr -d ' ')" = 1 ] || attach_ok=0
sess6_id=$(echo "$sess6" | head -1)
sess6_file=$(session_file "$sess6_id")
grep -q "\"cwd\":\"$S/full/chats/6\"" "$sess6_file" 2>/dev/null || attach_ok=0
grep -q "\"cwd\":\"$S/full/worktrees/1/4\"" "$sess6_file" 2>/dev/null || attach_ok=0
wt_list3=$(git -C "$S/cwd" worktree list)
echo "$wt_list3" | grep -q "^$S/full/worktrees/1/4 .*\[work-item/4\]" || attach_ok=0
[ "$(git -C "$S/cwd" status --porcelain)" = "?? dirty.txt" ] || attach_ok=0
check tree-attach-dirty "$attach_ok" "attach 200 with the move note, 2 lead turns share session $sess6_id (resumed in the worktree), main checkout only ?? dirty.txt"

### Retry after a crash, space 3

api_call POST /spaces/3/send '{"text":"@developer Write the numbers from 1 to 600 separated by spaces. Use no tools."}'
sleep 3
"$H" serve-stop --kill
ANTHROPIC_MODEL=sonnet "$H" serve-start --data-name full || exit 1
A=$(cat "$S/serve.addr")
F=$("$H" sql "select max(id) from turn where space_id = 3" | tail -1)
api_call GET "/turns/$F"
retry_ok=1
echo "$BODY" | grep -q '"status":"failed"' || retry_ok=0
echo "$BODY" | grep -q '"error":"server restarted"' || retry_ok=0
echo "$BODY" | grep -q '"retried":false' || retry_ok=0
echo "$BODY" | grep -q '"wakeText":""' && retry_ok=0
api_call GET '/messages?space=3'
echo "$BODY" | grep -qF "Turn $F by developer failed: server restarted. The worktree may be dirty; retry or roll back." || retry_ok=0
api_call POST "/turns/$F/retry"
[ "$STATUS" = 202 ] || retry_ok=0
wait_done 3 2
rows3=$("$H" sql "select id, role, status, retried from turn where space_id = 3 order by id")
echo "$rows3" | grep -qF "$F|developer|failed|1" || retry_ok=0
case $(echo "$rows3" | tail -1) in *"|developer|done|0") ;; *) retry_ok=0 ;; esac
check retry-restart "$retry_ok" "turn $F failed with server restarted, then retried into a new done developer turn: $(echo "$rows3" | tr '\n' ' ')"

api_call POST "/turns/$F/retry"
twice_ok=1
[ "$STATUS" = 409 ] || twice_ok=0
echo "$BODY" | grep -qF "turn $F cannot be retried: conflict" || twice_ok=0
api_call POST /turns/1/retry
[ "$STATUS" = 409 ] || twice_ok=0
echo "$BODY" | grep -qF "turn 1 cannot be retried: conflict" || twice_ok=0
check retry-twice "$twice_ok" "re-retry of $F and retry of done turn 1 are both 409"

### Inbox (spaces 7 to 9 are quick sessions, so they make no worktree)

api_call POST /spaces '{"projectId":1,"name":"relay","team":"relay"}'
api_call POST /spaces '{"projectId":1,"name":"budget","team":"crew"}'
api_call POST /spaces '{"projectId":1,"name":"crashed","team":"crew"}'
api_call POST /spaces/9/send '{"text":"Write the numbers from 1 to 600 separated by spaces."}'
sleep 3
api_call POST /spaces/9/stop
[ "$STATUS" = 204 ] || echo "note: Space 9 stop returned $STATUS, not 204" >&2
api_call GET '/turns?space=9'
echo "$BODY" | grep -q '"status":"failed"' || echo "note: Space 9 turn not failed after stop" >&2
echo "$BODY" | grep -q '"error":"interrupted"' || echo "note: Space 9 turn error not interrupted" >&2
api_call GET /inbox
echo "$BODY" | grep -q '"spaceId":9' && echo "note: Space 9 unexpectedly in the inbox after a user stop" >&2

api_call POST /spaces/9/send '{"text":"Write the numbers from 1 to 600 separated by spaces."}'
sleep 3
"$H" serve-stop --kill
ANTHROPIC_MODEL=sonnet "$H" serve-start --data-name full || exit 1
A=$(cat "$S/serve.addr")
C=$("$H" sql "select max(id) from turn where space_id = 9" | tail -1)

curl -sN --max-time 400 "http://$A/inbox/events" >"$E/inbox-events.txt" &
inbox_stream_pid=$!

api_call POST /spaces/8/send '{"text":"Reply with one word."}'
wait_done 8 1
"$H" sql --write "update Space set token_budget = 1 where id = 8" >/dev/null
api_call POST /spaces/8/send '{"text":"Reply with one word."}'
budget_status=$STATUS budget_body=$BODY

api_call GET /inbox
inbox_kinds_ok=1
[ "$budget_status" = 409 ] || inbox_kinds_ok=0
echo "$budget_body" | grep -q 'space 8 is paused: token budget of 1 used: [0-9]* tokens: conflict' || inbox_kinds_ok=0
echo "$BODY" | grep -q "\"kind\":\"failed\",\"group\":\"needs_you\",\"spaceId\":9" || inbox_kinds_ok=0
echo "$BODY" | grep -q '"text":"lead failed: server restarted"' || inbox_kinds_ok=0
echo "$BODY" | grep -qF "\"turnId\":$C" || inbox_kinds_ok=0
echo "$BODY" | grep -q '"kind":"paused","group":"needs_you","spaceId":8' || inbox_kinds_ok=0
echo "$BODY" | grep -q '"text":"token budget of 1 used: [0-9]* tokens"' || inbox_kinds_ok=0
echo "$BODY" | grep -q '"kind":"waiting","group":"fyi","spaceId":4' || inbox_kinds_ok=0
check inbox-kinds "$inbox_kinds_ok" "inbox has failed(9), paused(8) and waiting(4) items"

api_call POST /spaces/8/continue
s1=$STATUS
api_call POST "/turns/$C/retry"
s2=$STATUS
api_call POST /spaces/4/send '{"text":"Reply with one other word."}'
s3=$STATUS
clear_ok=1
[ "$s1" = 204 ] && [ "$s2" = 202 ] && [ "$s3" = 202 ] || clear_ok=0
api_call GET /inbox
# 4 and 9 each started a new turn, so they have no item at all while it runs; 8's
# continue starts none, so its cleared paused item is at once a waiting item instead.
for ch in 4 9; do
	echo "$BODY" | grep -q "\"spaceId\":$ch" && clear_ok=0
done
echo "$BODY" | grep -q '"kind":"paused","group":"needs_you","spaceId":8' && clear_ok=0

wait_done 9 1
wait_done 4 2
api_call POST /spaces/8/send '{"text":"Reply with one word."}'
[ "$STATUS" = 202 ] || clear_ok=0
wait_done 8 2

[ "$("$H" sql "select role, status from turn where space_id = 9 order by id desc limit 1" | tail -1)" = "lead|done" ] || clear_ok=0
[ "$("$H" sql "select retried from turn where id = $C" | tail -1)" = 1 ] || clear_ok=0
[ "$("$H" sql "select role, status from turn where space_id = 4 order by id desc limit 1" | tail -1)" = "lead|done" ] || clear_ok=0
[ "$("$H" sql "select role, status from turn where space_id = 8 order by id desc limit 1" | tail -1)" = "lead|done" ] || clear_ok=0

api_call GET /inbox
for ch in 4 8 9; do
	echo "$BODY" | grep -q "\"kind\":\"waiting\",\"group\":\"fyi\",\"spaceId\":$ch" || clear_ok=0
done
check inbox-clear "$clear_ok" "continue/retry/send cleared their items (204,202,202); each Space ran on and now lists a new waiting item"

kill "$inbox_stream_pid" 2>/dev/null
wait "$inbox_stream_pid" 2>/dev/null
frames=$(grep '^data:' "$E/inbox-events.txt" | while IFS= read -r l; do echo "$l" | grep -o '"kind":"[a-z_]*","group":"[a-z_]*","spaceId":[489]' | tr '\n' ' '; echo; done)
stream_ok=1
[ -n "$frames" ] || stream_ok=0
echo "$frames" | head -1 | grep -q '"kind":"failed","group":"needs_you","spaceId":9' || stream_ok=0
echo "$frames" | grep -q '"kind":"paused","group":"needs_you","spaceId":8' || stream_ok=0
echo "$frames" | grep -q '"kind":"waiting","group":"fyi","spaceId":9' || stream_ok=0
check inbox-stream "$stream_ok" "frames across the stream: $(echo "$frames" | tr '\n' '|')"

"$H" sql --write "update Space set token_budget = 1 where id = 8" >/dev/null
api_call POST /spaces/8/send '{"text":"Reply with one word."}'
restart_budget_status=$STATUS
api_call GET /inbox
echo "$BODY" >"$E/inbox-before.txt"
"$H" serve-stop
ANTHROPIC_MODEL=sonnet "$H" serve-start --data-name full || exit 1
A=$(cat "$S/serve.addr")
api_call GET /inbox
echo "$BODY" >"$E/inbox-after.txt"
restart_ok=1
[ "$restart_budget_status" = 409 ] || restart_ok=0
diff "$E/inbox-before.txt" "$E/inbox-after.txt" >/dev/null || restart_ok=0
kinds_after=$(grep -o '"kind":"[a-z_]*","group":"[a-z_]*","spaceId":[0-9]*' "$E/inbox-after.txt")
echo "$kinds_after" | grep -q '"kind":"paused","group":"needs_you","spaceId":8' || restart_ok=0
echo "$kinds_after" | grep -q '"kind":"failed","group":"needs_you","spaceId":5' || restart_ok=0
echo "$kinds_after" | grep -q '"kind":"waiting"' || restart_ok=0
check inbox-restart "$restart_ok" "inbox before/after a restart match byte for byte: $(echo "$kinds_after" | tr '\n' ' ')"
}

section_cx() {
### Codex on the test fixture (write-codex-readonly), folder cx

mkdir -p "$S/spy"
printf '#!/bin/sh\ntee -a %s/codex-in.log | %s/agent/codex/testdata/codex "$@"\n' "$E" "$PWD" >"$S/spy/codex"
chmod +x "$S/spy/codex"
BASH_ENV= FAKE_CODEX=replay PATH="$S/spy:$PATH" ANTHROPIC_MODEL=sonnet "$H" serve-start --data-name cx || exit 1
A=$(cat "$S/serve.addr")
"$H" team-apply "$E/cx.json" >/dev/null
api_call POST /projects '{"name":"cx","path":"'"$S"'/cwd"}'
api_call POST /spaces '{"projectId":1,"name":"cx room","team":"cx"}'
api_call POST /spaces/1/send '{"text":"@tester hi"}'
wait_done 1 1
api_call POST /spaces/1/send '{"text":"@developer hi"}'
wait_done 1 2

codex_ok=1
rows_cx=$("$H" sql "select id, role, backend, status from turn")
echo "$rows_cx" | grep -qF '1|tester|codex|done' || codex_ok=0
echo "$rows_cx" | grep -qF '2|developer|codex|done' || codex_ok=0
sandbox_seq=$(grep 'thread/start' "$E/codex-in.log" | grep -o '"sandbox":"[a-z-]*"')
first_sandbox=$(echo "$sandbox_seq" | sed -n 1p)
second_sandbox=$(echo "$sandbox_seq" | sed -n 2p)
[ "$first_sandbox" = '"sandbox":"read-only"' ] || codex_ok=0
[ "$second_sandbox" = '"sandbox":"workspace-write"' ] || codex_ok=0
check write-codex-readonly "$codex_ok" "turns: $(echo "$rows_cx" | tr '\n' ' '); sandbox sequence: $first_sandbox then $second_sandbox"
}

section_ui2() {
ANTHROPIC_MODEL=sonnet "$H" serve-start --data-name ui2 || exit 1
run_ui window-rollback rollback
run_ui window-retry retry
}

section_google() {
### Google login end to end (google-login), folder google, real claude on sonnet

mkdir -p "$S/google-app"
cat >"$S/google-app/go.mod" <<'EOF'
module example.com/shop

go 1.22
EOF
cat >"$S/google-app/main.go" <<'EOF'
package main

import "net/http"

func routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/login", loginHandler)
	return mux
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		w.WriteHeader(http.StatusOK)
		return
	}
	w.Write([]byte(`<form method="post" action="/login">
<input name="email" type="email">
<input name="password" type="password">
<button type="submit">Sign in</button>
</form>`))
}

func main() {
	http.ListenAndServe(":8080", routes())
}
EOF
cat >"$S/google-app/main_test.go" <<'EOF'
package main

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoginPage(t *testing.T) {
	srv := httptest.NewServer(routes())
	defer srv.Close()
	resp, err := srv.Client().Get(srv.URL + "/login")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "Sign in") {
		t.Fatalf("login page missing Sign in: %s", body)
	}
}
EOF
google_app_test_out=$(cd "$S/google-app" && go test ./... 2>&1)
google_app_test_rc=$?
git -C "$S/google-app" init -q -b main
git -C "$S/google-app" add -A
git -C "$S/google-app" -c user.name=v -c user.email=v@example.com commit -qm "shop with email login" >/dev/null

ANTHROPIC_MODEL=sonnet "$H" serve-start --data-name google || exit 1
A=$(cat "$S/serve.addr")
"$H" team-apply "$PWD/teams/full.json" >/dev/null
api_call POST /projects '{"name":"shop","path":"'"$S"'/google-app"}'
api_call POST /work-items '{"projectId":1,"title":"Google login"}'
api_call POST /spaces '{"projectId":1,"workItemId":1,"name":"google","team":"full"}'

curl -sN --max-time 1800 "http://$A/inbox/events" >"$E/google-inbox.txt" &
google_inbox_pid=$!
curl -sN --max-time 1800 "http://$A/spaces/1/events" >"$E/google-events.txt" &
google_events_pid=$!

idle() {
	for i in $(seq 120); do
		[ "$(curl -s "http://$A/turns?space=1&status=running")" = "[]" ] && {
			sleep 4
			[ "$(curl -s "http://$A/turns?space=1&status=running")" = "[]" ] && return 0
		}
		sleep 5
	done
	echo "timeout waiting for Space 1 to idle" >&2
	return 1
}

google_ok=1 google_bad=
[ "$google_app_test_rc" = 0 ] || { google_ok=0; google_bad+=" $LINENO"; }

api_call POST /spaces/1/send '{"text":"Add Google login to this app: a Sign in with Google link on the login page that goes to /auth/google, which redirects to the Google OAuth URL with the client id from the GOOGLE_CLIENT_ID env var. Keep it small and covered by a test."}'
idle

wait_done 1 5
rows_g=$("$H" sql "select id, role, status from turn where space_id = 1 order by id")
for row in '1|lead|done' '2|architect|done' '3|developer|done' '4|tester|done' '5|lead|done'; do
	echo "$rows_g" | grep -qF "$row" || { google_ok=0; google_bad+=" $LINENO"; }
done
[ "$("$H" sql "select count(*) from approval" | tail -1)" = 0 ] || { google_ok=0; google_bad+=" $LINENO"; }

dev_turn_g=$("$H" sql "select id from turn where space_id = 1 and role = 'developer' order by id desc limit 1" | tail -1)
[ "$(turn_col "$dev_turn_g" status)" = done ] || { google_ok=0; google_bad+=" $LINENO"; }
[[ "$(turn_col "$dev_turn_g" checkpoint)" =~ ^refs/reef/checkpoints/[0-9a-f]{12}/$dev_turn_g$ ]] || { google_ok=0; google_bad+=" $LINENO"; }
[ "$(git -C "$S/google-app" log --oneline main..work-item/1 | wc -l | tr -d ' ')" = 1 ] || { google_ok=0; google_bad+=" $LINENO"; }
git -C "$S/google-app" worktree list | grep -q "^$S/google/worktrees/1/1 .*\[work-item/1\]" || { google_ok=0; google_bad+=" $LINENO"; }
[ -z "$(git -C "$S/google-app" status --porcelain)" ] || { google_ok=0; google_bad+=" $LINENO"; }
[ "$(git -C "$S/google-app" log --oneline main | wc -l | tr -d ' ')" = 1 ] || { google_ok=0; google_bad+=" $LINENO"; }

tester_turn_g=$("$H" sql "select id from turn where space_id = 1 and role = 'tester' order by id desc limit 1" | tail -1)
[ "$(turn_col "$tester_turn_g" status)" = done ] || { google_ok=0; google_bad+=" $LINENO"; }
[[ "$(turn_col "$tester_turn_g" checkpoint)" =~ ^refs/reef/checkpoints/[0-9a-f]{12}/$tester_turn_g$ ]] || { google_ok=0; google_bad+=" $LINENO"; }
lead_turn_g=$("$H" sql "select id from turn where space_id = 1 and role = 'lead' order by id desc limit 1" | tail -1)
[ "$(turn_col "$lead_turn_g" status)" = done ] || { google_ok=0; google_bad+=" $LINENO"; }
[ -z "$(echo "$("$H" sql "select paused from space")" | tail -n +2 | tr -d '[:space:]')" ] || { google_ok=0; google_bad+=" $LINENO"; }
api_call GET /inbox
[ "$(echo "$BODY" | grep -o '"kind"' | wc -l | tr -d ' ')" = 1 ] || { google_ok=0; google_bad+=" $LINENO"; }
echo "$BODY" | grep -q '"kind":"waiting"' || { google_ok=0; google_bad+=" $LINENO"; }

api_call GET '/messages?space=1'
api_call GET '/turns?space=1'
git -C "$S/google-app" log --stat main..work-item/1 >"$E/google-log-stat.txt"

kill "$google_inbox_pid" 2>/dev/null
wait "$google_inbox_pid" 2>/dev/null
kill "$google_events_pid" 2>/dev/null
wait "$google_events_pid" 2>/dev/null
[ "$(grep -c '^event: approval' "$E/google-events.txt")" = 0 ] || { google_ok=0; google_bad+=" $LINENO"; }

check google-login "$google_ok" "${google_bad:+failed at full-team.sh lines$google_bad; }turns: $(echo "$rows_g" | tr '\n' ' '); lead, architect, developer, tester and lead ran without approval; checkpoints, worktree and clean main checkout match"
}

section_fu() {
### Handoffs, notes, retry and rollback rules, folder fu

prompt() { grep -h '"type":"user"' "$HOME"/.claude/projects/*/"$1".jsonl 2>/dev/null | grep -o '"content":"\[Space[^"]*' | tail -1; }

cat >"$E/pair.json" <<'EOF'
{
  "name": "pair",
  "roles": [
    {"name": "lead", "lead": true, "backend": "claude", "write": true,
     "description": "You are the lead. Do exactly what the user asks. Never @mention anyone unless the user tells you to."},
    {"name": "developer", "write": true, "backend": "claude", "permissionMode": "acceptEdits",
     "description": "You are the developer. Do what the user asks with as few tool calls as you can, then answer in one short sentence. Never @mention anyone.",
     "tools": ["Read", "Edit", "Write", "Bash"]},
    {"name": "tester", "backend": "claude", "permissionMode": "acceptEdits",
     "description": "You are the tester. Do what the user asks, then answer in one short sentence. Never @mention anyone.",
     "tools": ["Read", "Edit", "Write", "Bash(ls:*)"]}
  ]
}
EOF
ANTHROPIC_MODEL=sonnet "$H" serve-start --data-name fu || exit 1
A=$(cat "$S/serve.addr")
"$H" team-apply "$PWD/teams/full.json" >/dev/null
"$H" team-apply "$E/pair.json" >/dev/null

api_call POST /projects '{"name":"main","path":"'"$S"'/cwd"}'
api_call POST /work-items '{"projectId":1,"title":"FA"}'
api_call POST /work-items '{"projectId":1,"title":"FB"}'
api_call POST /work-items '{"projectId":1,"title":"FC"}'
api_call POST /spaces '{"projectId":1,"workItemId":1,"name":"fa1","team":"pair"}'
api_call POST /spaces '{"projectId":1,"name":"fa2","team":"pair"}'
api_call POST /spaces '{"projectId":1,"workItemId":2,"name":"fb1","team":"full"}'
api_call POST /spaces '{"projectId":1,"workItemId":3,"name":"fc1","team":"full"}'
WA="$S/fu/worktrees/1/1"
P='Reply with exactly one line and nothing else: an at sign immediately followed by the word developer, then a space, then the words: reply with one word, use no tools.'

api_call POST /spaces/1/send '{"text":"@developer Use the Write tool to create the file f.txt containing the word one."}'
wait_done 1 1
api_call POST /spaces/1/send "{\"text\":\"$P\"}"
wait_done 1 3
handoff_ok=1
rows_handoff=$("$H" sql "select role, status from turn where space_id = 1 order by id")
echo "$rows_handoff" | grep -qF 'lead|done' || handoff_ok=0
[ "$(echo "$rows_handoff" | grep -c 'developer|done')" = 2 ] || handoff_ok=0
[ "$("$H" sql "select count(*) from approval where space_id = 1" | tail -1)" = 0 ] || handoff_ok=0
check handoff-no-approval "$handoff_ok" "lead woke developer automatically with no approval row: $(echo "$rows_handoff" | tr '\n' ' ')"

api_call POST /spaces/1/send '{"text":"@developer Write the numbers from 1 to 600 separated by spaces. Use no tools."}'
sleep 4
"$H" serve-stop --kill
ANTHROPIC_MODEL=sonnet "$H" serve-start --data-name fu || exit 1
A=$(cat "$S/serve.addr")
F2=$("$H" sql "select id from turn where space_id = 1 order by id desc limit 1" | tail -1)
api_call GET /inbox
note_retry_ok=1
echo "$BODY" | grep -qF "\"kind\":\"failed\",\"group\":\"needs_you\",\"spaceId\":1" || note_retry_ok=0
echo "$BODY" | grep -qF '"text":"developer failed: server restarted"' || note_retry_ok=0
echo "$BODY" | grep -qF "\"turnId\":$F2" || note_retry_ok=0
api_call POST "/turns/$F2/rollback"
[ "$STATUS" = 204 ] || note_retry_ok=0
api_call POST "/turns/$F2/retry"
[ "$STATUS" = 202 ] || note_retry_ok=0
wait_done 1 4
sess_f2=$("$H" sql "select session_id from turn where space_id = 1 order by id desc limit 1" | tail -1)
file_f2=$(session_file "$sess_f2")
grep -qF "reef wrote: Turn $F2 by developer failed: server restarted. The worktree may be dirty; retry or roll back." "$file_f2" 2>/dev/null || note_retry_ok=0
grep -q "reef wrote: Rolled back to before turn $F2\\." "$file_f2" 2>/dev/null || note_retry_ok=0
api_call POST /spaces/1/send '{"text":"@developer Reply with one word. Use no tools."}'
wait_done 1 5
sess_after=$("$H" sql "select session_id from turn where space_id = 1 order by id desc limit 1" | tail -1)
last_p=$(prompt "$sess_after")
echo "$last_p" | grep -q 'reef wrote:' && note_retry_ok=0
check note-retry "$note_retry_ok" "failed/rollback/retry notes land in the retried prompt; the next wake's prompt has none"

api_call POST /spaces/1/send '{"text":"@developer Write the numbers from 1 to 600 separated by spaces. Use no tools."}'
sleep 4
"$H" serve-stop --kill
ANTHROPIC_MODEL=sonnet "$H" serve-start --data-name fu || exit 1
A=$(cat "$S/serve.addr")
G=$("$H" sql "select id from turn where space_id = 1 order by id desc limit 1" | tail -1)
api_call POST /spaces/1/send '{"text":"@developer Reply with one word. Use no tools."}'
wait_done 1 6
api_call POST "/turns/$G/retry"
superseded_ok=1
[ "$STATUS" = 409 ] || superseded_ok=0
echo "$BODY" | grep -qF "turn $G cannot be retried: conflict" || superseded_ok=0
api_call GET '/turns?space=1&retryable=1'
[ "$BODY" = "[]" ] || superseded_ok=0
api_call GET /inbox
echo "$BODY" | grep -q "\"kind\":\"failed\",\"group\":\"needs_you\",\"spaceId\":1" && superseded_ok=0
check retry-superseded "$superseded_ok" "retry of the superseded turn $G is 409, not retryable, not in the inbox"

api_call POST /spaces/1/wrap
wrap_send_status=$STATUS
sleep 4
"$H" serve-stop --kill
ANTHROPIC_MODEL=sonnet "$H" serve-start --data-name fu || exit 1
A=$(cat "$S/serve.addr")
newest1=$("$H" sql "select id from turn where space_id = 1 order by id desc limit 1" | tail -1)
row_wrap=$("$H" sql "select role, status, error, wake_text is null from turn where id = $newest1" | tail -1)
retry_wrap_ok=1
[ "$wrap_send_status" = 202 ] || retry_wrap_ok=0
[ "$row_wrap" = "lead|failed|server restarted|1" ] || retry_wrap_ok=0
api_call POST "/turns/$newest1/retry"
[ "$STATUS" = 409 ] || retry_wrap_ok=0
echo "$BODY" | grep -qF "turn $newest1 cannot be retried: conflict" || retry_wrap_ok=0
api_call GET '/turns?space=1&retryable=1'
[ "$BODY" = "[]" ] || retry_wrap_ok=0
api_call GET /inbox
echo "$BODY" | grep -q "\"spaceId\":1" && retry_wrap_ok=0
check retry-wrap "$retry_wrap_ok" "a failed wrap-up turn keeps no wake: retry 409, not retryable, not in the inbox"

api_call POST /spaces '{"projectId":1,"name":"q"}'
q_chan=$("$H" sql "select id from Space where name = 'q'" | tail -1)
api_call POST "/spaces/$q_chan/send" '{"text":"Reply with one word."}'
wait_done "$q_chan" 1
Q=$("$H" sql "select id from turn where space_id = $q_chan order by id desc limit 1" | tail -1)
[ -z "$(turn_col "$Q" checkpoint)" ] || echo "note: quick session turn $Q has a checkpoint" >&2
api_call POST /work-items '{"projectId":1,"title":"FQ"}'
api_call POST "/spaces/$q_chan/attach" '{"workItemId":4}'
api_call POST "/spaces/$q_chan/send" '{"text":"Reply with one word."}'
wait_done "$q_chan" 2
{
	git -C "$S/fu/worktrees/1/4" rev-parse HEAD
	git -C "$S/fu/worktrees/1/4" status --porcelain
	ls -a "$S/fu/worktrees/1/4"
} >"$E/fq-before.txt"
api_call POST "/turns/$Q/rollback"
moved_ok=1
[ "$STATUS" = 400 ] || moved_ok=0
echo "$BODY" | grep -qF "turn $Q has no checkpoint" || moved_ok=0
[ -z "$(turn_col "$Q" checkpoint)" ] || moved_ok=0
{
	git -C "$S/fu/worktrees/1/4" rev-parse HEAD
	git -C "$S/fu/worktrees/1/4" status --porcelain
	ls -a "$S/fu/worktrees/1/4"
} >"$E/fq-after.txt"
diff "$E/fq-before.txt" "$E/fq-after.txt" >/dev/null || moved_ok=0
check rollback-moved "$moved_ok" "rollback of a free Space's turn $Q, which has no checkpoint, is 400 after the attach too; the fq tree is unchanged"


H0=$(git -C "$WA" rev-parse HEAD)
api_call POST /spaces/1/send '{"text":"@developer Run this one shell command and nothing else: git checkout -q -b side && echo s > s.txt && git add s.txt && git commit -qm side"}'
wait_done 1 7
D2=$("$H" sql "select id from turn where space_id = 1 order by id desc limit 1" | tail -1)
branch_ok=1
[ "$(git -C "$WA" branch --show-current)" = side ] || branch_ok=0
api_call POST "/turns/$D2/rollback"
[ "$STATUS" = 204 ] || branch_ok=0
[ "$(git -C "$WA" branch --show-current)" = work-item/1 ] || branch_ok=0
[ "$(git -C "$WA" rev-parse HEAD)" = "$H0" ] || branch_ok=0
[ "$(git -C "$WA" rev-parse work-item/1)" = "$H0" ] || branch_ok=0
check rollback-branch "$branch_ok" "after the developer switched to side, rollback of turn $D2 returns to work-item/1 at $H0"

api_call POST /work-items '{"projectId":1,"title":"FD"}'
fd_item=$("$H" sql "select max(id) from work_item" | tail -1)
api_call POST /spaces '{"projectId":1,"workItemId":'"$fd_item"',"name":"solo fa"}'
solofa_chan=$("$H" sql "select id from Space where name = 'solo fa'" | tail -1)
api_call POST "/spaces/$solofa_chan/send" '{"text":"Reply with one word."}'
wait_done "$solofa_chan" 1
Cchat=$("$H" sql "select id from turn where space_id = $solofa_chan order by id desc limit 1" | tail -1)
api_call POST "/spaces/$solofa_chan/stop"
(sleep 30 | ANTHROPIC_MODEL=sonnet "$S/bin/reef" chat --space "$solofa_chan" --data "$S/fu" >"$E/chat-open.txt" 2>&1 &)
sleep 8
api_call POST "/turns/$Cchat/rollback"
chat_ok=1
[ "$STATUS" = 409 ] || chat_ok=0
echo "$BODY" | grep -qF "space $solofa_chan is open elsewhere: conflict" || chat_ok=0
check rollback-chat "$chat_ok" "rollback of a checkpointed turn while its Space is open in reef chat is 409"

mkdir -p "$S/fresh"
git -C "$S/fresh" init -q -b main
api_call POST /projects '{"name":"fresh","path":"'"$S"'/fresh"}'
fresh_id=$("$H" sql "select id from project where name = 'fresh'" | tail -1)
api_call POST /spaces '{"projectId":'"$fresh_id"',"name":"fresh q"}'
freshq_chan=$("$H" sql "select id from Space where name = 'fresh q'" | tail -1)
api_call POST "/spaces/$freshq_chan/send" '{"text":"Reply with one word."}'
wait_done "$freshq_chan" 1
api_call GET "/turns?space=$freshq_chan"
unborn_ok=1
echo "$BODY" | grep -q '"status":"done"' || unborn_ok=0
echo "$BODY" | grep -q '"checkpoint":null' || unborn_ok=0
check checkpoint-unborn "$unborn_ok" "a quick session associated with a project whose git repo has no commit: its lead turn is done with no checkpoint"

rm -rf "$S/fu/worktrees/1/2"
api_call POST /spaces/3/send '{"text":"@developer Reply with one word. Use no tools."}'
wait_done 3 1
recreate_ok=1
newest3=$("$H" sql "select id from turn where space_id = 3 order by id desc limit 1" | tail -1)
[ "$(turn_col "$newest3" role)" = developer ] || recreate_ok=0
[ "$(turn_col "$newest3" status)" = done ] || recreate_ok=0
[ "$(turn_col "$newest3" "checkpoint is not null")" = 1 ] || recreate_ok=0
ls "$S/fu/worktrees/1/2" 2>/dev/null | grep -qx a.txt || recreate_ok=0
git -C "$S/cwd" worktree list | grep -q "^$S/fu/worktrees/1/2 .*\[work-item/2\]" || recreate_ok=0
check tree-recreate "$recreate_ok" "worktree fb was deleted by hand; the next wake remade it with no error"

mkdir -p "$S/p157"
git -C "$S/p157" init -q -b main
echo one >"$S/p157/a.txt"
git -C "$S/p157" add a.txt
git -C "$S/p157" -c user.name=v -c user.email=v@example.com commit -qm init
api_call POST /projects '{"name":"p157","path":"'"$S"'/p157"}'
p157_id=$("$H" sql "select id from project where name = 'p157'" | tail -1)
api_call POST /work-items '{"projectId":'"$p157_id"',"title":"X"}'
x_item_id=$("$H" sql "select id from work_item where title = 'X'" | tail -1)
git -C "$S/p157" checkout -q -b "work-item/$x_item_id"
api_call POST /spaces '{"projectId":'"$p157_id"',"workItemId":'"$x_item_id"',"name":"x1"}'
x1_chan=$("$H" sql "select id from Space where name = 'x1'" | tail -1)
api_call POST "/spaces/$x1_chan/send" '{"text":"Reply with one word."}'
wait_for "/turns?space=$x1_chan&status=failed" lead
api_call GET "/turns?space=$x1_chan&status=failed"
branch_taken_ok=1
echo "$BODY" | grep -qF "branch work-item/$x_item_id is checked out at $S/p157; switch that checkout to another branch" || branch_taken_ok=0
check tree-branch-taken "$branch_taken_ok" "a wake of work item X failed naming the taken branch and the main checkout"

mkdir -p "$S/p157c"
git -C "$S/p157c" init -q -b main
echo one >"$S/p157c/a.txt"
git -C "$S/p157c" add a.txt
git -C "$S/p157c" -c user.name=v -c user.email=v@example.com commit -qm init
api_call POST /projects '{"name":"p157c","path":"'"$S"'/p157c"}'
p157c_id=$("$H" sql "select id from project where name = 'p157c'" | tail -1)
api_call POST /spaces '{"projectId":'"$p157c_id"',"name":"clean q"}'
cleanq_chan=$("$H" sql "select id from Space where name = 'clean q'" | tail -1)
api_call POST "/spaces/$cleanq_chan/send" '{"text":"Reply with one word."}'
wait_done "$cleanq_chan" 1
clean_ok=1
[ -z "$(git -C "$S/p157c" status --porcelain)" ] || clean_ok=0
api_call POST /work-items '{"projectId":'"$p157c_id"',"title":"PC"}'
pc_id=$("$H" sql "select id from work_item where title = 'PC'" | tail -1)
api_call POST "/spaces/$cleanq_chan/attach" '{"workItemId":'"$pc_id"'}'
api_call GET "/messages?space=$cleanq_chan"
clean_note='This Space is now attached to work item \"PC\" ('"$pc_id"'). It works in '"$S"'/fu/worktrees/'"$p157c_id/$pc_id"' on work-item/'"$pc_id"'.'
echo "$BODY" | grep -qF "\"author\":\"reef\",\"body\":\"$clean_note\"" || clean_ok=0
check attach-clean "$clean_ok" "attach on a clean main checkout: the reef note names only the worktree path and branch"

mkdir -p "$S/p153/.claude"
git -C "$S/p153" init -q -b main
echo one >"$S/p153/a.txt"
git -C "$S/p153" add a.txt
git -C "$S/p153" -c user.name=v -c user.email=v@example.com commit -qm init
printf '{"permissions":{"allow":["Edit","Write","Edit(*)","Write(*)"]}}' >"$S/p153/.claude/settings.json"
api_call POST /projects '{"name":"p153","path":"'"$S"'/p153"}'
p153_id=$("$H" sql "select id from project where name = 'p153'" | tail -1)
api_call POST /spaces '{"projectId":'"$p153_id"',"name":"settings q","team":"pair"}'
settingsq_chan=$("$H" sql "select id from Space where name = 'settings q'" | tail -1)
api_call POST "/spaces/$settingsq_chan/send" '{"text":"@tester Use the Edit tool to replace the word one with nope in a.txt. Say what happened."}'
wait_done "$settingsq_chan" 1
settings_ok=1
[ "$(cat "$S/p153/a.txt")" = one ] || settings_ok=0
[ "$(git -C "$S/p153" status --porcelain)" = "?? .claude/" ] || settings_ok=0
settings_sess=$("$H" sql "select session_id from turn where space_id = $settingsq_chan order by id desc limit 1" | tail -1)
settings_file=$(session_file "$settings_sess")
if [ -n "$settings_file" ]; then grep -q '"is_error":true' "$settings_file" || settings_ok=0; else settings_ok=0; fi
check write-settings "$settings_ok" "a project .claude/settings.json allowing Edit still refuses the tester"

### Space kinds and members

api_call POST /work-items '{"projectId":1,"title":"K"}'
k_item=$("$H" sql "select max(id) from work_item" | tail -1)
api_call POST /spaces '{"projectId":1,"workItemId":'"$k_item"',"name":"k1","team":"pair"}'
api_call POST /spaces '{"projectId":1,"workItemId":'"$k_item"',"name":"k2"}'
active_ok=1
[ "$STATUS" = 409 ] || active_ok=0
echo "$BODY" | grep -qF "{\"error\":\"conflict: work item $k_item already has an active Space\"}" || active_ok=0
[ "$("$H" sql "select count(*) from space where work_item_id = $k_item" | tail -1)" = 1 ] || active_ok=0
check space-active-one "$active_ok" "a second Space for work item $k_item is $STATUS $BODY"

api_call POST /spaces '{"projectId":1,"name":"free k"}'
free_k=$("$H" sql "select id from space where name = 'free k'" | tail -1)
api_call POST /work-items '{"projectId":1,"title":"K2"}'
k2_item=$("$H" sql "select max(id) from work_item" | tail -1)
api_call POST "/spaces/$free_k/attach" '{"workItemId":'"$k2_item"'}'
attach_list_ok=1
[ "$STATUS" = 200 ] || attach_list_ok=0
api_call GET "/spaces?workItem=$k2_item"
echo "$BODY" | grep -q "\"id\":$free_k,\"kind\":\"work_item\"" || attach_list_ok=0
check space-attach-list "$attach_list_ok" "free Space $free_k attached to work item $k2_item shows in GET /spaces?workItem=: $BODY"

api_call POST /bots '{"name":"dev","backend":"claude","write":true,"tools":["Read","Edit"]}'
dev_bot=$("$H" sql "select id from bot where name = 'dev'" | tail -1)
api_call PUT /teams/frozen '{"name":"frozen","roles":[{"botId":1,"lead":true},{"botId":'"$dev_bot"'}]}'
api_call POST /spaces '{"projectId":1,"name":"frozen room","team":"frozen"}'
frozen_chan=$("$H" sql "select id from space where name = 'frozen room'" | tail -1)
live_ok=1
api_call GET "/spaces/$frozen_chan/members"
echo "$BODY" | grep -qF '"name":"dev","lead":false,"write":true' || live_ok=0
echo "$BODY" | grep -qF '"tools":["Read","Edit"]' || live_ok=0
[ "$("$H" sql "select bot_id || '|' || lead from space_member where space_id = $frozen_chan order by id" | tail -n +2 | tr '\n' ' ')" = "1|1 $dev_bot|0 " ] || live_ok=0
api_call PUT "/bots/$dev_bot" '{"name":"dev","backend":"claude","tools":["Read"]}'
api_call GET "/spaces/$frozen_chan/members"
echo "$BODY" | grep -qF '"name":"dev","lead":false,"write":false' || live_ok=0
echo "$BODY" | grep -qF '"tools":["Read"]' || live_ok=0
check space-members-live "$live_ok" "a Space member holds only its Bot id; after the Bot dev lost write and Edit, the running Space shows it: $BODY"
}

# section NAME: run section_NAME under the run id $REEF_VERIFY_RUN/NAME, so it has its own
# scratch folder, evidence folder and server; its output goes to $E/NAME/full-team.log.
section() (
	export REEF_VERIFY_RUN=$REEF_VERIFY_RUN/$1
	"$H" build >/dev/null || exit 1
	"$H" doctor >/dev/null
	cp "$E/crew.json" "$E/cx.json" "$E/relay.json" "$E/$1/"
	E=$E/$1 S=$S/$1
	{
		git -C "$S/cwd" rev-parse --verify HEAD >/dev/null 2>&1 || {
			git -C "$S/cwd" init -q -b main
			echo one >"$S/cwd/a.txt"
			git -C "$S/cwd" add a.txt
			git -C "$S/cwd" -c user.name=v -c user.email=v@example.com commit -qm init
		}
		"section_$1"
		"$H" serve-stop
	} >"$E/full-team.log" 2>&1
)

# Both window sections would run npm ci in scripts/ui at the same time.
ui=.claude/skills/verify-reef/scripts/ui
[ -d "$ui/node_modules" ] || (cd "$ui" && npm ci >&2)

for sec in $sections; do
	section "$sec" &
done
wait

if [ "$sections" != "$all_sections" ]; then
	for sec in $sections; do
		lines=$(grep -h '^\(PASS\|FAIL\) ' "$E/$sec/full-team.log" 2>/dev/null)
		if [ -z "$lines" ]; then
			not_run "section-$sec" "no PASS or FAIL line in $E/$sec/full-team.log"
			continue
		fi
		echo "$lines"
		echo "$lines" | grep -q '^FAIL' && failed=1
	done
else
	for id in $(sed -n '/^## Sub-features/,/^## /s/^- `\([a-z0-9-]*\)`.*/\1/p' "$(dirname "$0")/full-team.md"); do
		line=$(grep -h "^\(PASS\|FAIL\) $id:" "$E"/*/full-team.log 2>/dev/null | head -1)
		if [ -n "$line" ]; then
			echo "$line"
			case $line in FAIL*) failed=1 ;; esac
		else
			not_run "$id" "no PASS or FAIL line in $E/*/full-team.log"
		fi
	done
fi

rm -rf "$HOME"/.claude/projects/"$(printf '%s' "$S" | sed 's/[^A-Za-z0-9]/-/g')"-*

exit "$failed"
