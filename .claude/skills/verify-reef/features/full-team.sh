#!/usr/bin/env bash
# Drives every line of full-team.md and prints one PASS/FAIL line per sub-feature id.
# Called by `verify-reef feature full-team`, which exports H, REEF_VERIFY_RUN, E, S and
# runs from the repo root. See ../SKILL.md and full-team.md (the recipe).
set -u
: "${H:?}" "${REEF_VERIFY_RUN:?}" "${E:?}" "${S:?}"
export REEF_VERIFY_RUN

failed=0
check() { # check ID OK DETAIL
	if [ "$2" = 1 ]; then echo "PASS $1: $3"; else echo "FAIL $1: $3"; failed=1; fi
}
not_run() { echo "FAIL $1: not run: $2"; failed=1; }

wait_done() { for i in $(seq 60); do [ "$(curl -s "http://$A/turns?channel=$1&status=done" | grep -o '"id"' | wc -l | tr -d ' ')" = "$2" ] && { sleep 1; return 0; }; sleep 3; done; echo "timeout: channel $1 never had $2 done turns" >&2; return 1; }
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

turn_col() { "$H" sql "select $2 from turn where id = $1" | tail -1; } # turn_col ID COLUMN(S)

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
    {"name": "lead", "lead": true, "backend": "claude",
     "persona": "You are the lead. Whatever you are told, reply with exactly one line: @tester check this"},
    {"name": "tester", "backend": "claude",
     "persona": "You are the tester. Answer in one short sentence. Never @mention anyone."}
  ],
  "gates": [{"from": "lead", "to": "tester", "on": true}]
}
EOF

section_full() {
### Teams, no server needed (data folder full)

apply_full=$("$H" reef --data-name full -- team apply "$PWD/teams/full.json")
"$H" reef -- team apply "$E/crew.json" >/dev/null
"$H" reef -- team apply "$E/cx.json" >/dev/null
"$H" reef -- team apply "$E/relay.json" >/dev/null
team_list=$("$H" reef -- team list)

apply_ok=1
case $apply_full in *"applied team full: lead*(claude) architect(claude) developer(claude) tester(claude)"*) ;; *) apply_ok=0 ;; esac
case $team_list in *"full  lead*(claude) architect(claude) developer(claude) tester(claude)"*) ;; *) apply_ok=0 ;; esac
check full-apply "$apply_ok" "team apply full printed the roster; team list has the full line"

wc_out=$("$H" sql "select team.name, role.name, lead, write from role join team on team.id = team_id order by team.name, role.id")
wc_ok=1
for row in "crew|lead|1|0" "crew|developer|0|1" "crew|tester|0|0" "full|lead|1|0" "full|architect|0|0" "full|developer|0|1" "full|tester|0|0" "relay|lead|1|0" "relay|tester|0|0" "solo|lead|1|1"; do
	echo "$wc_out" | grep -qF "$row" || wc_ok=0
done
check write-default "$wc_ok" "role/team write column: $(echo "$wc_out" | tr '\n' ' ')"

### Server (data folder full)

ANTHROPIC_MODEL=sonnet "$H" serve-start --data-name full
A=$(cat "$S/serve.addr")

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
api_call POST /features '{"projectId":1,"slug":"a","name":"A"}'
api_call POST /features '{"projectId":1,"slug":"b","name":"B"}'
api_call POST /features '{"projectId":2,"slug":"ng","name":"No git"}'
api_call POST /channels '{"projectId":1,"featureId":1,"name":"a1","team":"crew"}'
api_call POST /channels '{"projectId":1,"featureId":1,"name":"a2","team":"crew"}'
api_call POST /channels '{"projectId":1,"featureId":2,"name":"b1","team":"crew"}'
api_call POST /channels '{"projectId":1,"name":"quick","team":"crew"}'
api_call POST /channels '{"projectId":2,"featureId":3,"name":"nogit"}'

T="$S/full/worktrees/1/a"

### Worktrees and writers

api_call POST /channels/1/send '{"text":"@developer Write the numbers from 1 to 300 separated by spaces. Use no tools."}'
send1=$STATUS
api_call POST /channels/3/send '{"text":"@developer Write the numbers from 1 to 300 separated by spaces. Use no tools."}'
send3=$STATUS
wait_done 1 1
wait_done 3 1

