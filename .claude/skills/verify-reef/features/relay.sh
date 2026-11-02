#!/usr/bin/env bash
# relay.sh: the user asks a Space with the team relay (lead + developer) for checks 1 to 3, one
# after another, and prints each role's turns, tokens and cost. Usage: REEF_VERIFY_RUN=ID relay.sh
set -eu
H=.claude/skills/verify-reef/scripts/verify-reef
"$H" serve-start --data-name "${RELAY_DATA:-relay}" >/dev/null
"$H" team-apply "$PWD/teams/relay.json" >/dev/null
"$H" api POST /projects '{"name":"relay","path":"/tmp"}' >/dev/null
"$H" api POST /spaces '{"projectId":1,"name":"relay","team":"relay"}' >/dev/null
idle() { # idle: no turn running for 5 s in a row
	local quiet=0
	for _ in $(seq 600); do
		if [ "$("$H" sql "select count(*) from turn where status in ('running','queued')" | tail -1)" = 0 ]; then
			quiet=$((quiet + 1)); [ "$quiet" -ge 5 ] && return 0
		else quiet=0; fi
		sleep 1
	done
	echo "timeout waiting for idle" >&2; return 1
}
for n in 1 2 3; do
	"$H" api POST /spaces/1/send "{\"text\":\"Run check $n.\"}" >/dev/null
	idle
done
"$H" sql "select id, role, status, cache_read_tokens, cache_write_tokens, input_tokens, output_tokens, printf('%.4f', cost_usd) cost from turn where space_id = 1 order by id"
"$H" sql "select role, count(*) turns, sum(cache_read_tokens) cache_read, sum(cache_read_tokens + cache_write_tokens + input_tokens) tokens_in, printf('%.4f', sum(cost_usd)) cost from turn where space_id = 1 group by role"
"$H" sql "select id, author, substr(replace(body, char(10), ' / '), 1, 160) body from message where space_id = 1 order by id"
"$H" serve-stop >/dev/null
