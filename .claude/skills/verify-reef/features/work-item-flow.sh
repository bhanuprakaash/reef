#!/usr/bin/env bash
# Drives the flow sub-features of queue-and-review.md, work-items.md, conversations.md and
# caps.md and prints one PASS/FAIL line per sub-feature id. Called by `verify-reef feature
# work-item-flow [section...]`, which exports H, REEF_VERIFY_RUN, E and S and runs from the
# repo root. See ../SKILL.md and those files (the recipes).
set -u
: "${H:?}" "${REEF_VERIFY_RUN:?}" "${E:?}" "${S:?}"
export REEF_VERIFY_RUN

all_sections="flow conv caps"
sections="$*"
for sec in $sections; do
	case " $all_sections " in *" $sec "*) ;; *) echo "work-item-flow: unknown section '$sec'; sections: $all_sections" >&2; exit 2 ;; esac
done
sections=${sections:-$all_sections}

failed=0
check() { # check ID OK DETAIL
	if [ "$2" = 1 ]; then echo "PASS $1: $3"; else echo "FAIL $1: $3"; failed=1; fi
}
not_run() { echo "FAIL $1: not run: $2"; failed=1; }

api_call() { # api_call METHOD PATH [JSON]: sets STATUS and BODY from one $H api call.
	local out
	out=$("$H" api "$@")
	STATUS=$(printf '%s\n' "$out" | head -1 | cut -d' ' -f2)
	BODY=$(printf '%s\n' "$out" | tail -n +2)
}

wait_for() { for i in $(seq 300); do curl -s "http://$A$1" | grep -q "$2" && return 0; sleep 1; done; echo "timeout: $1 never had $2" >&2; return 1; } # wait_for PATH TEXT
wait_sql() { for i in $(seq 300); do [ "$("$H" sql "$1" | tail -1)" = "$2" ] && return 0; sleep 1; done; echo "timeout: '$1' never gave '$2'" >&2; return 1; } # wait_sql QUERY WANT
review_patch() { for i in $(seq 300); do api_call PATCH "/work-items/$1" "$2"; [ "$STATUS" = 409 ] && echo "$BODY" | grep -q 'wait for the wrap-up' || return 0; sleep 1; done; } # review_patch ID JSON: PATCH an item in review, again while its wrap-up runs
replays() { grep -cE "(scenario=|record: recorded )$1( |\$)" "$E/fake-claude.log"; } # replays SCENARIO -> turns that replayed or recorded it
sql1() { "$H" sql "$1" | tail -1; } # sql1 QUERY -> the first column of the last row
item() { sql1 "select status || ':' || coalesce(queue_position, '-') from work_item where id = $1"; } # item ID -> status:position
wait_item() { wait_sql "select status || ':' || coalesce(queue_position, '-') from work_item where id = $1" "$2"; } # wait_item ID status:position
paused() { "$H" sql "select paused from space where id = $1" | tail -n +2; } # paused SPACE -> the pause reason, empty when not paused
spaces() { sql1 "select count(*) from space where work_item_id = $1"; } # spaces ITEM -> count
fyi() { curl -s "http://$A/inbox" | grep -o "\"kind\":\"$1\",\"group\":\"fyi\",\"spaceId\":[0-9]*,\"workItemId\":$2," | wc -l | tr -d ' '; } # fyi KIND ITEM -> count
start_server() { ANTHROPIC_MODEL=sonnet "$H" serve-start --data-name "$1" >/dev/null; A=$(cat "$S/serve.addr"); } # start_server DATA-NAME

### Team files in $E

