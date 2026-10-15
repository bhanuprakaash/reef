#!/usr/bin/env bash
# Drives every line of github.md and prints one PASS/FAIL line per sub-feature id.
# Called by `verify-reef feature github`, which exports H, REEF_VERIFY_RUN, E and S and runs
# from the repo root. See ../SKILL.md and github.md (the recipe).
set -u
: "${H:?}" "${REEF_VERIFY_RUN:?}" "${E:?}" "${S:?}"
export REEF_VERIFY_RUN

failed=0
check() { # check ID OK DETAIL
	if [ "$2" = 1 ]; then echo "PASS $1: $3"; else echo "FAIL $1: $3"; failed=1; fi
}

api_call() { # api_call METHOD PATH [JSON]: sets STATUS and BODY from one $H api call.
	local out
	out=$("$H" api "$@")
	STATUS=$(printf '%s\n' "$out" | head -1 | cut -d' ' -f2)
	BODY=$(printf '%s\n' "$out" | tail -n +2)
}

wait_for() { for i in $(seq 30); do curl -s "http://$A$1" | grep -q "$2" && return 0; sleep 1; done; echo "timeout: $1 never had $2" >&2; return 1; }
msg_count() { "$H" sql "select count(*) from message where channel_id = $1 and author = 'reef'" | tail -1; } # msg_count CHANNEL
turn_col() { "$H" sql "select $2 from turn where id = $1" | tail -n +2; } # turn_col ID COLUMN

### Project, team, fake gh

git -C "$S/cwd" init -q -b main
echo one >"$S/cwd/a.txt"
git -C "$S/cwd" add a.txt
git -C "$S/cwd" -c user.name=v -c user.email=v@example.com commit -qm init

"$H" reef --data-name github -- team apply "$PWD/teams/full.json" >/dev/null

GH="$E/gh"
mkdir -p "$GH/bin"
cat >"$GH/bin/gh" <<'EOF'
#!/bin/sh
if [ "$1" = run ] && [ "$2" = view ]; then
	cat "$GH_FAKE_LOG" 2>/dev/null
	exit 0
fi
if [ "$1" != pr ] || [ "$2" != view ]; then exit 1; fi
if [ -s "$GH_FAKE_ERR" ]; then cat "$GH_FAKE_ERR" >&2; exit 1; fi
cat "$GH_FAKE_RESPONSE"
EOF
chmod +x "$GH/bin/gh"
: >"$GH/err.txt"
: >"$GH/log.txt"
: >"$GH/resp.json"
export GH_FAKE_RESPONSE="$GH/resp.json" GH_FAKE_ERR="$GH/err.txt" GH_FAKE_LOG="$GH/log.txt"

REEF_GH_INTERVAL=1s PATH="$GH/bin:$PATH" ANTHROPIC_MODEL=sonnet "$H" serve-start --data-name github
A=$(cat "$S/serve.addr")

api_call POST /projects '{"name":"app","path":"'"$S"'/cwd"}'
api_call POST /features '{"projectId":1,"slug":"demo","name":"Demo"}'
api_call POST /features/1/worktree ""
api_call POST /channels '{"projectId":1,"featureId":1,"name":"main","team":"full"}'
api_call POST /features '{"projectId":1,"slug":"solo-demo","name":"Solo Demo"}'
api_call POST /features/2/worktree ""
api_call POST /channels '{"projectId":1,"featureId":2,"name":"only"}'

### PR note, and a repeat poll with the same answer

echo '{"number":12,"url":"https://github.com/x/y/pull/12","state":"OPEN","statusCheckRollup":[]}' >"$GH/resp.json"
wait_for '/messages?channel=1' 'PR #12 open' && pr_wait=0 || pr_wait=1
pr_body=$(curl -s "http://$A/messages?channel=1")
pr_ok=1
[ "$pr_wait" = 0 ] || pr_ok=0
echo "$pr_body" | grep -qF '"author":"reef","body":"PR #12 open: https://github.com/x/y/pull/12"' || pr_ok=0
check pr-note "$pr_ok" "channel 1 messages: $pr_body"

sleep 3
n1=$(msg_count 1)
check pr-repeat "$([ "$n1" = 1 ] && echo 1 || echo 0)" "message count after repeat polls = $n1, want 1"

### A finished successful check

echo '{"number":12,"url":"https://github.com/x/y/pull/12","state":"OPEN","statusCheckRollup":[' \
	'{"__typename":"CheckRun","name":"build","status":"COMPLETED","conclusion":"SUCCESS",' \
	'"detailsUrl":"https://github.com/x/y/actions/runs/1/job/1"}]}' | tr -d '\n' >"$GH/resp.json"
wait_for '/messages?channel=1' 'build succeeded' && succ_wait=0 || succ_wait=1
n2=$(msg_count 1)
check check-success "$([ "$succ_wait" = 0 ] && [ "$n2" = 2 ] && echo 1 || echo 0)" \
	"messages after the success check = $n2 (want 2): $(curl -s "http://$A/messages?channel=1")"

### A finished failed check: note, and a developer wake with the check name and log tail

