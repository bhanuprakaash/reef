#!/usr/bin/env bash
# Drives every line of queue-and-review.md and prints one PASS/FAIL line per sub-feature id.
# Called by `verify-reef feature queue-and-review`, which exports H, REEF_VERIFY_RUN, E and S and runs
# from the repo root. See ../SKILL.md and queue-and-review.md (the recipe).
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

wait_done() { for i in $(seq 40); do [ "$(curl -s "http://$A/turns?space=$1&status=done" | grep -o '"id"' | wc -l | tr -d ' ')" = "$2" ] && return 0; sleep 1; done; echo "timeout: Space $1 never had $2 done turns" >&2; return 1; }
wait_for() { for i in $(seq 40); do curl -s "http://$A$1" | grep -q "$2" && return 0; sleep 1; done; echo "timeout: $1 never had $2" >&2; return 1; }
wait_item() { for i in $(seq 40); do [ "$(item "$1")" = "$2" ] && return 0; sleep 1; done; echo "timeout: item $1 never was $2" >&2; return 1; } # wait_item ID status:position
item() { "$H" sql "select status || ':' || coalesce(queue_position, '-') from work_item where id = $1" | tail -1; } # item ID -> status:position
spaces() { "$H" sql "select count(*) from space where work_item_id = $1" | tail -1; } # spaces ITEM -> count
started() { curl -s "http://$A/inbox" | grep -o "\"kind\":\"started\",\"group\":\"fyi\",\"spaceId\":0,\"workItemId\":$1," | wc -l | tr -d ' '; } # started ITEM -> count of its started items in the inbox
start_server() { ANTHROPIC_MODEL=sonnet "$H" serve-start --data-name queue >/dev/null; A=$(cat "$S/serve.addr"); }

### Project (a git repo with one commit), the solo team (id 1) and four work items

git -C "$S/cwd" init -q -b main
echo one >"$S/cwd/a.txt"
git -C "$S/cwd" add a.txt
git -C "$S/cwd" -c user.name=v -c user.email=v@example.com commit -qm init

start_server
api_call POST /projects '{"name":"app","path":"'"$S"'/cwd"}'
api_call POST /work-items '{"projectId":1,"title":"Login","description":"email and password","teamId":1,"acceptanceLines":[{"text":"wrong password is refused"},{"text":"session lasts a day"}]}'
api_call POST /work-items '{"projectId":1,"title":"Logout","teamId":1}'
api_call POST /work-items '{"projectId":1,"title":"Signup","teamId":1}'
api_call POST /work-items '{"projectId":1,"title":"Archive","teamId":1}'
api_call PUT /work-items/1/brief '{"body":"sessions, not JWT","author":"user"}'

### Queue the first item: it starts at once, with its Space and its first prompt

api_call PATCH /work-items/1 '{"status":"queued"}'
first_ok=1
[ "$STATUS" = 200 ] || first_ok=0
echo "$BODY" | grep -q '"status":"in_progress"' || first_ok=0
wait_done 1 1 || first_ok=0
first_ok_log=$(grep -c 'scenario=queue-lead' "$E/fake-claude.log")
[ "$first_ok_log" = 1 ] || first_ok=0
check queue-first-starts "$first_ok" "PATCH queued: $STATUS $BODY; Spaces of item 1: $(spaces 1); queue-lead replays: $first_ok_log"

prompt=$("$H" sql "select body from message where space_id = 1 and author = 'user'" | tail -n +2)
prompt_ok=1
for want in "Work item 1: Login" "Description: email and password" "1. wrong password is refused" "2. session lasts a day" "Brief: sessions, not JWT"; do
	echo "$prompt" | grep -qxF "$want" || prompt_ok=0
done
check queue-prompt "$prompt_ok" "first message of Space 1: $(echo "$prompt" | tr '\n' '|')"

### The second item waits while the first runs

api_call PATCH /work-items/2 '{"status":"queued"}'
second_ok=1
echo "$BODY" | grep -q '"status":"queued"' || second_ok=0
echo "$BODY" | grep -q '"queuePosition":1' || second_ok=0
[ "$(item 2)" = "queued:1" ] || second_ok=0
[ "$(spaces 2)" = 0 ] || second_ok=0
check queue-second-waits "$second_ok" "item 1 $(item 1), item 2 $(item 2), Spaces of item 2: $(spaces 2)"

### Needs you keeps the Team busy: a cap pause on item 1 does not start item 2

"$H" sql --write "update work_item set token_budget = 1 where id = 1" >/dev/null
api_call POST /spaces/1/send '{"text":"hello"}'
send_status=$STATUS
needs_ok=1
[ "$send_status" = 409 ] || needs_ok=0
[ "$(item 1)" = "needs_you:-" ] || needs_ok=0
[ "$(item 2)" = "queued:1" ] || needs_ok=0
[ "$(spaces 2)" = 0 ] || needs_ok=0
paused_detail="send over budget $send_status; item 1 $(item 1), item 2 $(item 2), Spaces of item 2: $(spaces 2)"
api_call POST /spaces/1/continue
[ "$STATUS" = 204 ] || needs_ok=0
[ "$(item 1)" = "in_progress:-" ] || needs_ok=0
[ "$(item 2)" = "queued:1" ] || needs_ok=0
check queue-needs-you-holds "$needs_ok" "$paused_detail; after continue item 1 $(item 1), item 2 $(item 2)"

### The lead of item 1 submits for review: item 2 starts live, with no restart and no PATCH

before=$(item 2)
api_call POST /spaces/1/send '{"text":"SUBMIT FOR REVIEW"}'
submit_ok=1
[ "$STATUS" = 202 ] || submit_ok=0
wait_item 1 "review:-" || submit_ok=0
wait_for "/spaces?workItem=2" '"workItemId":2' || submit_ok=0
wait_done 2 1 || submit_ok=0
[ "$before" = "queued:1" ] || submit_ok=0
[ "$(item 2)" = "in_progress:-" ] || submit_ok=0
note=$(started 2)
[ "$note" = 1 ] || submit_ok=0
[ "$("$H" sql "select count(*) from message where space_id = 2 and author = 'reef'" | tail -1)" = 0 ] || submit_ok=0
first=$("$H" sql "select body from message where space_id = 2 and author = 'user' order by id limit 1" | tail -n +2 | head -1)
[ "$first" = "Work item 2: Logout" ] || submit_ok=0
check queue-submit-starts "$submit_ok" "before submit item 2 $before; after: item 1 $(item 1), item 2 $(item 2), Spaces of item 2: $(spaces 2), started notes: $note, first prompt: $first"

api_call GET /work-items/1/proof
proof_body=$BODY
proof_ok=1
[ "$STATUS" = 200 ] || proof_ok=0
echo "$proof_body" | grep -q '"checked":true,"note":"a wrong password gets 401"' || proof_ok=0
echo "$proof_body" | grep -q '"commands":\[{"command":"go test ./...","output":"ok  login 0.4s"}\]' || proof_ok=0
wait_for "/spaces/1/proposal" '"kind":"wrap"' || proof_ok=0
check queue-submit-proof "$proof_ok" "proof of item 1: $STATUS $proof_body; wrap-up of Space 1: $(curl -s "http://$A/spaces/1/proposal" | cut -c1-120)"

### Item 2 submits through reef mcp while the server is down; the restart opens item 3

api_call PATCH /work-items/3 '{"status":"queued"}'
queued_before=$(item 3)
"$H" serve-stop
"$H" sql --write "insert into token (token, space_id, role, created_at) values ('lead-2', 2, 'lead', '2026-10-27T00:00:00Z')" >/dev/null
submit_out=$(REEF_TOKEN=lead-2 "$H" mcp-call submit_for_review '{"commands":[{"command":"true","output":""}]}')
before=$(item 3)
start_server
restart_ok=1
wait_for "/spaces?workItem=3" '"workItemId":3' || restart_ok=0
wait_done 3 1 || restart_ok=0
echo "$submit_out" | grep -q '< isError=false' || restart_ok=0
[ "$queued_before" = "queued:1" ] || restart_ok=0
[ "$(item 2)" = "review:-" ] || restart_ok=0
[ "$(item 3)" = "in_progress:-" ] || restart_ok=0
note=$(started 3)
[ "$note" = 1 ] || restart_ok=0
[ "$("$H" sql "select count(*) from message where space_id = 3 and author = 'reef'" | tail -1)" = 0 ] || restart_ok=0
check queue-restart-starts "$restart_ok" "item 3 queued $queued_before; submit while down: $(echo "$submit_out" | grep '^< ' | cut -c1-60); item 3 $before before the restart; after: item 2 $(item 2), item 3 $(item 3), Spaces of item 3: $(spaces 3), started notes: $note"

### The submit of item 2 was never heard by a live server: the restart runs its wrap-up, and Done approves it

wrap_ok=1
wait_for "/spaces/2/proposal" '"kind":"wrap"' || wrap_ok=0
wait_done 2 2 || wrap_ok=0
api_call DELETE /spaces/2
check queue-review-keeps-space "$([ "$STATUS" = 409 ] && echo 1 || echo 0)" "DELETE /spaces/2 while item 2 is in review: $STATUS $BODY; Spaces of item 2: $(spaces 2)"
api_call PATCH /work-items/2 '{"status":"done"}'
[ "$STATUS" = 200 ] || wrap_ok=0
[ "$("$H" sql "select status from proposal where space_id = 2 order by id desc limit 1" | tail -1)" = approved ] || wrap_ok=0
[ "$("$H" sql "select closed_at is not null from space where id = 2" | tail -1)" = 1 ] || wrap_ok=0
check queue-restart-wraps "$wrap_ok" "wrap-up of Space 2 after the restart: $("$H" sql "select status from proposal where space_id = 2 order by id desc limit 1" | tail -1); Done: $STATUS; Space 2 closed: $("$H" sql "select closed_at is not null from space where id = 2" | tail -1), item 2 $(item 2)"

### Send item 1 back: it goes to position 1, ahead of an item queued earlier, and starts next

api_call PATCH /work-items/4 '{"status":"queued"}'
c_pos=$(item 4)
api_call PATCH /work-items/1 '{"status":"queued","note":"SAY IT TWICE"}'
back_ok=1
[ "$STATUS" = 200 ] || back_ok=0
[ "$c_pos" = "queued:1" ] || back_ok=0
[ "$(item 1)" = "queued:1" ] || back_ok=0
[ "$(item 4)" = "queued:2" ] || back_ok=0
[ "$(item 3)" = "in_progress:-" ] || back_ok=0
check queue-send-back-first "$back_ok" "item 4 queued alone: $c_pos; after send back item 1 $(item 1), item 4 $(item 4), item 3 $(item 3)"

api_call PATCH /work-items/3 '{"status":"dropped"}'
next_ok=1
[ "$STATUS" = 200 ] || next_ok=0
[ "$(item 3)" = "dropped:-" ] || next_ok=0
[ "$(item 1)" = "in_progress:-" ] || next_ok=0
[ "$(item 4)" = "queued:1" ] || next_ok=0
[ "$(spaces 1)" = 1 ] || next_ok=0
check queue-send-back-starts "$next_ok" "after item 3 dropped: item 1 $(item 1), item 4 $(item 4), Spaces of item 1: $(spaces 1)"

for i in $(seq 40); do grep -q 'scenario=sent-back-lead' "$E/fake-claude.log" && break; sleep 1; done
note_prompt=$("$H" sql "select body from message where space_id = 1 and author = 'user' order by id desc limit 1" | tail -n +2)
note_ok=1
[ "$note_prompt" = "$(printf 'Sent back from review by the user:\nSAY IT TWICE')" ] || note_ok=0
[ "$(grep -c 'scenario=sent-back-lead' "$E/fake-claude.log")" = 1 ] || note_ok=0
[ "$("$H" sql "select status from proposal where space_id = 1 order by id desc limit 1" | tail -1)" = rejected ] || note_ok=0
check queue-send-back-note "$note_ok" "newest user message of Space 1: $(echo "$note_prompt" | tr '\n' '|'); sent-back-lead replays: $(grep -c 'scenario=sent-back-lead' "$E/fake-claude.log"); wrap-up proposal of Space 1: $("$H" sql "select status from proposal where space_id = 1 order by id desc limit 1" | tail -1)"

"$H" serve-stop

exit $failed