cat >"$E/crew.json" <<'EOF'
{
  "name": "crew",
  "responsibility": "Checkout and search pages: forms, results and their tests.",
  "roles": [
    {"name": "lead", "lead": true, "backend": "claude",
     "description": "You are the lead. For the messages below do exactly what is written, with no other tool call and no other words. When Work item 1, Checkout form, starts: call create_sub_item with title 'Build the form' and description 'form.txt with the two fields'; when it has returned, call create_sub_item with title 'Check the form' and description 'form.txt exists'; then answer with the one line '@developer build the form'. When the tester writes that the form passes: call set_sub_item_status with status done for each of the two sub-items, then call submit_for_review with acceptance line 1 checked, the note 'form.txt has both fields' and the one command 'grep -c password form.txt' with output '1'; then answer 'Submitted for review.' When the user sends the work item back: call submit_for_review with acceptance line 1 checked, the note 'phone field is out of scope for the form' and the one command 'grep -c email form.txt' with output '1'; then answer 'Submitted again.' When any other work item starts: answer in one short sentence and call no tool. When you are asked to wrap up the Space, do what that message asks."},
    {"name": "developer", "write": true, "backend": "claude", "permissionMode": "acceptEdits",
     "description": "You are the developer. When the lead writes '@developer build the form': call Write once to create form.txt in the current folder with the two lines email and password, then answer with the one line '@tester check the form'. Call no other tool and write no other words.",
     "tools": ["Read", "Edit", "Write", "Bash"]},
    {"name": "tester", "backend": "claude", "permissionMode": "acceptEdits",
     "description": "You are the tester. When the developer writes '@tester check the form': run the one Bash command grep -c password form.txt, then answer with the one line '@lead the form passes'. Call no other tool and write no other words.",
     "tools": ["Read", "Bash(grep:*)"]}
  ]
}
EOF

cat >"$E/pong.json" <<'EOF'
{
  "name": "pong",
  "roles": [
    {"name": "lead", "lead": true, "backend": "claude",
     "description": "You are the lead. Follow these rules with no other tool call and no other words. When the user writes workpong: answer with the one line '@developer work'. When the user writes talkpong, or the developer writes '@lead pong': call post_to_space once with the body '@developer ping', then answer 'posted'."},
    {"name": "developer", "write": true, "backend": "claude", "permissionMode": "acceptEdits",
     "description": "You are the developer. Follow these rules with no other tool call and no other words. When the lead writes '@developer work' or the tester writes '@developer again': run the one Bash command echo work >> work.txt, then answer with the one line '@tester check'. When the lead writes '@developer ping': call post_to_space once with the body '@lead pong', then answer 'posted'.",
     "tools": ["Bash"]},
    {"name": "tester", "backend": "claude", "permissionMode": "acceptEdits",
     "description": "You are the tester. When the developer writes '@tester check': run the one Bash command grep -c work work.txt, then answer with the one line '@developer again'. Call no other tool and write no other words.",
     "tools": ["Bash(grep:*)"]}
  ]
}
EOF

