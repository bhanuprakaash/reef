#!/usr/bin/env bash
# The real-claude set: the lines that test claude's own behaviour, run on real claude.
# Called by `verify-reef real`, which exports H, REEF_VERIFY_RUN, E, S, forces
# REEF_VERIFY_CLAUDE=real and ANTHROPIC_MODEL=sonnet. See ../SKILL.md, ../features/full-team.md,
# ../features/turn.md, ../features/subagents.md.
set -u
: "${H:?}" "${REEF_VERIFY_RUN:?}" "${E:?}" "${S:?}"
export REEF_VERIFY_RUN

failed=0
check() { # check ID OK DETAIL
	if [ "$2" = 1 ]; then echo "PASS $1: $3"; else echo "FAIL $1: $3"; failed=1; fi
}

wait_done() { for i in $(seq 60); do [ "$(curl -s "http://$A/turns?channel=$1&status=done" | grep -o '"id"' | wc -l)" = "$2" ] && { sleep 1; return 0; }; sleep 3; done; echo "timeout: channel $1 never had $2 done turns" >&2; return 1; }

# --- turn-permission: turn.md, allowed tools run, denied tools give isError ---
printf 'reef-read-ok' >"$S/cwd/real-note.txt"
"$H" run --name real-turn-allow -- --backend claude --permission-mode default \
	"Use the Read tool on real-note.txt in the current directory and tell me its contents." >/dev/null
allow_ok=0
grep -q '"toolName":"Read"' "$E/real-turn-allow.jsonl" 2>/dev/null || allow_ok=1
grep -q '"isError":true' "$E/real-turn-allow.jsonl" 2>/dev/null && allow_ok=1
grep -q 'reef-read-ok' "$E/real-turn-allow.jsonl" 2>/dev/null || allow_ok=1

"$H" run --name real-turn-deny -- --backend claude --permission-mode default \
	"Create the file real-deny.txt with the Write tool, content x" >/dev/null
deny_ok=0
grep -q '"isError":true' "$E/real-turn-deny.jsonl" 2>/dev/null || deny_ok=1
[ ! -e "$S/cwd/real-deny.txt" ] || deny_ok=1

if [ "$allow_ok" = 0 ] && [ "$deny_ok" = 0 ]; then
	check turn-permission 1 "Read ran and returned reef-read-ok; Write under --permission-mode default gave isError and wrote no file"
else
	check turn-permission 0 "allow ok=$allow_ok deny ok=$deny_ok; see real-turn-allow.jsonl, real-turn-deny.jsonl"
fi

# --- subagent-parent, subagent-top-level: subagents.md ---
printf 'sub-hello' >"$S/cwd/real-sub.txt"
sub_ok=0 agent_id="" detail=""
for attempt in 1 2; do
	name="real-subagent-$attempt"
	"$H" run --name "$name" -- --backend claude \
		"Use the Agent tool to start a subagent that reads the file real-sub.txt in the current directory with the Read tool. Then tell me its contents." >/dev/null
	jsonl="$E/$name.jsonl"
	agent_id=$(python3 -I -c '
import json, sys
for l in open(sys.argv[1]):
    try:
        e = json.loads(l)
    except Exception:
        continue
    if e.get("kind") == "tool_call" and e.get("toolName") == "Agent" and not e.get("parentToolId"):
        print(e.get("toolId", "")); break
' "$jsonl" 2>/dev/null)
	[ -n "$agent_id" ] && break
done

if [ -z "$agent_id" ]; then
	check subagent-parent 0 "no Agent tool_call with no parentToolId after 2 attempts"
	check subagent-top-level 0 "no Agent tool_call with no parentToolId after 2 attempts"
else
	jsonl="$E/real-subagent-$attempt.jsonl"
	child_ok=$(python3 -I -c '
import json, sys
agent_id = sys.argv[2]
call_ok = result_ok = False
for l in open(sys.argv[1]):
    try:
        e = json.loads(l)
    except Exception:
        continue
    if e.get("parentToolId") != agent_id:
        continue
    if e.get("kind") == "tool_call" and e.get("toolName") == "Read":
        call_ok = True
    if e.get("kind") == "tool_result" and "sub-hello" in (e.get("output") or ""):
        result_ok = True
print("1" if call_ok and result_ok else "0")
' "$jsonl" "$agent_id")
	if [ "$child_ok" = 1 ]; then
		check subagent-parent 1 "Read tool_call/tool_result under Agent toolId $agent_id, output has sub-hello"
	else
		check subagent-parent 0 "no matching Read tool_call/tool_result under parentToolId $agent_id; see $jsonl"
	fi
	agent_result_ok=$(python3 -I -c '
import json, sys
agent_id = sys.argv[2]
ok = False
for l in open(sys.argv[1]):
    try:
        e = json.loads(l)
    except Exception:
        continue
    if e.get("kind") == "tool_result" and e.get("toolId") == agent_id and not e.get("parentToolId"):
        ok = True
print("1" if ok else "0")
' "$jsonl" "$agent_id")
	if [ "$agent_result_ok" = 1 ]; then
		check subagent-top-level 1 "Agent tool_call and its tool_result (toolId $agent_id) carry no parentToolId"
	else
		check subagent-top-level 0 "no top-level tool_result for toolId $agent_id; see $jsonl"
	fi
fi

# --- write-claude-refused, write-settings: full-team.md, a tiny team, no worktree needed ---
cat >"$E/realteam.json" <<'EOF'
{
  "name": "realteam",
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

mkdir -p "$S/real-proj"
git -C "$S/real-proj" rev-parse --verify HEAD >/dev/null 2>&1 || {
	git -C "$S/real-proj" init -q -b main
	echo one >"$S/real-proj/a.txt"
	git -C "$S/real-proj" add a.txt
	git -C "$S/real-proj" -c user.name=v -c user.email=v@example.com commit -qm init
}

"$H" reef --data-name real -- team apply "$E/realteam.json" >/dev/null
"$H" serve-start --data-name real
A=$(cat "$S/serve.addr")
"$H" api POST /projects '{"name":"real","path":"'"$S"'/real-proj"}' >/dev/null
"$H" api POST /channels '{"projectId":1,"name":"real1","team":"realteam"}' >/dev/null
"$H" api POST /channels '{"projectId":1,"name":"real2","team":"realteam"}' >/dev/null

git -C "$S/real-proj" status --porcelain >"$S/real-status-before.txt"
"$H" api POST /channels/1/send '{"text":"@developer Write the numbers from 1 to 600 separated by spaces. Use no tools."}' >/dev/null
sleep 1
"$H" api POST /channels/2/send '{"text":"@tester Use the Write tool to create the file rtwrite.txt containing the word nope. If that is refused, try once with Bash. Say what happened."}' >/dev/null
sleep 4

dev_line="" tester_line=""
for p in $(pgrep -P "$(cat "$S/serve.pid")" -x claude); do
	line=$(ps -ww -o args= -p "$p")
	case $line in
	*--disallowedTools*) tester_line=$line ;;
	*) dev_line=$line ;;
	esac
done

wait_done 1 1
wait_done 2 1

refused_ok=1
case $dev_line in *--permission-mode\ acceptEdits*) ;; *) refused_ok=0 ;; esac
case $dev_line in *--disallowedTools*) refused_ok=0 ;; esac
case $tester_line in *--permission-mode\ default*) ;; *) refused_ok=0 ;; esac
case $tester_line in *--disallowedTools\ Edit,Write,MultiEdit,NotebookEdit*) ;; *) refused_ok=0 ;; esac
[ -e "$S/real-proj/rtwrite.txt" ] && refused_ok=0
git -C "$S/real-proj" status --porcelain | diff -q "$S/real-status-before.txt" - >/dev/null || refused_ok=0