wt_list=$(git -C "$S/cwd" worktree list)
two_ok=1
[ "$send1" = 202 ] && [ "$send3" = 202 ] || two_ok=0
echo "$wt_list" | grep -q "^$S/cwd .*\[main\]" || two_ok=0
echo "$wt_list" | grep -q "^$S/full/worktrees/1/a .*\[feature/a\]" || two_ok=0
echo "$wt_list" | grep -q "^$S/full/worktrees/1/b .*\[feature/b\]" || two_ok=0
[ -z "$(git -C "$S/cwd" status --porcelain)" ] || two_ok=0
[ "$(turn_col 1 role)" = developer ] && [ "$(turn_col 1 channel_id)" = 1 ] || two_ok=0
[ "$(turn_col 2 role)" = developer ] && [ "$(turn_col 2 channel_id)" = 3 ] || two_ok=0
[ "$(turn_col 1 checkpoint)" = refs/reef/checkpoints/1 ] || two_ok=0
[ "$(turn_col 2 checkpoint)" = refs/reef/checkpoints/2 ] || two_ok=0
sess1=$(turn_col 1 session_id) sess2=$(turn_col 2 session_id)
f1=$(session_file "$sess1") f2=$(session_file "$sess2")
[ "$(grep -ho '"cwd":"[^"]*"' "$f1" 2>/dev/null | sort -u)" = "\"cwd\":\"$S/full/worktrees/1/a\"" ] || two_ok=0
[ "$(grep -ho '"cwd":"[^"]*"' "$f2" 2>/dev/null | sort -u)" = "\"cwd\":\"$S/full/worktrees/1/b\"" ] || two_ok=0
check tree-two-features "$two_ok" "worktree list: $(echo "$wt_list" | tr '\n' ' '); status clean; each session's cwd is its own tree"

s1=$(turn_col 1 started_at) e1=$(turn_col 1 ended_at)
s2=$(turn_col 2 started_at) e2=$(turn_col 2 ended_at)
par_ok=1
[[ "$s1" < "$e2" ]] || par_ok=0
[[ "$s2" < "$e1" ]] || par_ok=0
check writer-parallel "$par_ok" "turn 1 $s1..$e1, turn 2 $s2..$e2: each started before the other ended"

echo mark >"$T/marker.txt"
api_call POST /channels/1/send '{"text":"@developer Write the numbers from 1 to 600 separated by spaces. Use no tools."}'
api_call POST /channels/2/send '{"text":"@developer Write the numbers from 1 to 600 separated by spaces. Use no tools."}'
wait_done 1 2
wait_done 2 1

c1_turn=$("$H" sql "select id from turn where channel_id = 1 order by id desc limit 1" | tail -1)
c2_turn=$("$H" sql "select id from turn where channel_id = 2 order by id desc limit 1" | tail -1)
s_c1=$(turn_col "$c1_turn" started_at) e_c1=$(turn_col "$c1_turn" ended_at)
s_c2=$(turn_col "$c2_turn" started_at) e_c2=$(turn_col "$c2_turn" ended_at)
serial_ok=1
if [[ "$s_c1" < "$s_c2" ]]; then
	[[ "$e_c1" < "$s_c2" || "$e_c1" = "$s_c2" ]] || serial_ok=0
else
	[[ "$e_c2" < "$s_c1" || "$e_c2" = "$s_c1" ]] || serial_ok=0
fi
check writer-serial "$serial_ok" "channel 1 turn $c1_turn ($s_c1..$e_c1), channel 2 turn $c2_turn ($s_c2..$e_c2): serialized on the shared tree"

wt_list2=$(git -C "$S/cwd" worktree list)
reuse_ok=1
[ "$wt_list2" = "$wt_list" ] || reuse_ok=0
[ "$(cat "$T/marker.txt")" = mark ] || reuse_ok=0
check tree-reuse "$reuse_ok" "worktree list unchanged, marker.txt still mark"

git -C "$T" status --porcelain >"$E/a-status-before.txt"
api_call POST /channels/1/send '{"text":"@developer Write the numbers from 1 to 600 separated by spaces. Use no tools."}'
sleep 1
api_call POST /channels/2/send '{"text":"@tester Use the Write tool to create the file t.txt containing the word nope. If that is refused, try once with Bash. Say what happened."}'
sleep 4

dev_line="" tester_line=""
for p in $(pgrep -P "$(cat "$S/serve.pid")" -x claude); do
	line=$(ps -ww -o args= -p "$p")
	case $line in
	*--disallowedTools*) tester_line=$line ;;
	*) dev_line=$line ;;
	esac
done

wait_done 1 3
wait_done 2 2

d_turn=$("$H" sql "select id from turn where channel_id = 1 order by id desc limit 1" | tail -1)
t_turn=$("$H" sql "select id from turn where channel_id = 2 order by id desc limit 1" | tail -1)
s_d=$(turn_col "$d_turn" started_at) e_d=$(turn_col "$d_turn" ended_at)
s_t=$(turn_col "$t_turn" started_at) e_t=$(turn_col "$t_turn" ended_at)
reader_ok=1
[[ "$s_t" < "$e_d" ]] || reader_ok=0
[[ "$e_t" < "$e_d" ]] || reader_ok=0
check writer-reader "$reader_ok" "tester turn $t_turn ($s_t..$e_t) ran inside developer turn $d_turn ($s_d..$e_d)"

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
api_call POST /channels/1/send '{"text":"@developer Reply with one word. Use no tools."}'
wait_done 1 4
N=$("$H" sql "select max(id) from turn where channel_id = 1" | tail -1)
api_call GET "/turns/$N"
cp_ok=1
echo "$BODY" | grep -q "\"checkpoint\":\"refs/reef/checkpoints/$N\"" || cp_ok=0
{
	git -C "$T" rev-parse HEAD
	git -C "$T" status --porcelain
	git -C "$T" ls-files --stage
} >"$E/a-after.txt"
diff "$E/a-before.txt" "$E/a-after.txt" >/dev/null || cp_ok=0
check checkpoint-untouched "$cp_ok" "turn $N checkpoint set; HEAD/status/index unchanged across the no-tool turn"

untracked_ok=1
[ "$(git -C "$T" show "refs/reef/checkpoints/$N:pre.txt")" = before ] || untracked_ok=0
[ "$(git -C "$T" show "refs/reef/checkpoints/$N^2:st.txt")" = staged ] || untracked_ok=0
check checkpoint-untracked "$untracked_ok" "checkpoint $N holds pre.txt (untracked) and ^2 holds st.txt (staged)"

api_call POST /channels/1/send '{"text":"@developer Use the Write tool to create the file d.txt containing the word yes."}'
wait_done 1 5
write_ok=1
[ "$(cat "$T/d.txt" 2>/dev/null)" = yes ] || write_ok=0
check write-claude-allowed "$write_ok" "d.txt=$(cat "$T/d.txt" 2>/dev/null)"

upgrade_out=$(go test ./store -run Upgrade -v 2>&1)
upgrade_rc=$?
check checkpoint-upgrade "$([ "$upgrade_rc" = 0 ] && echo 1 || echo 0)" "go test ./store -run Upgrade -v exit $upgrade_rc: $(echo "$upgrade_out" | tail -3 | tr '\n' ' ')"

### Rollback, channel 1

snap() {
	git -C "$T" rev-parse HEAD
	git -C "$T" status --porcelain
	git -C "$T" log --oneline
	git -C "$T" ls-files --stage
	for f in $(ls "$T"); do echo "$f: $(cat "$T/$f" 2>/dev/null)"; done
}

snap >"$E/rb-before.txt"
api_call POST /channels/1/send '{"text":"@developer Append a line with the word two to a.txt, create the file c.txt containing the word sea, then run the shell command: git add -A && git commit -qm dev-change"}'
wait_done 1 6
N=$("$H" sql "select max(id) from turn where channel_id = 1" | tail -1)
api_call POST "/turns/$N/rollback"
rb_status=$STATUS
snap >"$E/rb-after.txt"
restore_ok=1
[ "$rb_status" = 204 ] || restore_ok=0
diff "$E/rb-before.txt" "$E/rb-after.txt" >/dev/null || restore_ok=0
check rollback-restores "$restore_ok" "rollback of turn $N returned $rb_status; snap before/after match (c.txt gone, a.txt back to one line)"

rb_note_ok=1
api_call GET '/messages?channel=1'
echo "$BODY" | grep -qF "\"author\":\"reef\",\"body\":\"Rolled back to before turn $N.\"" || rb_note_ok=0

api_call POST "/turns/$t_turn/rollback"
[ "$STATUS" = 400 ] || rb_note_ok=0
echo "$BODY" | grep -qF "invalid: turn $t_turn has no checkpoint" || rb_note_ok=0

api_call POST /turns/999/rollback
[ "$STATUS" = 404 ] || rb_note_ok=0
echo "$BODY" | grep -qF "turn 999: not found" || rb_note_ok=0

git -C "$T" rev-parse HEAD >"$E/busy-before.txt"
git -C "$T" status --porcelain >>"$E/busy-before.txt"
api_call POST /channels/1/send '{"text":"@developer Write the numbers from 1 to 600 separated by spaces. Use no tools."}'
sleep 2
api_call POST "/turns/$N/rollback"
busy1_status=$STATUS busy1_body=$BODY
dev_turn_ch2=$("$H" sql "select id from turn where channel_id = 2 and role = 'developer' limit 1" | tail -1)
api_call POST "/turns/$dev_turn_ch2/rollback"
busy2_status=$STATUS busy2_body=$BODY
wait_done 1 7
git -C "$T" rev-parse HEAD >"$E/busy-after.txt"
git -C "$T" status --porcelain >>"$E/busy-after.txt"

busy_ok=1
[ "$busy1_status" = 409 ] || busy_ok=0
echo "$busy1_body" | grep -qF "channel 1 has a turn running: conflict" || busy_ok=0
[ "$busy2_status" = 409 ] || busy_ok=0
echo "$busy2_body" | grep -qF "worktrees/1/a has a turn running: conflict" || busy_ok=0
diff "$E/busy-before.txt" "$E/busy-after.txt" >/dev/null || busy_ok=0
msgcount=$("$H" sql "select count(*) from message where channel_id = 1 and author = 'reef'" | tail -1)
[ "$msgcount" = 1 ] || busy_ok=0
check rollback-busy "$busy_ok" "busy1=$busy1_status busy2=$busy2_status; tree unchanged across the busy window; $msgcount reef message(s) on channel 1"

last_turn=$("$H" sql "select id from turn where channel_id = 1 order by id desc limit 1" | tail -1)
last_wake=$("$H" sql "select wake_text from turn where id = $last_turn" | tail -n +2)
echo "$last_wake" | grep -q '^\[channel 1, message [0-9]*\]' || rb_note_ok=0
echo "$last_wake" | grep -q 'reef wrote:' && rb_note_ok=0
last_sess=$(turn_col "$last_turn" session_id)
last_file=$(session_file "$last_sess")
grep -q "reef wrote: Rolled back to before turn $N\\." "$last_file" 2>/dev/null || rb_note_ok=0
check rollback-note "$rb_note_ok" "reef message and 400/404 as expected; next wake $last_turn's wake_text has no reef line, its session prompt does"

### Quick session, file isolation and a project without git

api_call POST /channels/4/send '{"text":"@tester Reply with one word."}'
wait_done 4 1
q_sess=$("$H" sql "select session_id from turn where channel_id = 4 order by id desc limit 1" | tail -1)
q_file=$(session_file "$q_sess")
quick_ok=1
[ "$(grep -ho '"cwd":"[^"]*"' "$q_file" 2>/dev/null | sort -u)" = "\"cwd\":\"$S/cwd\"" ] || quick_ok=0
check tree-quick "$quick_ok" "quick session's tester session ran with cwd $S/cwd only"

iso_ok=1
[ -e "$S/full/worktrees/1/a/d.txt" ] || iso_ok=0
[ ! -e "$S/full/worktrees/1/b/d.txt" ] || iso_ok=0
[ ! -e "$S/cwd/d.txt" ] || iso_ok=0
[ -z "$(git -C "$S/cwd" status --porcelain)" ] || iso_ok=0
check tree-isolated "$iso_ok" "d.txt is only in worktrees/1/a; main checkout clean"

api_call POST /channels/5/send '{"text":"Reply with one word."}'
notgit_send_status=$STATUS
wait_for '/turns?channel=5&status=failed' lead
api_call GET '/turns?channel=5'
notgit_ok=1
[ "$notgit_send_status" = 202 ] || notgit_ok=0
echo "$BODY" | grep -q '"role":"lead"' || notgit_ok=0
echo "$BODY" | grep -q '"status":"failed"' || notgit_ok=0
echo "$BODY" | grep -qF "features need the project folder to be a git repo with a commit: $S/plain:" || notgit_ok=0
grep -q "turn of lead in channel 5 failed" "$E/serve.log" || notgit_ok=0
api_call GET /health
[ "$STATUS" = 200 ] || notgit_ok=0
check tree-not-git "$notgit_ok" "channel 5 lead turn failed with the not-a-git-repo error; /health still 200"

### Promote with a dirty main checkout (channel 6, solo)

api_call POST /channels '{"projectId":1,"name":"promo"}'
echo dirty >"$S/cwd/dirty.txt"
api_call POST /channels/6/send '{"text":"Reply with one word."}'
wait_done 6 1
api_call POST /channels/6/promote '{"slug":"c","name":"C"}'
promote_status=$STATUS
wait_done 6 2
api_call GET '/messages?channel=6'
promote_msg_ok=1
echo "$BODY" | grep -qF "\"author\":\"reef\",\"body\":\"This feature works in $S/full/worktrees/1/c on feature/c. Uncommitted changes stay on the main checkout.\"" || promote_msg_ok=0
api_call POST /channels/6/send '{"text":"Reply with one word."}'
wait_done 6 3

turns6=$("$H" sql "select id, role, status, session_id from turn where channel_id = 6")
sess6=$(echo "$turns6" | tail -n +2 | awk -F'|' '{print $4}' | sort -u)
promote_ok=1
[ "$promote_status" = 202 ] || promote_ok=0
[ "$promote_msg_ok" = 1 ] || promote_ok=0
[ "$(echo "$turns6" | tail -n +2 | wc -l | tr -d ' ')" = 3 ] || promote_ok=0
[ "$(echo "$sess6" | wc -l | tr -d ' ')" = 1 ] || promote_ok=0
sess6_id=$(echo "$sess6" | head -1)
sess6_file=$(session_file "$sess6_id")
grep -q "\"cwd\":\"$S/cwd\"" "$sess6_file" 2>/dev/null || promote_ok=0
grep -q "\"cwd\":\"$S/full/worktrees/1/c\"" "$sess6_file" 2>/dev/null || promote_ok=0
wt_list3=$(git -C "$S/cwd" worktree list)
echo "$wt_list3" | grep -q "^$S/full/worktrees/1/c .*\[feature/c\]" || promote_ok=0
[ "$(git -C "$S/cwd" status --porcelain)" = "?? dirty.txt" ] || promote_ok=0
check tree-promote-dirty "$promote_ok" "promote 202 with the move note, 3 lead turns share session $sess6_id (resumed in the worktree), main checkout only ?? dirty.txt"

### Retry after a crash, channel 3

api_call POST /channels/3/send '{"text":"@developer Write the numbers from 1 to 600 separated by spaces. Use no tools."}'
sleep 3
"$H" serve-stop --kill
ANTHROPIC_MODEL=sonnet "$H" serve-start --data-name full
A=$(cat "$S/serve.addr")
F=$("$H" sql "select max(id) from turn where channel_id = 3" | tail -1)
api_call GET "/turns/$F"
retry_ok=1
echo "$BODY" | grep -q '"status":"failed"' || retry_ok=0
echo "$BODY" | grep -q '"error":"server restarted"' || retry_ok=0
echo "$BODY" | grep -q '"retried":false' || retry_ok=0
echo "$BODY" | grep -q '"wakeText":""' && retry_ok=0
api_call GET '/messages?channel=3'
echo "$BODY" | grep -qF "Turn $F by developer failed: server restarted. The worktree may be dirty; retry or roll back." || retry_ok=0
api_call POST "/turns/$F/retry"
[ "$STATUS" = 202 ] || retry_ok=0
wait_done 3 2
rows3=$("$H" sql "select id, role, status, retried from turn where channel_id = 3 order by id")
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

### Inbox (channels 7 to 9 are quick sessions, so they make no worktree)

api_call POST /channels '{"projectId":1,"name":"gate","team":"relay"}'
api_call POST /channels '{"projectId":1,"name":"budget","team":"crew"}'
api_call POST /channels '{"projectId":1,"name":"crashed","team":"crew"}'
api_call POST /channels/9/send '{"text":"Write the numbers from 1 to 600 separated by spaces."}'
sleep 3
api_call POST /channels/9/stop
[ "$STATUS" = 204 ] || echo "note: channel 9 stop returned $STATUS, not 204" >&2
api_call GET '/turns?channel=9'
echo "$BODY" | grep -q '"status":"failed"' || echo "note: channel 9 turn not failed after stop" >&2
echo "$BODY" | grep -q '"error":"interrupted"' || echo "note: channel 9 turn error not interrupted" >&2
api_call GET /inbox
echo "$BODY" | grep -q '"channelId":9' && echo "note: channel 9 unexpectedly in the inbox after a user stop" >&2

api_call POST /channels/9/send '{"text":"Write the numbers from 1 to 600 separated by spaces."}'
sleep 3
"$H" serve-stop --kill
ANTHROPIC_MODEL=sonnet "$H" serve-start --data-name full
A=$(cat "$S/serve.addr")
C=$("$H" sql "select max(id) from turn where channel_id = 9" | tail -1)

curl -sN --max-time 400 "http://$A/inbox/events" >"$E/inbox-events.txt" &
inbox_stream_pid=$!

api_call POST /channels/7/send '{"text":"go"}'
api_call POST /channels/8/send '{"text":"Reply with one word."}'
wait_done 7 1
wait_done 8 1
"$H" sql --write "update channel set token_budget = 1 where id = 8" >/dev/null
api_call POST /channels/8/send '{"text":"Reply with one word."}'
budget_status=$STATUS budget_body=$BODY

api_call GET /inbox
inbox_kinds_ok=1
[ "$budget_status" = 409 ] || inbox_kinds_ok=0
echo "$budget_body" | grep -q 'channel 8 is paused: token budget of 1 used: [0-9]* tokens: conflict' || inbox_kinds_ok=0
echo "$BODY" | grep -q "\"kind\":\"failed\",\"channelId\":9" || inbox_kinds_ok=0
echo "$BODY" | grep -q '"text":"lead failed: server restarted"' || inbox_kinds_ok=0
echo "$BODY" | grep -qF "\"turnId\":$C" || inbox_kinds_ok=0
echo "$BODY" | grep -q '"kind":"approval","channelId":7' || inbox_kinds_ok=0
echo "$BODY" | grep -q '"text":"lead wants to wake tester"' || inbox_kinds_ok=0
echo "$BODY" | grep -q '"approvalId":1' || inbox_kinds_ok=0
echo "$BODY" | grep -q '"kind":"paused","channelId":8' || inbox_kinds_ok=0
echo "$BODY" | grep -q '"text":"token budget of 1 used: [0-9]* tokens"' || inbox_kinds_ok=0
echo "$BODY" | grep -q '"kind":"waiting","channelId":4' || inbox_kinds_ok=0
check inbox-kinds "$inbox_kinds_ok" "inbox has failed(9), approval(7), paused(8) and waiting(4) items"

api_call POST /channels/7/approvals/1 '{"approve":true}'
s1=$STATUS
api_call POST /channels/8/continue
s2=$STATUS
api_call POST "/turns/$C/retry"
s3=$STATUS
api_call POST /channels/4/send '{"text":"Reply with one other word."}'
s4=$STATUS
clear_ok=1
[ "$s1" = 204 ] && [ "$s2" = 204 ] && [ "$s3" = 202 ] && [ "$s4" = 202 ] || clear_ok=0
api_call GET /inbox
# 4, 7 and 9 each started a new turn, so they have no item at all while it runs; 8's
# continue starts none, so its cleared paused item is at once a waiting item instead.
for ch in 4 7 9; do
	echo "$BODY" | grep -q "\"channelId\":$ch" && clear_ok=0
done
echo "$BODY" | grep -q '"kind":"paused","channelId":8' && clear_ok=0

wait_done 7 2
wait_done 9 1
wait_done 4 2
api_call POST /channels/8/send '{"text":"Reply with one word."}'
[ "$STATUS" = 202 ] || clear_ok=0
wait_done 8 2

[ "$("$H" sql "select role, status from turn where channel_id = 7 order by id desc limit 1" | tail -1)" = "tester|done" ] || clear_ok=0
[ "$("$H" sql "select role, status from turn where channel_id = 9 order by id desc limit 1" | tail -1)" = "lead|done" ] || clear_ok=0
[ "$("$H" sql "select retried from turn where id = $C" | tail -1)" = 1 ] || clear_ok=0
[ "$("$H" sql "select role, status from turn where channel_id = 4 order by id desc limit 1" | tail -1)" = "lead|done" ] || clear_ok=0
[ "$("$H" sql "select role, status from turn where channel_id = 8 order by id desc limit 1" | tail -1)" = "lead|done" ] || clear_ok=0

api_call GET /inbox
for ch in 4 7 8 9; do
	echo "$BODY" | grep -q "\"kind\":\"waiting\",\"channelId\":$ch" || clear_ok=0
done
check inbox-clear "$clear_ok" "approve/continue/retry/send cleared their items (204,204,202,202); each channel ran on and now lists a new waiting item"

kill "$inbox_stream_pid" 2>/dev/null
wait "$inbox_stream_pid" 2>/dev/null
frames=$(grep '^data:' "$E/inbox-events.txt" | while IFS= read -r l; do echo "$l" | grep -o '"kind":"[a-z]*","channelId":[4789]' | tr '\n' ' '; echo; done)
stream_ok=1
[ -n "$frames" ] || stream_ok=0
echo "$frames" | head -1 | grep -q '"kind":"failed","channelId":9' || stream_ok=0
echo "$frames" | grep -q '"kind":"approval","channelId":7' || stream_ok=0
echo "$frames" | grep -q '"kind":"paused","channelId":8' || stream_ok=0
echo "$frames" | grep -q '"kind":"waiting","channelId":7' || stream_ok=0
echo "$frames" | grep -q '"kind":"waiting","channelId":9' || stream_ok=0
check inbox-stream "$stream_ok" "frames across the stream: $(echo "$frames" | tr '\n' '|')"

api_call POST /channels/7/send '{"text":"go"}'
wait_for /inbox '"kind":"approval"'
"$H" sql --write "update channel set token_budget = 1 where id = 8" >/dev/null
api_call POST /channels/8/send '{"text":"Reply with one word."}'
restart_budget_status=$STATUS
api_call GET /inbox
echo "$BODY" >"$E/inbox-before.txt"
"$H" serve-stop
ANTHROPIC_MODEL=sonnet "$H" serve-start --data-name full
A=$(cat "$S/serve.addr")
api_call GET /inbox
echo "$BODY" >"$E/inbox-after.txt"
restart_ok=1
[ "$restart_budget_status" = 409 ] || restart_ok=0
diff "$E/inbox-before.txt" "$E/inbox-after.txt" >/dev/null || restart_ok=0
kinds_after=$(grep -o '"kind":"[a-z]*","channelId":[0-9]*' "$E/inbox-after.txt")
echo "$kinds_after" | grep -q '"kind":"approval","channelId":7' || restart_ok=0
echo "$kinds_after" | grep -q '"kind":"paused","channelId":8' || restart_ok=0
echo "$kinds_after" | grep -q '"kind":"failed","channelId":5' || restart_ok=0
echo "$kinds_after" | grep -q '"kind":"waiting"' || restart_ok=0
check inbox-restart "$restart_ok" "inbox before/after a restart match byte for byte: $(echo "$kinds_after" | tr '\n' ' ')"
}

section_cx() {
### Codex on the test fixture (write-codex-readonly), folder cx

mkdir -p "$S/spy"
printf '#!/bin/sh\ntee -a %s/codex-in.log | %s/agent/codex/testdata/codex "$@"\n' "$E" "$PWD" >"$S/spy/codex"
chmod +x "$S/spy/codex"
"$H" reef --data-name cx -- team apply "$E/cx.json" >/dev/null
BASH_ENV= FAKE_CODEX=replay PATH="$S/spy:$PATH" ANTHROPIC_MODEL=sonnet "$H" serve-start --data-name cx
A=$(cat "$S/serve.addr")
api_call POST /projects '{"name":"cx","path":"'"$S"'/cwd"}'
api_call POST /channels '{"projectId":1,"name":"cx room","team":"cx"}'
api_call POST /channels/1/send '{"text":"@tester hi"}'
wait_done 1 1
api_call POST /channels/1/send '{"text":"@developer hi"}'
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

section_inbox() {
### Window (`$H ui`)

cat >"$E/gated.json" <<'EOF'
{
  "name": "gated",
  "gates": [{"from": "lead", "to": "architect", "on": true}],
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

"$H" reef --data-name inbox -- team apply "$E/gated.json" >/dev/null
ANTHROPIC_MODEL=sonnet "$H" serve-start --data-name inbox
run_ui window-inbox inbox
}

section_ui2() {
ANTHROPIC_MODEL=sonnet "$H" serve-start --data-name ui2
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

"$H" reef --data-name google -- team apply "$PWD/teams/full.json" >/dev/null
ANTHROPIC_MODEL=sonnet "$H" serve-start --data-name google
A=$(cat "$S/serve.addr")
api_call POST /projects '{"name":"shop","path":"'"$S"'/google-app"}'
api_call POST /features '{"projectId":1,"slug":"google-login","name":"Google login"}'
api_call POST /channels '{"projectId":1,"featureId":1,"name":"google","team":"full"}'

curl -sN --max-time 1800 "http://$A/inbox/events" >"$E/google-inbox.txt" &
google_inbox_pid=$!
curl -sN --max-time 1800 "http://$A/channels/1/events" >"$E/google-events.txt" &
google_events_pid=$!

idle() {
	for i in $(seq 120); do
		[ "$(curl -s "http://$A/turns?channel=1&status=running")" = "[]" ] && {
			sleep 4
			[ "$(curl -s "http://$A/turns?channel=1&status=running")" = "[]" ] && return 0
		}
		sleep 5
	done
	echo "timeout waiting for channel 1 to idle" >&2
	return 1
}

google_ok=1 google_bad=
[ "$google_app_test_rc" = 0 ] || { google_ok=0; google_bad+=" $LINENO"; }

api_call POST /channels/1/send '{"text":"Add Google login to this app: a Sign in with Google link on the login page that goes to /auth/google, which redirects to the Google OAuth URL with the client id from the GOOGLE_CLIENT_ID env var. Keep it small and covered by a test."}'
idle

rows_g=$("$H" sql "select role, status, checkpoint from turn where channel_id = 1 order by id")
echo "$rows_g" | sed -n '2p' | grep -qF 'lead|done|' || { google_ok=0; google_bad+=" $LINENO"; }
echo "$rows_g" | sed -n '3p' | grep -qF 'architect|done|' || { google_ok=0; google_bad+=" $LINENO"; }
appr=$("$H" sql "select id, from_role, to_role, kind, status from approval")
echo "$appr" | grep -qF '1|architect|developer|gate|pending' || { google_ok=0; google_bad+=" $LINENO"; }
api_call GET /inbox
echo "$BODY" | grep -q '"kind":"approval".*"channelId":1' || { google_ok=0; google_bad+=" $LINENO"; }
echo "$BODY" | grep -q 'architect wants to wake developer' || { google_ok=0; google_bad+=" $LINENO"; }

api_call POST /channels/1/approvals/1 '{"approve":true}'
idle

dev_turn_g=$("$H" sql "select id from turn where channel_id = 1 and role = 'developer' order by id desc limit 1" | tail -1)
[ "$(turn_col "$dev_turn_g" status)" = done ] || { google_ok=0; google_bad+=" $LINENO"; }
[ "$(turn_col "$dev_turn_g" checkpoint)" = "refs/reef/checkpoints/$dev_turn_g" ] || { google_ok=0; google_bad+=" $LINENO"; }
[ "$(git -C "$S/google-app" log --oneline main..feature/google-login | wc -l | tr -d ' ')" = 1 ] || { google_ok=0; google_bad+=" $LINENO"; }
git -C "$S/google-app" worktree list | grep -q "^$S/google/worktrees/1/google-login .*\[feature/google-login\]" || { google_ok=0; google_bad+=" $LINENO"; }
[ -z "$(git -C "$S/google-app" status --porcelain)" ] || { google_ok=0; google_bad+=" $LINENO"; }
[ "$(git -C "$S/google-app" log --oneline main | wc -l | tr -d ' ')" = 1 ] || { google_ok=0; google_bad+=" $LINENO"; }
appr2=$("$H" sql "select id, from_role, to_role, kind, status from approval where id = 2")
echo "$appr2" | grep -qF '2|developer|tester|gate|pending' || { google_ok=0; google_bad+=" $LINENO"; }
api_call GET /inbox
[ "$(echo "$BODY" | grep -o '"kind"' | wc -l | tr -d ' ')" = 1 ] || { google_ok=0; google_bad+=" $LINENO"; }
echo "$BODY" | grep -q '"kind":"approval".*"channelId":1' || { google_ok=0; google_bad+=" $LINENO"; }

api_call POST /channels/1/approvals/2 '{"approve":true}'
idle

tester_turn_g=$("$H" sql "select id from turn where channel_id = 1 and role = 'tester' order by id desc limit 1" | tail -1)
[ "$(turn_col "$tester_turn_g" status)" = done ] || { google_ok=0; google_bad+=" $LINENO"; }
[ "$(turn_col "$tester_turn_g" "checkpoint is null")" = 1 ] || { google_ok=0; google_bad+=" $LINENO"; }
lead_turn_g=$("$H" sql "select id from turn where channel_id = 1 and role = 'lead' order by id desc limit 1" | tail -1)
[ "$(turn_col "$lead_turn_g" status)" = done ] || { google_ok=0; google_bad+=" $LINENO"; }
[ -z "$(echo "$("$H" sql "select paused from channel")" | tail -n +2 | tr -d '[:space:]')" ] || { google_ok=0; google_bad+=" $LINENO"; }
api_call GET /inbox
[ "$(echo "$BODY" | grep -o '"kind"' | wc -l | tr -d ' ')" = 1 ] || { google_ok=0; google_bad+=" $LINENO"; }
echo "$BODY" | grep -q '"kind":"waiting"' || { google_ok=0; google_bad+=" $LINENO"; }

api_call GET '/messages?channel=1'
api_call GET '/turns?channel=1'
git -C "$S/google-app" log --stat main..feature/google-login >"$E/google-log-stat.txt"

kill "$google_inbox_pid" 2>/dev/null
wait "$google_inbox_pid" 2>/dev/null
kill "$google_events_pid" 2>/dev/null
wait "$google_events_pid" 2>/dev/null
grep -A1 '^event: approval' "$E/google-events.txt" | grep -q pending || { google_ok=0; google_bad+=" $LINENO"; }
grep -A1 '^event: approval' "$E/google-events.txt" | grep -q approved || { google_ok=0; google_bad+=" $LINENO"; }

check google-login "$google_ok" "${google_bad:+failed at full-team.sh lines$google_bad; }lead, architect, gate, developer, gate, tester, lead ran end to end; checkpoints, worktree, clean main checkout, inbox and event streams all match"
}

section_fu() {
### Gates, notes, retry and rollback rules, folder fu

prompt() { grep -h '"type":"user"' "$HOME"/.claude/projects/*/"$1".jsonl 2>/dev/null | grep -o '"content":"\[channel[^"]*' | tail -1; }

python3 -I -c 'import json,sys
t=json.load(open(sys.argv[1]))
t["name"]="open"
t["gates"]=[{"from":"lead","to":"developer","on":False}]
json.dump(t, open(sys.argv[2],"w"))' "$PWD/teams/full.json" "$E/open.json"
cat >"$E/pair.json" <<'EOF'
{
  "name": "pair",
  "roles": [
    {"name": "lead", "lead": true, "backend": "claude",
     "persona": "You are the lead. Do exactly what the user asks. Never @mention anyone unless the user tells you to."},
    {"name": "developer", "write": true, "backend": "claude", "permissionMode": "acceptEdits",
     "persona": "You are the developer. Do what the user asks with as few tool calls as you can, then answer in one short sentence. Never @mention anyone.",
     "tools": ["Read", "Edit", "Write", "Bash"]},
    {"name": "tester", "backend": "claude", "permissionMode": "acceptEdits",
     "persona": "You are the tester. Do what the user asks, then answer in one short sentence. Never @mention anyone.",
     "tools": ["Read", "Edit", "Write", "Bash(ls:*)"]}
  ]
}
EOF
"$H" reef --data-name fu -- team apply "$PWD/teams/full.json" >/dev/null
"$H" reef -- team apply "$E/open.json" >/dev/null
"$H" reef -- team apply "$E/pair.json" >/dev/null
ANTHROPIC_MODEL=sonnet "$H" serve-start --data-name fu
A=$(cat "$S/serve.addr")
api_call GET /teams/open
echo "$BODY" | grep -qF '"gates":[{"from":"lead","to":"developer","on":false}]' || echo "note: /teams/open missing expected gates" >&2

api_call POST /projects '{"name":"main","path":"'"$S"'/cwd"}'
api_call POST /features '{"projectId":1,"slug":"fa","name":"FA"}'
api_call POST /features '{"projectId":1,"slug":"fb","name":"FB"}'
api_call POST /features '{"projectId":1,"slug":"fc","name":"FC"}'
api_call POST /channels '{"projectId":1,"featureId":1,"name":"fa1","team":"pair"}'
api_call POST /channels '{"projectId":1,"featureId":1,"name":"fa2","team":"pair"}'
api_call POST /channels '{"projectId":1,"featureId":2,"name":"fb1","team":"full"}'
api_call POST /channels '{"projectId":1,"featureId":3,"name":"fc1","team":"open"}'
WA="$S/fu/worktrees/1/fa"
P='Reply with exactly one line and nothing else: an at sign immediately followed by the word developer, then a space, then the words: reply with one word, use no tools.'

api_call POST /channels/3/send "{\"text\":\"$P\"}"
wait_for /channels/3/approvals pending
gate1_ok=1
api_call GET '/messages?channel=3'
echo "$BODY" | grep -qF '@developer reply with one word, use no tools.' || gate1_ok=0
apr=$("$H" sql "select channel_id, from_role, to_role, kind, status from approval")
echo "$apr" | grep -qF '3|lead|developer|gate|pending' || gate1_ok=0
[ "$(echo "$("$H" sql "select role from turn where channel_id = 3")" | tail -n +2 | tr -d '[:space:]')" = lead ] || gate1_ok=0
check gate-lead-developer "$gate1_ok" "channel 3 held lead|developer|gate|pending; only a lead turn ran"

api_call POST /channels/4/send "{\"text\":\"$P\"}"
wait_done 4 2
gate_off_ok=1
rows4f=$("$H" sql "select role, status from turn where channel_id = 4 order by id")
echo "$rows4f" | grep -qF 'lead|done' || gate_off_ok=0
echo "$rows4f" | grep -qF 'developer|done' || gate_off_ok=0
[ "$("$H" sql "select count(*) from approval where channel_id = 4" | tail -1)" = 0 ] || gate_off_ok=0
check gate-off "$gate_off_ok" "channel 4 (gate off): lead and developer both done, no approval rows"

api_call POST /channels/1/send '{"text":"@developer Use the Write tool to create the file f.txt containing the word one."}'
wait_done 1 1
D=$("$H" sql "select id from turn where channel_id = 1 order by id desc limit 1" | tail -1)
api_call POST /channels/1/send "{\"text\":\"$P\"}"
wait_for /channels/1/approvals pending
api_call POST "/turns/$D/rollback"
note_approved_ok=1
[ "$STATUS" = 204 ] || note_approved_ok=0
[ ! -e "$WA/f.txt" ] || note_approved_ok=0
appr_id1=$("$H" sql "select id from approval where channel_id = 1" | tail -1)
api_call POST "/channels/1/approvals/$appr_id1" '{"approve":true}'
[ "$STATUS" = 204 ] || note_approved_ok=0
wait_done 1 3
new_dev_sess=$("$H" sql "select session_id from turn where channel_id = 1 order by id desc limit 1" | tail -1)
new_dev_file=$(session_file "$new_dev_sess")
grep -q "reef wrote: Rolled back to before turn $D\\." "$new_dev_file" 2>/dev/null || note_approved_ok=0
check note-approved "$note_approved_ok" "a wake held at a gate, a rollback, then approve: the approved wake's prompt starts with the rollback note"

api_call POST /channels/1/send '{"text":"@developer Write the numbers from 1 to 600 separated by spaces. Use no tools."}'
sleep 4
"$H" serve-stop --kill
ANTHROPIC_MODEL=sonnet "$H" serve-start --data-name fu
A=$(cat "$S/serve.addr")
F2=$("$H" sql "select id from turn where channel_id = 1 order by id desc limit 1" | tail -1)
api_call GET /inbox
note_retry_ok=1
echo "$BODY" | grep -qF "\"kind\":\"failed\",\"channelId\":1" || note_retry_ok=0
echo "$BODY" | grep -qF '"text":"developer failed: server restarted"' || note_retry_ok=0
echo "$BODY" | grep -qF "\"turnId\":$F2" || note_retry_ok=0
api_call POST "/turns/$F2/rollback"
[ "$STATUS" = 204 ] || note_retry_ok=0
api_call POST "/turns/$F2/retry"
[ "$STATUS" = 202 ] || note_retry_ok=0
wait_done 1 4
sess_f2=$("$H" sql "select session_id from turn where channel_id = 1 order by id desc limit 1" | tail -1)
file_f2=$(session_file "$sess_f2")
grep -qF "reef wrote: Turn $F2 by developer failed: server restarted. The worktree may be dirty; retry or roll back." "$file_f2" 2>/dev/null || note_retry_ok=0
grep -q "reef wrote: Rolled back to before turn $F2\\." "$file_f2" 2>/dev/null || note_retry_ok=0
api_call POST /channels/1/send '{"text":"@developer Reply with one word. Use no tools."}'
wait_done 1 5
sess_after=$("$H" sql "select session_id from turn where channel_id = 1 order by id desc limit 1" | tail -1)
last_p=$(prompt "$sess_after")
echo "$last_p" | grep -q 'reef wrote:' && note_retry_ok=0
check note-retry "$note_retry_ok" "failed/rollback/retry notes land in the retried prompt; the next wake's prompt has none"

api_call POST /channels/1/send '{"text":"@developer Write the numbers from 1 to 600 separated by spaces. Use no tools."}'
sleep 4
"$H" serve-stop --kill
ANTHROPIC_MODEL=sonnet "$H" serve-start --data-name fu
A=$(cat "$S/serve.addr")
G=$("$H" sql "select id from turn where channel_id = 1 order by id desc limit 1" | tail -1)
api_call POST /channels/1/send '{"text":"@developer Reply with one word. Use no tools."}'
wait_done 1 6
api_call POST "/turns/$G/retry"
superseded_ok=1
[ "$STATUS" = 409 ] || superseded_ok=0
echo "$BODY" | grep -qF "turn $G cannot be retried: conflict" || superseded_ok=0
api_call GET '/turns?channel=1&retryable=1'
[ "$BODY" = "[]" ] || superseded_ok=0
api_call GET /inbox
echo "$BODY" | grep -q "\"kind\":\"failed\",\"channelId\":1" && superseded_ok=0
check retry-superseded "$superseded_ok" "retry of the superseded turn $G is 409, not retryable, not in the inbox"

api_call POST /channels/1/wrap
wrap_send_status=$STATUS
sleep 4
"$H" serve-stop --kill
ANTHROPIC_MODEL=sonnet "$H" serve-start --data-name fu
A=$(cat "$S/serve.addr")
newest1=$("$H" sql "select id from turn where channel_id = 1 order by id desc limit 1" | tail -1)
row_wrap=$("$H" sql "select role, status, error, wake_text is null from turn where id = $newest1" | tail -1)
retry_wrap_ok=1
[ "$wrap_send_status" = 202 ] || retry_wrap_ok=0
[ "$row_wrap" = "lead|failed|server restarted|1" ] || retry_wrap_ok=0
api_call POST "/turns/$newest1/retry"
[ "$STATUS" = 409 ] || retry_wrap_ok=0
echo "$BODY" | grep -qF "turn $newest1 cannot be retried: conflict" || retry_wrap_ok=0
api_call GET '/turns?channel=1&retryable=1'
[ "$BODY" = "[]" ] || retry_wrap_ok=0
api_call GET /inbox
echo "$BODY" | grep -q "\"channelId\":1" && retry_wrap_ok=0
check retry-wrap "$retry_wrap_ok" "a failed wrap-up turn keeps no wake: retry 409, not retryable, not in the inbox"

api_call POST /channels '{"projectId":1,"name":"q"}'
q_chan=$("$H" sql "select id from channel where name = 'q'" | tail -1)
api_call POST "/channels/$q_chan/send" '{"text":"Reply with one word."}'
wait_done "$q_chan" 1
Q=$("$H" sql "select id from turn where channel_id = $q_chan order by id desc limit 1" | tail -1)
[ "$(turn_col "$Q" checkpoint)" = "refs/reef/checkpoints/$Q" ] || echo "note: quick session turn $Q has no expected checkpoint" >&2
api_call POST "/channels/$q_chan/promote" '{"slug":"fq","name":"FQ"}'
wait_done "$q_chan" 2
api_call POST "/channels/$q_chan/send" '{"text":"Reply with one word."}'
wait_done "$q_chan" 3
{
	git -C "$S/fu/worktrees/1/fq" rev-parse HEAD
	git -C "$S/fu/worktrees/1/fq" status --porcelain
	ls -a "$S/fu/worktrees/1/fq"
} >"$E/fq-before.txt"
api_call POST "/turns/$Q/rollback"
moved_ok=1
[ "$STATUS" = 409 ] || moved_ok=0
echo "$BODY" | grep -qF "turn $Q was checkpointed in $S/cwd, not $S/fu/worktrees/1/fq: conflict" || moved_ok=0
{
	git -C "$S/fu/worktrees/1/fq" rev-parse HEAD
	git -C "$S/fu/worktrees/1/fq" status --porcelain
	ls -a "$S/fu/worktrees/1/fq"
} >"$E/fq-after.txt"
diff "$E/fq-before.txt" "$E/fq-after.txt" >/dev/null || moved_ok=0
check rollback-moved "$moved_ok" "rollback of the moved checkpoint $Q is 409; the fq tree is unchanged"

api_call POST /channels/2/send '{"text":"@tester Write the numbers from 1 to 600 separated by spaces. Use no tools."}'
sleep 2
newest_dev1=$("$H" sql "select id from turn where channel_id = 1 and role = 'developer' order by id desc limit 1" | tail -1)
api_call POST "/turns/$newest_dev1/rollback"
reader_hold_ok=1
[ "$STATUS" = 409 ] || reader_hold_ok=0
echo "$BODY" | grep -qF "$S/fu/worktrees/1/fa has a turn running: conflict" || reader_hold_ok=0
wait_done 2 1
check rollback-reader "$reader_hold_ok" "rollback while channel 2's tester runs on the same tree is 409"

H0=$(git -C "$WA" rev-parse HEAD)
api_call POST /channels/1/send '{"text":"@developer Run this one shell command and nothing else: git checkout -q -b side && echo s > s.txt && git add s.txt && git commit -qm side"}'
wait_done 1 7
D2=$("$H" sql "select id from turn where channel_id = 1 order by id desc limit 1" | tail -1)
branch_ok=1
[ "$(git -C "$WA" branch --show-current)" = side ] || branch_ok=0
api_call POST "/turns/$D2/rollback"
[ "$STATUS" = 204 ] || branch_ok=0
[ "$(git -C "$WA" branch --show-current)" = feature/fa ] || branch_ok=0
[ "$(git -C "$WA" rev-parse HEAD)" = "$H0" ] || branch_ok=0
[ "$(git -C "$WA" rev-parse feature/fa)" = "$H0" ] || branch_ok=0
check rollback-branch "$branch_ok" "after the developer switched to side, rollback of turn $D2 returns to feature/fa at $H0"

api_call POST /channels '{"projectId":1,"featureId":1,"name":"solo fa"}'
solofa_chan=$("$H" sql "select id from channel where name = 'solo fa'" | tail -1)
api_call POST "/channels/$solofa_chan/send" '{"text":"Reply with one word."}'
wait_done "$solofa_chan" 1
Cchat=$("$H" sql "select id from turn where channel_id = $solofa_chan order by id desc limit 1" | tail -1)
api_call POST "/channels/$solofa_chan/stop"
(sleep 30 | ANTHROPIC_MODEL=sonnet "$S/bin/reef" chat --channel "$solofa_chan" --data "$S/fu" >"$E/chat-open.txt" 2>&1 &)
sleep 8
api_call POST "/turns/$Cchat/rollback"
chat_ok=1
[ "$STATUS" = 409 ] || chat_ok=0
echo "$BODY" | grep -qF "channel $solofa_chan is open elsewhere: conflict" || chat_ok=0
check rollback-chat "$chat_ok" "rollback of a checkpointed turn while its channel is open in reef chat is 409"

mkdir -p "$S/fresh"
git -C "$S/fresh" init -q -b main
api_call POST /projects '{"name":"fresh","path":"'"$S"'/fresh"}'
fresh_id=$("$H" sql "select id from project where name = 'fresh'" | tail -1)
api_call POST /channels '{"projectId":'"$fresh_id"',"name":"fresh q"}'
freshq_chan=$("$H" sql "select id from channel where name = 'fresh q'" | tail -1)
api_call POST "/channels/$freshq_chan/send" '{"text":"Reply with one word."}'
wait_done "$freshq_chan" 1
api_call GET "/turns?channel=$freshq_chan"
unborn_ok=1
echo "$BODY" | grep -q '"status":"done"' || unborn_ok=0
echo "$BODY" | grep -q '"checkpoint":null' || unborn_ok=0
check checkpoint-unborn "$unborn_ok" "a quick session's lead turn on an unborn git repo is done with no checkpoint"

rm -rf "$S/fu/worktrees/1/fb"
api_call POST /channels/3/send '{"text":"@developer Reply with one word. Use no tools."}'
wait_done 3 2
recreate_ok=1
newest3=$("$H" sql "select id from turn where channel_id = 3 order by id desc limit 1" | tail -1)
[ "$(turn_col "$newest3" role)" = developer ] || recreate_ok=0
[ "$(turn_col "$newest3" status)" = done ] || recreate_ok=0
[ "$(turn_col "$newest3" "checkpoint is not null")" = 1 ] || recreate_ok=0
ls "$S/fu/worktrees/1/fb" 2>/dev/null | grep -qx a.txt || recreate_ok=0
git -C "$S/cwd" worktree list | grep -q "^$S/fu/worktrees/1/fb .*\[feature/fb\]" || recreate_ok=0
check tree-recreate "$recreate_ok" "worktree fb was deleted by hand; the next wake remade it with no error"

mkdir -p "$S/p157"
git -C "$S/p157" init -q -b main
echo one >"$S/p157/a.txt"
git -C "$S/p157" add a.txt
git -C "$S/p157" -c user.name=v -c user.email=v@example.com commit -qm init
git -C "$S/p157" checkout -q -b feature/x
api_call POST /projects '{"name":"p157","path":"'"$S"'/p157"}'
p157_id=$("$H" sql "select id from project where name = 'p157'" | tail -1)
api_call POST /features '{"projectId":'"$p157_id"',"slug":"x","name":"X"}'
x_feature_id=$("$H" sql "select id from feature where slug = 'x'" | tail -1)
api_call POST /channels '{"projectId":'"$p157_id"',"featureId":'"$x_feature_id"',"name":"x1"}'
x1_chan=$("$H" sql "select id from channel where name = 'x1'" | tail -1)
api_call POST "/channels/$x1_chan/send" '{"text":"Reply with one word."}'
wait_for "/turns?channel=$x1_chan&status=failed" lead
api_call GET "/turns?channel=$x1_chan&status=failed"
branch_taken_ok=1
echo "$BODY" | grep -qF "branch feature/x is checked out at $S/p157; switch that checkout to another branch" || branch_taken_ok=0
check tree-branch-taken "$branch_taken_ok" "a wake of feature x failed naming the taken branch and the main checkout"

mkdir -p "$S/p157c"
git -C "$S/p157c" init -q -b main
echo one >"$S/p157c/a.txt"
git -C "$S/p157c" add a.txt
git -C "$S/p157c" -c user.name=v -c user.email=v@example.com commit -qm init
api_call POST /projects '{"name":"p157c","path":"'"$S"'/p157c"}'
p157c_id=$("$H" sql "select id from project where name = 'p157c'" | tail -1)
api_call POST /channels '{"projectId":'"$p157c_id"',"name":"clean q"}'
cleanq_chan=$("$H" sql "select id from channel where name = 'clean q'" | tail -1)
api_call POST "/channels/$cleanq_chan/send" '{"text":"Reply with one word."}'
wait_done "$cleanq_chan" 1
clean_ok=1
[ -z "$(git -C "$S/p157c" status --porcelain)" ] || clean_ok=0
api_call POST "/channels/$cleanq_chan/promote" '{"slug":"pc","name":"PC"}'
wait_done "$cleanq_chan" 2
api_call GET "/messages?channel=$cleanq_chan"
echo "$BODY" | grep -qF "\"author\":\"reef\",\"body\":\"This feature works in $S/fu/worktrees/$p157c_id/pc on feature/pc.\"" || clean_ok=0
check promote-clean "$clean_ok" "promote on a clean main checkout: the reef note names only the worktree path and branch"

mkdir -p "$S/p153/.claude"
git -C "$S/p153" init -q -b main
echo one >"$S/p153/a.txt"
git -C "$S/p153" add a.txt
git -C "$S/p153" -c user.name=v -c user.email=v@example.com commit -qm init
printf '{"permissions":{"allow":["Edit","Write","Edit(*)","Write(*)"]}}' >"$S/p153/.claude/settings.json"
api_call POST /projects '{"name":"p153","path":"'"$S"'/p153"}'
p153_id=$("$H" sql "select id from project where name = 'p153'" | tail -1)
api_call POST /channels '{"projectId":'"$p153_id"',"name":"settings q","team":"pair"}'
settingsq_chan=$("$H" sql "select id from channel where name = 'settings q'" | tail -1)
api_call POST "/channels/$settingsq_chan/send" '{"text":"@tester Use the Edit tool to replace the word one with nope in a.txt. Say what happened."}'
wait_done "$settingsq_chan" 1
settings_ok=1
[ "$(cat "$S/p153/a.txt")" = one ] || settings_ok=0
[ "$(git -C "$S/p153" status --porcelain)" = "?? .claude/" ] || settings_ok=0
settings_sess=$("$H" sql "select session_id from turn where channel_id = $settingsq_chan order by id desc limit 1" | tail -1)
settings_file=$(session_file "$settings_sess")
if [ -n "$settings_file" ]; then grep -q '"is_error":true' "$settings_file" || settings_ok=0; else settings_ok=0; fi
check write-settings "$settings_ok" "a project .claude/settings.json allowing Edit still refuses the tester"
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

for sec in full cx inbox ui2 google fu; do
	section "$sec" &
done
wait

for id in $(sed -n '/^## Sub-features/,/^## /s/^- `\([a-z0-9-]*\)`.*/\1/p' "$(dirname "$0")/full-team.md"); do
	line=$(grep -h "^\(PASS\|FAIL\) $id:" "$E"/*/full-team.log 2>/dev/null | head -1)
	if [ -n "$line" ]; then
		echo "$line"
		case $line in FAIL*) failed=1 ;; esac
	else
		not_run "$id" "no PASS or FAIL line in $E/*/full-team.log"
	fi
done

rm -rf "$HOME"/.claude/projects/"$(printf '%s' "$S" | sed 's/[^A-Za-z0-9]/-/g')"-*

exit "$failed"