section_flow() {
### Two work items owned by the lead of the crew, one queued (data folder flow)

start_server flow
"$H" team-apply "$E/crew.json" >/dev/null
lead=$(sql1 "select id from bot where name = 'lead'")
dev=$(sql1 "select id from bot where name = 'developer'")
tester=$(sql1 "select id from bot where name = 'tester'")
api_call POST /projects '{"name":"shop","path":"'"$S"'/cwd"}'
api_call POST /work-items '{"projectId":1,"title":"Checkout form","description":"a form that takes an email and a password","ownerBotId":'"$lead"',"participantBotIds":['"$dev,$tester"'],"acceptanceLines":[{"text":"the form has an email and a password field"}]}'
api_call POST /work-items '{"projectId":1,"title":"Order history","ownerBotId":'"$lead"',"participantBotIds":['"$dev,$tester"']}'
api_call POST /work-items/1/approve '{"key":"k1"}'

queue_ok=1
[ "$STATUS" = 200 ] || queue_ok=0
[ "$(item 1)" = "in_progress:-" ] || queue_ok=0
[ "$(item 2)" = "backlog:-" ] || queue_ok=0
[ "$(spaces 1)" = 1 ] || queue_ok=0
[ "$(spaces 2)" = 0 ] || queue_ok=0
[ "$(sql1 "select group_concat(bot_id || ':' || lead) from (select bot_id, lead from space_member where space_id = 1 order by id)")" = "$lead:1,$dev:0,$tester:0" ] || queue_ok=0
first=$("$H" sql "select body from message where space_id = 1 and author = 'user' order by id limit 1" | tail -n +2 | head -1)
[ "$first" = "Work item 1: Checkout form" ] || queue_ok=0
check flow-queue-starts "$queue_ok" "item 1 $(item 1), item 2 $(item 2), Spaces of item 1: $(spaces 1) and of item 2: $(spaces 2); first prompt: $first"

### The lead makes sub-items; developer and tester work; the lead submits

wait_for "/work-items?parent=1" '"title":"Check the form"'
sub_body=$(curl -s "http://$A/work-items?parent=1")
sub_ok=1
for want in '"id":3,"projectId":1,"parentId":1,"type":"task","title":"Build the form"' '"id":4,"projectId":1,"parentId":1,"type":"task","title":"Check the form"'; do
	echo "$sub_body" | grep -qF "$want" || sub_ok=0
done
[ "$(fyi sub_item 3)" = 1 ] || sub_ok=0
[ "$(fyi sub_item 4)" = 1 ] || sub_ok=0
[ "$(spaces 3)" = 0 ] || sub_ok=0
check flow-sub-items "$sub_ok" "sub-items of item 1: $(echo "$sub_body" | grep -o '"title":"[^"]*"' | tr '\n' ' '); sub_item FYI items for 3 and 4: $(fyi sub_item 3) and $(fyi sub_item 4); Spaces of the sub-items: $(spaces 3)"

wait_item 1 "review:-"
wait_sql "select count(*) >= 2 from turn where space_id = 1 and role = 'lead' and status = 'done'" 1
rows=$("$H" sql "select role, status from turn where space_id = 1 and status = 'done' order by id" | tail -n +2 | tr '\n' ' ')
tools=$("$H" sql "select turn.role, t.name, count(*) from tool_call t join turn on turn.id = t.turn_id where turn.space_id = 1 and t.name in ('Write', 'Bash') group by turn.role, t.name" | tail -n +2 | tr '\n' ' ')
tree="$S/flow/worktrees/1/1"
work_ok=1
case $rows in "lead|done developer|done tester|done lead|done "*) ;; *) work_ok=0 ;; esac
[ "$tools" = "developer|Write|1 tester|Bash|1 " ] || work_ok=0
[ "$(cat "$tree/form.txt" 2>/dev/null | tr '\n' ' ')" = "email password " ] || work_ok=0
check flow-team-works "$work_ok" "done turns of Space 1: $rows; Write and Bash calls: $tools; $tree/form.txt: $(cat "$tree/form.txt" 2>/dev/null | tr '\n' ' ')"

api_call GET /work-items/1/proof
proof=$BODY
submit_ok=1
[ "$STATUS" = 200 ] || submit_ok=0
echo "$proof" | grep -q '"checked":true,"note":"form.txt has both fields"' || submit_ok=0
echo "$proof" | grep -q '"commands":\[{"command":"grep -c password form.txt","output":"1"}\]' || submit_ok=0
[ "$(sql1 "select group_concat(status) from work_item where parent_id = 1")" = "done,done" ] || submit_ok=0
wait_for "/spaces/1/proposal" '"kind":"wrap"' || submit_ok=0
[ "$(sql1 "select status from proposal where space_id = 1 order by id desc limit 1")" = pending ] || submit_ok=0
check flow-submit-review "$submit_ok" "item 1 $(item 1), sub-items $(sql1 "select group_concat(status) from work_item where parent_id = 1"); proof: $STATUS $(echo "$proof" | cut -c1-200); wrap-up of Space 1: $(sql1 "select status from proposal where space_id = 1 order by id desc limit 1")"

### The second item, queued while the first is in review, starts at once

api_call POST /work-items/2/approve '{"key":"k2"}'
wait_item 2 "in_progress:-"
wait_for "/spaces?workItem=2" '"workItemId":2'
first2=$("$H" sql "select body from message where space_id = 2 and author = 'user' order by id limit 1" | tail -n +2 | head -1)
next_ok=1
[ "$(spaces 2)" = 1 ] || next_ok=0
[ "$first2" = "Work item 2: Order history" ] || next_ok=0
[ "$(fyi started 2)" = 1 ] || next_ok=0
check flow-next-starts "$next_ok" "item 2 $(item 2), Spaces of item 2: $(spaces 2), first prompt: $first2, started FYI items: $(fyi started 2)"

### Send item 1 back: it restarts at once, next to item 2