tester_session=$("$H" sql "select session_id from turn where channel_id = 2 order by id desc limit 1" | tail -1)
tester_file=$(ls ~/.claude/projects/*/"$tester_session".jsonl 2>/dev/null | head -1)
if [ -n "$tester_file" ]; then
	errs=$(grep -c '"is_error":true' "$tester_file")
	[ "$errs" -ge 2 ] || refused_ok=0
else
	refused_ok=0
fi

if [ "$refused_ok" = 1 ]; then
	check write-claude-refused 1 "tester args had --permission-mode default --disallowedTools Edit,Write,MultiEdit,NotebookEdit, developer had acceptEdits and no --disallowedTools, rtwrite.txt absent, tree unchanged, $errs is_error tool results"
else
	check write-claude-refused 0 "dev_line=[$dev_line] tester_line=[$tester_line] rtwrite.txt exists=$([ -e "$S/real-proj/rtwrite.txt" ] && echo yes || echo no)"
fi

# --- write-settings: a project .claude/settings.json that allows Edit still refuses the tester ---
mkdir -p "$S/real-settings/.claude"
git -C "$S/real-settings" rev-parse --verify HEAD >/dev/null 2>&1 || {
	git -C "$S/real-settings" init -q -b main
	echo one >"$S/real-settings/a.txt"
	git -C "$S/real-settings" add a.txt
	git -C "$S/real-settings" -c user.name=v -c user.email=v@example.com commit -qm init
}
printf '{"permissions":{"allow":["Edit","Write","Edit(*)","Write(*)"]}}' >"$S/real-settings/.claude/settings.json"

"$H" api POST /projects '{"name":"realsettings","path":"'"$S"'/real-settings"}' >/dev/null
"$H" api POST /channels '{"projectId":2,"name":"real3","team":"realteam"}' >/dev/null
"$H" api POST /channels/3/send '{"text":"@tester Use the Edit tool to replace the word one with nope in a.txt. Say what happened."}' >/dev/null
wait_done 3 1

settings_ok=1
[ "$(cat "$S/real-settings/a.txt")" = one ] || settings_ok=0
[ "$(git -C "$S/real-settings" status --porcelain)" = "?? .claude/" ] || settings_ok=0
settings_session=$("$H" sql "select session_id from turn where channel_id = 3 order by id desc limit 1" | tail -1)
settings_file=$(ls ~/.claude/projects/*/"$settings_session".jsonl 2>/dev/null | head -1)
if [ -n "$settings_file" ]; then
	grep -q '"is_error":true' "$settings_file" || settings_ok=0
else
	settings_ok=0
fi

if [ "$settings_ok" = 1 ]; then
	check write-settings 1 "a.txt still 'one', git status only '?? .claude/', tester session has an is_error tool result"
else
	check write-settings 0 "a.txt=$(cat "$S/real-settings/a.txt" 2>/dev/null), git status=$(git -C "$S/real-settings" status --porcelain 2>/dev/null)"
fi

"$H" serve-stop

exit "$failed"