printf 'FAIL TestFoo\nwant 2 got 1\n' >"$GH/log.txt"
echo '{"number":12,"url":"https://github.com/x/y/pull/12","state":"OPEN","statusCheckRollup":[' \
	'{"__typename":"CheckRun","name":"build","status":"COMPLETED","conclusion":"FAILURE",' \
	'"detailsUrl":"https://github.com/x/y/actions/runs/9/job/9"}]}' | tr -d '\n' >"$GH/resp.json"
wait_for '/messages?channel=1' 'build failed' && fail_wait=0 || fail_wait=1
sleep 2
turn1=$("$H" sql "select id from turn where channel_id = 1 and role = 'developer' order by id limit 1" | tail -1)
wake1=$(turn_col "$turn1" wake_text)
fail_ok=1
[ "$fail_wait" = 0 ] || fail_ok=0
[ -n "$turn1" ] || fail_ok=0
echo "$wake1" | grep -q build || fail_ok=0
echo "$wake1" | grep -qF 'PR #12' || fail_ok=0
echo "$wake1" | grep -qF 'want 2 got 1' || fail_ok=0
check check-failed-wake "$fail_ok" "turn $turn1 wake_text: $(echo "$wake1" | tr '\n' '|')"

### Restart: no repeat note, no repeat wake

n3=$(msg_count 1)
t3=$("$H" sql "select count(*) from turn where channel_id = 1" | tail -1)
"$H" serve-stop
REEF_GH_INTERVAL=1s PATH="$GH/bin:$PATH" ANTHROPIC_MODEL=sonnet "$H" serve-start --data-name github
A=$(cat "$S/serve.addr")
sleep 3
n4=$(msg_count 1)
t4=$("$H" sql "select count(*) from turn where channel_id = 1" | tail -1)
check restart-no-repeat "$([ "$n3" = "$n4" ] && [ "$t3" = "$t4" ] && echo 1 || echo 0)" \
	"messages $n3 -> $n4, turns $t3 -> $t4 across a restart"

### The same check name, a new run, failing again: a new note and a new wake

printf 'FAIL TestBar\nwant 3 got 4\n' >"$GH/log.txt"
echo '{"number":12,"url":"https://github.com/x/y/pull/12","state":"OPEN","statusCheckRollup":[' \
	'{"__typename":"CheckRun","name":"build","status":"COMPLETED","conclusion":"FAILURE",' \
	'"detailsUrl":"https://github.com/x/y/actions/runs/10/job/10"}]}' | tr -d '\n' >"$GH/resp.json"
rerun_wait=1
for i in $(seq 30); do [ "$(msg_count 1)" -gt "$n4" ] 2>/dev/null && { rerun_wait=0; break; }; sleep 1; done
sleep 2
n5=$(msg_count 1)
dev_turns=$("$H" sql "select count(*) from turn where channel_id = 1 and role = 'developer'" | tail -1)
turn2=$("$H" sql "select id from turn where channel_id = 1 and role = 'developer' order by id desc limit 1" | tail -1)
wake2=$(turn_col "$turn2" wake_text)
rerun_ok=1
[ "$rerun_wait" = 0 ] || rerun_ok=0
[ "$dev_turns" = 2 ] || rerun_ok=0
[ "$turn2" != "$turn1" ] || rerun_ok=0
echo "$wake2" | grep -qF 'want 3 got 4' || rerun_ok=0
check check-rerun-wakes-again "$rerun_ok" "messages $n4 -> $n5, developer turns=$dev_turns, turn $turn2 wake_text: $(echo "$wake2" | tr '\n' '|')"

### A solo-team feature channel: the note, no turn

wait_for '/messages?channel=2' 'build failed' && solo_wait=0 || solo_wait=1
turns_solo=$("$H" sql "select count(*) from turn where channel_id = 2" | tail -1)
check solo-note-only "$([ "$solo_wait" = 0 ] && [ "$turns_solo" = 0 ] && echo 1 || echo 0)" \
	"channel 2 messages: $(curl -s "http://$A/messages?channel=2" | tr -d '\n'); turns=$turns_solo"

"$H" serve-stop

### No gh on PATH: the server runs, posts nothing, and logs the cause once

NOGH="$E/no-gh-bin"
mkdir -p "$NOGH"
for d in /usr/local/sbin /usr/local/bin /usr/sbin /usr/bin /sbin /bin; do
	[ -d "$d" ] || continue
	for f in "$d"/*; do
		b=$(basename "$f")
		[ "$b" = gh ] && continue
		[ -e "$NOGH/$b" ] || ln -s "$f" "$NOGH/$b" 2>/dev/null
	done
done
PATH="$NOGH" REEF_GH_INTERVAL=1s ANTHROPIC_MODEL=sonnet "$H" serve-start --data-name github-nogh
sleep 3
health=$(curl -s -o /dev/null -w '%{http_code}' --max-time 2 "http://$(cat "$S/serve.addr")/health")
"$H" serve-stop
log_count=$(grep -c 'gh is not on PATH' "$E/serve.log")
check no-gh-logged-once "$([ "$health" = 200 ] && [ "$log_count" = 1 ] && echo 1 || echo 0)" \
	"/health was $health while gh was missing; 'gh is not on PATH' logged $log_count time(s)"

exit $failed