review_patch 1 '{"status":"queued","note":"ADD A PHONE FIELD"}'
back_ok=1
[ "$STATUS" = 200 ] || back_ok=0
[ "$(sql1 "select status in ('in_progress', 'review') and queue_position is null from work_item where id = 1")" = 1 ] || back_ok=0
[ "$(item 2)" = "in_progress:-" ] || back_ok=0
[ "$(sql1 "select status from proposal where space_id = 1 order by id desc limit 1")" = rejected ] || back_ok=0
[ "$(spaces 1)" = 1 ] || back_ok=0
check flow-send-back "$back_ok" "PATCH queued with a note: $STATUS; item 1 $(item 1), item 2 $(item 2), wrap-up of Space 1: $(sql1 "select status from proposal where space_id = 1 order by id desc limit 1"), Spaces of item 1: $(spaces 1)"

### Item 2 is dropped; item 1 heard the note in its own Space and submits again

api_call PATCH /work-items/2 '{"status":"dropped"}'
resubmit_ok=1
[ "$STATUS" = 200 ] || resubmit_ok=0
wait_for "/work-items/1/proof" 'phone field is out of scope for the form' || resubmit_ok=0
wait_item 1 "review:-" || resubmit_ok=0
wait_sql "select status from proposal where space_id = 1 order by id desc limit 1" pending || resubmit_ok=0
note=$("$H" sql "select body from message where space_id = 1 and author = 'user' and body like 'Sent back%' order by id desc limit 1" | tail -n +2)
[ "$note" = "$(printf 'Sent back from review by the user:\nADD A PHONE FIELD')" ] || resubmit_ok=0
[ "$(replays flow-lead-resubmit)" = 1 ] || resubmit_ok=0
[ "$(item 2)" = "dropped:-" ] || resubmit_ok=0
[ "$(spaces 1)" = 1 ] || resubmit_ok=0
check flow-resubmit "$resubmit_ok" "item 2 dropped, item 1 $(item 1) with Spaces: $(spaces 1); sent-back message of Space 1: $(echo "$note" | tr '\n' '|'); flow-lead-resubmit replays: $(replays flow-lead-resubmit); wrap-up of Space 1: $(sql1 "select status from proposal where space_id = 1 order by id desc limit 1")"

### Done: the wrap-up is approved with it

review_patch 1 '{"status":"done"}'
done_ok=1
[ "$STATUS" = 200 ] || done_ok=0
[ "$(item 1)" = "done:-" ] || done_ok=0
[ "$(sql1 "select status from proposal where space_id = 1 order by id desc limit 1")" = approved ] || done_ok=0
[ "$(sql1 "select count(*) from brief where work_item_id = 1")" -ge 1 ] || done_ok=0
[ "$(sql1 "select count(*) from space where work_item_id = 1 and closed_at is not null")" = 1 ] || done_ok=0
check flow-done "$done_ok" "PATCH done: $STATUS; item 1 $(item 1); wrap-up $(sql1 "select status from proposal where space_id = 1 order by id desc limit 1"); briefs of item 1: $(sql1 "select count(*) from brief where work_item_id = 1"); closed Spaces of item 1: $(sql1 "select count(*) from space where work_item_id = 1 and closed_at is not null")"
}

section_conv() {
### A conversation with sam, kit added, kit proposes a work item (data folder conv)

start_server conv
api_call POST /projects '{"name":"talk","path":"'"$S"'/cwd"}'
api_call POST /bots '{"name":"sam","backend":"claude","write":false}'
api_call POST /bots '{"name":"kit","backend":"claude","write":false,"description":"You are kit. Unless a message asks you for a tool call, call no tool, answer in one short sentence and start no line with @."}'
"$H" team-apply "$E/crew.json" >/dev/null
api_call POST /spaces '{"projectId":1,"name":"plan","botId":2,"kind":"conversation"}'
api_call POST /spaces/1/send '{"text":"Ada wants a login page with an email field and a Google button. Use no tools. Acknowledge in one short sentence."}'
wait_sql "select count(*) from turn where space_id = 1 and status = 'done'" 1
conv_ok=1
[ "$STATUS" = 202 ] || conv_ok=0
[ "$(sql1 "select kind || '|' || coalesce(work_item_id, '-') || '|' || coalesce(team_id, '-') from space where id = 1")" = "conversation|-|-" ] || conv_ok=0
[ "$(sql1 "select role from turn where space_id = 1")" = sam ] || conv_ok=0
[ "$(replays sam-login-plan)" = 1 ] || conv_ok=0
check flow-conversation "$conv_ok" "conversation Space 1: $(sql1 "select kind || ' work item ' || coalesce(work_item_id, 'none') from space where id = 1"); its turn: $(sql1 "select role || ' ' || status from turn where space_id = 1")"

api_call POST /spaces/1/members '{"botId":3}'
add_status=$STATUS
wait_sql "select count(*) from turn where space_id = 2 and role = 'kit' and status = 'done'" 1
add_ok=1
[ "$add_status" = 202 ] || add_ok=0
api_call GET /spaces/1
echo "$BODY" | grep -q '"movedTo":2' || add_ok=0
api_call POST /spaces/1/send '{"text":"more"}'
[ "$STATUS" = 409 ] || add_ok=0
[ "$(sql1 "select kind || '|' || (team_id is null) from space where id = 2")" = "free|1" ] || add_ok=0
[ "$("$H" sql "select bot.name || '|' || lead || '|' || write from space_member member join bot on bot.id = bot_id where space_id = 2 order by member.id" | tail -n +2 | tr '\n' ' ')" = "sam|1|0 kit|0|0 " ] || add_ok=0
[ "$(replays kit-takeover)" = 1 ] || add_ok=0
check flow-add-bot "$add_ok" "POST members: $add_status; Space 1 moved to Space 2 ($(sql1 "select kind from space where id = 2"), members: $("$H" sql "select bot.name from space_member member join bot on bot.id = bot_id where space_id = 2 order by member.id" | tail -n +2 | tr '\n' ' ')); send to Space 1: $STATUS; kit-takeover replays: $(replays kit-takeover)"

api_call POST /spaces/2/send '{"text":"@kit PROPOSE THE SLOW SEARCH: call propose_work_item once with type bug, title \"Search results load slowly\", description \"The search page takes ten seconds to show results.\" and the one acceptance line \"Results show within two seconds\", then answer Proposed."}'
wait_sql "select count(*) from work_item" 1
api_call GET /work-items/1
propose_ok=1
echo "$BODY" | grep -q '"type":"bug","title":"Search results load slowly"' || propose_ok=0
echo "$BODY" | grep -q '"status":"backlog"' || propose_ok=0
echo "$BODY" | grep -q '"proposedBy":"kit"' || propose_ok=0
echo "$BODY" | grep -q '"ownerBotId":null' || propose_ok=0
echo "$BODY" | grep -q '"text":"Results show within two seconds"' || propose_ok=0
[ "$(fyi proposed 1)" = 1 ] || propose_ok=0
[ "$(replays flow-propose-kit)" = 1 ] || propose_ok=0
check flow-propose "$propose_ok" "item 1: $(echo "$BODY" | cut -c1-300); proposed FYI items: $(fyi proposed 1); flow-propose-kit replays: $(replays flow-propose-kit)"
}

section_caps() {
### Hop cap with the team pong (data folder caps)

start_server caps
"$H" team-apply "$E/pong.json" >/dev/null
api_call POST /projects '{"name":"hop","path":"'"$S"'/cwd"}'
api_call POST /work-items '{"projectId":1,"title":"workpong"}'
api_call POST /work-items '{"projectId":1,"title":"talkpong"}'
"$H" sql --write "update work_item set status = 'in_progress' where id in (1, 2)" >/dev/null
api_call POST /spaces '{"projectId":1,"workItemId":1,"name":"s1","team":"pong"}'
api_call POST /spaces '{"projectId":1,"workItemId":2,"name":"s2","team":"pong"}'

api_call POST /spaces/1/send '{"text":"workpong"}'
api_call POST /spaces/2/send '{"text":"talkpong"}'

wait_sql "select count(*) from turn where space_id = 1 and status = 'done'" 13
api_call POST /spaces/1/stop
work_ok=1
[ "$STATUS" = 204 ] || work_ok=0
first13="select id from turn where space_id = 1 and status = 'done' order by id limit 13"
seq=$("$H" sql "select role from ($first13) join turn using (id) order by id" | tail -n +2 | tr '\n' ' ')
[ "$seq" = "lead developer tester developer tester developer tester developer tester developer tester developer tester " ] || work_ok=0
[ -z "$(paused 1)" ] || work_ok=0
calls=$("$H" sql "select turn.role, t.name, count(*) from tool_call t join turn on turn.id = t.turn_id where turn.id in ($first13) group by turn.role, t.name order by 1" | tail -n +2 | tr '\n' ' ')
[ "$calls" = "developer|Bash|6 tester|Bash|6 " ] || work_ok=0
check hop-work-resets "$work_ok" "done turns of Space 1: $seq; paused: '$(paused 1)'; tool calls: $calls"

wait_for "/spaces/2" '"paused":"hop cap'
sleep 1
talk_ok=1
seq=$("$H" sql "select role from turn where space_id = 2 order by id" | tail -n +2 | tr '\n' ' ')
[ "$seq" = "lead developer lead developer lead developer lead " ] || talk_ok=0
[ "$("$H" sql "select distinct status from turn where space_id = 2" | tail -n +2 | tr '\n' ' ')" = "done " ] || talk_ok=0
talk_calls=$("$H" sql "select t.name, count(*) from tool_call t join turn on turn.id = t.turn_id where turn.space_id = 2 and t.name != 'ToolSearch' group by t.name" | tail -n +2 | tr '\n' ' ')
[ "$talk_calls" = "mcp__reef__post_to_space|7 " ] || talk_ok=0
[ "$(item 2)" = "needs_you:-" ] || talk_ok=0
inbox=$(curl -s "http://$A/inbox")
echo "$inbox" | grep -q '"kind":"held","group":"needs_you","spaceId":2,"workItemId":2,"text":"hop cap: 6 agent hops without a user message (1 wake held)"' || talk_ok=0
check hop-talk-pauses "$talk_ok" "turns of Space 2: $seq; tool calls besides ToolSearch: $talk_calls; item 2 $(item 2); paused: $(paused 2); inbox: $(echo "$inbox" | cut -c1-240)"
}

# section NAME: run section_NAME under the run id $REEF_VERIFY_RUN/NAME, so it has its own
# scratch folder, evidence folder and server; its output goes to $E/NAME/work-item-flow.log.
section() (
	export REEF_VERIFY_RUN=$REEF_VERIFY_RUN/$1
	"$H" build >/dev/null || exit 1
	"$H" doctor >/dev/null
	cp "$E/crew.json" "$E/pong.json" "$E/$1/"
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
	} >"$E/work-item-flow.log" 2>&1
)

for sec in $sections; do
	section "$sec" &
done
wait

here=$(dirname "$0")
if [ "$sections" != "$all_sections" ]; then
	for sec in $sections; do
		lines=$(grep -h '^\(PASS\|FAIL\) ' "$E/$sec/work-item-flow.log" 2>/dev/null)
		if [ -z "$lines" ]; then
			not_run "section-$sec" "no PASS or FAIL line in $E/$sec/work-item-flow.log"
			continue
		fi
		echo "$lines"
		echo "$lines" | grep -q '^FAIL' && failed=1
	done
else
	for id in $(sed -nE '/^## Sub-features/,/^## /s/^- `(flow-[a-z-]*|hop-[a-z-]*)`.*/\1/p' "$here/queue-and-review.md" "$here/work-items.md" "$here/conversations.md" "$here/caps.md"); do
		line=$(grep -h "^\(PASS\|FAIL\) $id:" "$E"/*/work-item-flow.log 2>/dev/null | head -1)
		if [ -n "$line" ]; then
			echo "$line"
			case $line in FAIL*) failed=1 ;; esac
		else
			not_run "$id" "no PASS or FAIL line in $E/*/work-item-flow.log"
		fi
	done
fi

rm -rf "$HOME"/.claude/projects/"$(printf '%s' "$S" | sed 's/[^A-Za-z0-9]/-/g')"-*

exit "$failed"
