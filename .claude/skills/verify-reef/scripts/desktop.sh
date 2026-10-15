#!/usr/bin/env bash
# Launch the Reef app (desktop/) with a scratch HOME and drive it: the window, every browser
# tool through reef mcp, the terminal pane on a feature, and quit. Called by
# `verify-reef desktop`, which exports H, REEF_VERIFY_RUN, E, S and runs it from the repo root.
# See ../SKILL.md (Desktop) and ../features/desktop.md.
set -u
: "${H:?}" "${REEF_VERIFY_RUN:?}" "${E:?}" "${S:?}"
export REEF_VERIFY_RUN

REPO=$(pwd)
CDP="$REPO/.claude/skills/verify-reef/scripts/cdp.mjs"
D="$E/desktop"
HOME_DIR="$S/home"
case $(uname) in
Darwin) rel="home/Library/Application Support/Reef" ;;
*) rel="home/.config/Reef" ;;
esac
DATA="$S/$rel"
SOCK="$DATA/browser.sock"

failed=0
check() { # check ID OK DETAIL
	if [ "$2" = 1 ]; then echo "PASS $1: $3"; else echo "FAIL $1: $3"; failed=1; fi
}

[ ! -e "$HOME_DIR" ] || { echo "FAIL desktop-setup: $HOME_DIR exists; desktop runs once per run id"; exit 1; }
mkdir -p "$D" "$DATA" "$S/page"

record() { # record PID: add PID and its command line to the run's pids, for cleanup
	c=$(ps -o command= -p "$1" 2>/dev/null | awk '{ $1 = $1; print }')
	[ -n "$c" ] && echo "$1 $c" >>"$S/pids"
}

sock_mode() { stat -c %a "$SOCK" 2>/dev/null || stat -f %Lp "$SOCK" 2>/dev/null; }

now() { python3 -I -c 'import time; print(f"{time.time():.3f}")'; }
since() { awk -v a="$(now)" -v b="$1" 'BEGIN { printf "%.2f", a - b }'; }

free_port() {
	python3 -I -c 'import socket; s = socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1])'
}

cdp() { # cdp EXPR [WINDOW]: a dead app fails at once, so the retry loops below end instead of waiting 10 s a try
	kill -0 "${app:-0}" 2>/dev/null || { echo "the app is not running" >&2; return 1; }
	node "$CDP" "$cdp_port" "$app_port" "$1" "${2:-}"
}

# shot NAME: the whole screen (Xvfb's on Linux) to $D/NAME.png; prints its standard deviation,
# 0 for a blank screen.
shot() {
	if [ "$(uname)" = Darwin ]; then
		screencapture -x "$D/$1.png"
	else
		DISPLAY=$(cat "$S/app.display") XAUTHORITY=$(cat "$S/app.xauth") import -window root "$D/$1.png"
	fi
	convert "$D/$1.png" -format '%[fx:standard_deviation]' info: 2>/dev/null || echo 0
}

nonzero() { awk -v v="$1" 'BEGIN { exit !(v > 0.01) }'; }

proc_cwd() {
	if [ "$(uname)" = Darwin ]; then lsof -a -p "$1" -d cwd -Fn | sed -n 's/^n//p'; else readlink "/proc/$1/cwd"; fi
}

proc_path() {
	if [ "$(uname)" = Darwin ]; then
		ps -Eww -o command= -p "$1" | tr ' ' '\n' | sed -n 's/^PATH=//p'
	else
		tr '\0' '\n' <"/proc/$1/environ" | sed -n 's/^PATH=//p'
	fi
}

descendants() {
	ps -axo pid=,ppid=,command= | awk -v root="$1" '
		{ pid[NR] = $1; ppid[NR] = $2; $1 = ""; $2 = ""; cmd[NR] = substr($0, 3) }
		END {
			want[root] = 1
			do {
				grew = 0
				for (i = 1; i <= NR; i++)
					if ((ppid[i] in want) && !(pid[i] in want)) { want[pid[i]] = 1; grew = 1 }
			} while (grew)
			for (i = 1; i <= NR; i++)
				if ((pid[i] in want) && pid[i] != root) print pid[i], cmd[i]
		}'
}

mcp() { REEF_TOKEN=$tok "$H" mcp-call "$@" 2>&1; }

# result_text: the "text" of the tool result in mcp-call's output on stdin.
result_text() {
	python3 -I -c 'import json, sys
for l in sys.stdin:
    if l.startswith("< isError=false "):
        print(json.loads(l.split(" ", 2)[2])["text"])'
}

page_pid="" app="" xvfb=""
stop_all() {
	[ -n "$page_pid" ] && kill "$page_pid" 2>/dev/null
	if [ -n "$app" ] && kill -0 "$app" 2>/dev/null; then
		kill -TERM "$app"
		for _ in $(seq 50); do kill -0 "$app" 2>/dev/null || break; sleep 0.2; done
		kill -KILL "$app" 2>/dev/null
	fi
}
trap stop_all EXIT

# --- build: the web app reef embeds, the desktop deps, and desktop/bin/reef the app starts ---
{
	(cd web && { [ -d node_modules ] || npm ci; } && npm run build) &&
		(cd desktop && { [ -d node_modules ] || npm ci; } && go build -o bin/reef ../cmd/reef)
} >"$D/build.log" 2>&1 || { echo "FAIL desktop-build: see $D/build.log"; exit 1; }
electron=$(cd desktop && node -p "require('electron')" | tail -n 1)
echo "build: web/dist, desktop/bin/reef, electron $electron"

# --- setup: a feature on the scratch cwd (a git repo, for its worktree) and a token for it ---
git -C "$S/cwd" rev-parse -q --verify HEAD >/dev/null 2>&1 || {
	git -C "$S/cwd" init -q -b main &&
		git -C "$S/cwd" -c user.name=v -c user.email=v@example.com commit -q --allow-empty -m init
}
"$H" reef --data-name "$rel" -- project add >/dev/null 2>&1
"$H" reef -- feature add term Terminal >/dev/null 2>&1
tok=$("$H" reef -- token term --role developer 2>/dev/null | sed -n 's/.*REEF_TOKEN=\([0-9a-f]*\).*/\1/p')
row=$("$H" sql "select id || ' ' || project_id from feature where slug = 'term'" | tail -n 1)
fid=${row% *} proj=${row#* }
[ -n "$tok" ] && [ -n "$fid" ] || { echo "FAIL desktop-setup: no feature or token; see $E/reef.log"; exit 1; }
WT="$DATA/worktrees/$proj/term"

# The app reads PATH from the login shell; with HOME scratch, these rc files are the user's.
if [ "${REEF_VERIFY_CLAUDE:-fake}" != real ]; then
	for f in .bash_profile .zprofile; do echo "export PATH=\"$S/fake:\$PATH\"" >"$HOME_DIR/$f"; done
fi

cat >"$S/page/index.html" <<'EOF'
<!doctype html>
<title>Reef test page</title>
<form id="f">
  <label>Name <input id="name"></label>
  <button>Send</button>
</form>
<p id="out">nobody yet</p>
<script>
  console.log('reef-page-loaded');
  console.error('reef-page-error');
  fetch('/missing.json');
  document.getElementById('f').onsubmit = (e) => {
    e.preventDefault();
    const name = document.getElementById('name').value;
    document.getElementById('out').textContent = 'hello ' + name;
    console.log('submitted ' + name);
  };
</script>
EOF
page_port=$(free_port)
URL="http://127.0.0.1:$page_port/"
python3 -m http.server --bind 127.0.0.1 -d "$S/page" "$page_port" >"$D/page.log" 2>&1 &
page_pid=$!
sleep 0.3
record "$page_pid"

# --- launch: on a free port (REEF_PORT) and its own X display, beside any other app or run ---
app_port=$(free_port)
cdp_port=$(free_port)
app_cmd=(env -u XDG_CONFIG_HOME HOME="$HOME_DIR" REEF_PORT="$app_port" "$electron" "$REPO/desktop" --remote-debugging-port="$cdp_port")
launch='echo "$DISPLAY" >"$1/app.display"; echo "${XAUTHORITY:-}" >"$1/app.xauth"; echo $$ >"$1/app.pid"; shift; exec "$@"'
if [ "$(uname)" = Darwin ]; then
	bash -c "$launch" _ "$S" "${app_cmd[@]}" </dev/null >"$D/app.log" 2>&1 &
else
	# xvfb-run -a starts at :99 and races other sessions' xvfb-run for it; a display one of
	# them takes over dies under the app.
	disp=$(for n in $(seq 140 199); do [ -e "/tmp/.X$n-lock" ] || [ -e "/tmp/.X11-unix/X$n" ] || { echo "$n"; break; }; done)
	xvfb-run -n "$disp" -s '-screen 0 1440x900x24' bash -c "$launch" _ "$S" "${app_cmd[@]}" --no-sandbox --disable-gpu \
		</dev/null >"$D/app.log" 2>&1 &
fi
xvfb=$!
t0=$(now)
for _ in $(seq 300); do
	# The app makes the socket, then sets its mode, so wait for both.
	[ -S "$SOCK" ] && [ "$(sock_mode)" = 600 ] &&
		[ "$(curl -s -o /dev/null -w '%{http_code}' --max-time 1 http://127.0.0.1:$app_port/health)" = 200 ] && break
	kill -0 "$xvfb" 2>/dev/null || break
	sleep 0.2
done
start_s=$(since "$t0")
app=$(cat "$S/app.pid" 2>/dev/null)
record "$xvfb"
[ -n "$app" ] && record "$app"
serve=$(pgrep -P "${app:-0}" -f "^$REPO/desktop/bin/reef serve")
[ -n "$serve" ] && record "$serve"
health=$(curl -s -o /dev/null -w '%{http_code}' --max-time 2 http://127.0.0.1:$app_port/health)
listener=$(lsof -tnP -iTCP:$app_port -sTCP:LISTEN 2>/dev/null | head -n 1)
mode=$(sock_mode)
{
	echo "app=${app:-none} serve=${serve:-none} listener=${listener:-none} health=$health socket_mode=${mode:-none} after_s=$start_s"
	[ -n "$app" ] && ps -o pid=,ppid=,command= -p "$app"
	[ -n "$serve" ] && ps -o pid=,ppid=,command= -p "$serve"
} >"$D/start.txt"
if [ "$health" = 200 ] && [ -n "$serve" ] && [ "$listener" = "$serve" ] && [ "$mode" = 600 ]; then
	check desktop-start 1 "/health 200 on $app_port from reef serve pid $serve, a child of the app pid $app; $SOCK mode 600 ($D/start.txt)"
else
	check desktop-start 0 "app=${app:-none} serve child=${serve:-none} $app_port listener=${listener:-none} health=$health socket mode=${mode:-none} ($D/start.txt, $D/app.log)"
	exit 1
fi

spath=$(proc_path "$serve")
resolved=$(PATH=$spath command -v claude)
if [ "${REEF_VERIFY_CLAUDE:-fake}" = real ]; then
	echo "skip desktop-fake-claude: REEF_VERIFY_CLAUDE=real; reef serve's claude is $resolved"
else
	check desktop-fake-claude "$([ "$resolved" = "$S/fake/claude" ] && echo 1)" "reef serve's PATH resolves claude to ${resolved:-nothing} ($D/start.txt)"
	echo "serve PATH=$spath" >>"$D/start.txt"
fi

ui=""
for _ in $(seq 50); do
	ui=$(cdp 'typeof window.reef + " " + location.href + " " + (document.body.innerText.length > 0)' 2>&1)
	case $ui in *"\"object http://127.0.0.1:$app_port/ true\"") break ;; esac
	sleep 0.2
done
sd=$(shot window)
check desktop-window "$(case $ui in *"\"object http://127.0.0.1:$app_port/ true\"") nonzero "$sd" && echo 1 ;; esac)" \
	"main window: $ui; screenshot std dev $sd ($D/window.png)"

# --- browser tools through reef mcp ---
# The pane starts closed and a closed pane has no size; turn the window's Browser toggle on.
cdp "(() => { const b = document.querySelector('button[aria-label=Browser]'); if (b && b.ariaPressed !== 'true') b.click(); return b?.ariaPressed ?? 'no button'; })()" >/dev/null 2>&1
sleep 0.5
out=$(mcp browser_navigate "{\"url\":\"$URL\"}")
check browser-navigate "$(case $out in *'isError=false'*"navigated to $URL"*) echo 1 ;; esac)" \
	"$(grep -m1 '^<' <<<"$out") ($E/reef-mcp.log)"

out=$(mcp browser_screenshot)
png=$(sed -n 's/.*isError=false image\/png [0-9]* bytes saved to //p' <<<"$out")
sd=0
[ -n "$png" ] && [ "$(head -c 8 "$png" | od -An -c | tr -d ' ')" = '211PNG\r\n032\n' ] &&
	sd=$(convert "$png" -format '%[fx:standard_deviation]' info: 2>/dev/null || echo 0)
check browser-screenshot "$(nonzero "$sd" && echo 1)" "$(grep -m1 '^<' <<<"$out"); std dev $sd"

mcp browser_snapshot | result_text >"$D/snapshot-1.txt"
tb=$(sed -n 's/.*- textbox "Name *" \[ref=\(e[0-9]*\)\].*/\1/p' "$D/snapshot-1.txt" | head -n 1)
btn=$(sed -n 's/.*- button "Send" \[ref=\(e[0-9]*\)\].*/\1/p' "$D/snapshot-1.txt" | head -n 1)
check browser-snapshot "$([ -n "$tb" ] && [ -n "$btn" ] && echo 1)" \
	"$(grep -h -e '- textbox' -e '- button' "$D/snapshot-1.txt" | sed 's/^ *//' | paste -sd ' ' -) ($D/snapshot-1.txt)"

typed=$(mcp browser_type "{\"ref\":\"$tb\",\"text\":\"reef-typed\"}")
clicked=$(mcp browser_click "{\"ref\":\"$btn\"}")
mcp browser_snapshot | result_text >"$D/snapshot-2.txt"
sd=$(shot window-page)
check browser-type-click "$([ -n "$tb" ] && [ -n "$btn" ] && grep -q 'StaticText "hello reef-typed"' "$D/snapshot-2.txt" &&
	case $typed$clicked in *"typed into $tb"*"clicked $btn"*) echo 1 ;; esac)" \
	"$(grep -m1 '^<' <<<"$typed"); $(grep -m1 '^<' <<<"$clicked"); then $(grep -m1 -o 'StaticText "hello[^"]*"' "$D/snapshot-2.txt" || echo 'no hello text') ($D/snapshot-2.txt, $D/window-page.png std dev $sd)"

mcp browser_console | result_text >"$D/console.txt"
check browser-console "$(grep -qx 'log: reef-page-loaded' "$D/console.txt" && grep -qx 'error: reef-page-error' "$D/console.txt" &&
	grep -qx 'log: submitted reef-typed' "$D/console.txt" && echo 1)" \
	"$(grep -e '^log: reef-page-loaded$' -e '^error: reef-page-error$' -e '^log: submitted reef-typed$' "$D/console.txt" | paste -sd ',' -) ($D/console.txt)"

mcp browser_network | result_text >"$D/network.txt"
check browser-network "$(grep -qx "GET $URL 200" "$D/network.txt" && grep -qx "GET ${URL}missing.json 404" "$D/network.txt" && echo 1)" \
	"$(paste -sd ',' "$D/network.txt") ($D/network.txt)"

on=$(cdp 'window.reef.setTakeOver(true)' 2>&1)
taken=$(mcp browser_snapshot)
off=$(cdp 'window.reef.setTakeOver(false)' 2>&1)
back=$(mcp browser_snapshot)
check browser-take-over "$(case $on$taken$off$back in true*'isError=true the user has the browser'*false*'isError=false'*) echo 1 ;; esac)" \
	"setTakeOver(true)=$on: $(grep -m1 '^<' <<<"$taken"); setTakeOver(false)=$off: $(grep -m1 '^<' <<<"$back" | cut -c1-40)... ($E/reef-mcp.log)"

mcp browser_navigate "{\"url\":\"$URL\"}" >/dev/null
out=$(mcp browser_click "{\"ref\":\"$btn\"}")
check browser-stale-ref "$([ -n "$btn" ] && case $out in *"isError=true ref $btn is not on the page; take a new snapshot"*) echo 1 ;; esac)" \
	"click $btn after a new navigation: $(grep -m1 '^<' <<<"$out") ($E/reef-mcp.log)"

# --- terminal pane on the feature's worktree ---
js_wt=$(python3 -I -c 'import json, sys; print(json.dumps(sys.argv[1]))' "$WT")
# xterm paints only while the pane is on screen, so scroll it into view before each read.
rows='(document.querySelector(".xterm")?.scrollIntoView(),
  [...document.querySelectorAll(".xterm-rows > div")].map((r) => r.textContent.trimEnd()).join("\n").trim())'
cdp "history.pushState({}, '', '/features/$fid'); dispatchEvent(new PopStateEvent('popstate')); location.pathname" >/dev/null
# Viewing a feature makes no worktree; turning the window's Terminal toggle on does.
opened=""
for _ in $(seq 25); do
	opened=$(cdp "(() => { const b = document.querySelector('button[aria-label=Terminal]'); if (!b) return 'no button'; if (b.ariaPressed === 'true') return 'clicked'; b.click(); return 'clicked'; })()" 2>&1)
	[ "$opened" = '"clicked"' ] && break
	sleep 0.2
done
shell="" before='""'
for _ in $(seq 75); do
	shell=$(ps -axo pid=,ppid=,command= | awk -v app="$app" -v sh="${SHELL:-/bin/bash}" '$2 == app && $3 == sh && $4 == "-l" { print $1 }')
	before=$(cdp "$rows" 2>/dev/null)
	[ -n "$shell" ] && [ -n "$before" ] && [ "$before" != '""' ] && break
	sleep 0.2
done
[ -n "$shell" ] && record "$shell"
cdp "window.reef.writeTerminal($js_wt, 'pwd; git branch --show-current\r')" >/dev/null
text='""'
for _ in $(seq 50); do
	text=$(cdp "$rows" 2>/dev/null)
	case $text in *"feature/term"*) break ;; esac
	sleep 0.2
done
python3 -I -c 'import json, sys; print(json.loads(sys.argv[1]))' "$text" >"$D/terminal.txt"
cwd=$([ -n "$shell" ] && proc_cwd "$shell")
sd=$(shot terminal)
# A long path wraps across rows, so match on the rows joined without line breaks.
check terminal-feature "$([ -n "$shell" ] && [ "$cwd" = "$WT" ] && ! grep -q 'feature/term' <<<"$before" &&
	tr -d '\n' <"$D/terminal.txt" | grep -qF "${WT}feature/term" && echo 1)" \
	"shell pid ${shell:-none} (child of the app) cwd ${cwd:-none}; after pwd; git branch --show-current the pane shows $(tr -d '\n' <"$D/terminal.txt" | grep -qF "${WT}feature/term" || echo 'no ')$WT then feature/term ($D/terminal.txt, $D/terminal.png std dev $sd)"

# --- quick capture: the global shortcut and the sidebar button open the same window ---
have_xdotool=$(command -v xdotool >/dev/null 2>&1 && echo 1 || echo 0)
disp=$(cat "$S/app.display")
xauth=$(cat "$S/app.xauth")
xdo() { DISPLAY="$disp" XAUTHORITY="$xauth" xdotool "$@"; }

wait_capture() { # polls for the capture window's CDP target; prints its title, or "" on timeout
	title=""
	for _ in $(seq 50); do
		title=$(cdp 'document.title' capture 2>&1)
		case $title in *"no capture window among"*) title="" ;; *) break ;; esac
		sleep 0.2
	done
	echo "$title"
}

wait_capture_closed() {
	for _ in $(seq 50); do
		case $(cdp 'document.title' capture 2>&1) in *"no capture window among"*) return 0 ;; esac
		sleep 0.2
	done
	return 1
}

if [ "$(uname)" = Darwin ]; then
	echo "skip capture-shortcut: xdotool only drives X11; on a Mac, open the app, press Cmd+Shift+Space and confirm the capture window opens (user check)"
elif [ "$have_xdotool" = 1 ]; then
	xdo key --clearmodifiers ctrl+shift+space
	title=$(wait_capture)
	sd=$([ -n "$title" ] && shot capture-shortcut || echo 0)
	check capture-shortcut "$([ -n "$title" ] && nonzero "$sd" && echo 1)" \
		"ctrl+shift+space via xdotool opened the capture window (title $title); screenshot std dev $sd ($D/capture-shortcut.png)"
	cdp 'window.close()' capture >/dev/null 2>&1
	wait_capture_closed >/dev/null
else
	echo "note capture-shortcut: xdotool not installed; proving the capture window via the sidebar button instead"
fi

cdp "(() => { const a = [...document.querySelectorAll('a')].find((a) => a.textContent.trim() === 'Capture'); a && a.click(); })()" >/dev/null
title=$(wait_capture)
sd=$([ -n "$title" ] && shot capture-button || echo 0)
check capture-button "$([ -n "$title" ] && nonzero "$sd" && echo 1)" \
	"the sidebar Capture button opened the capture window (title $title); screenshot std dev $sd ($D/capture-button.png)"

# a project chosen (set directly; the select itself is not under test here) and "bug: login fails" typed, then Enter
cdp "document.querySelector('[aria-label=\"Capture project\"]').value = '$proj';
document.querySelector('[aria-label=\"Capture project\"]').dispatchEvent(new Event('change', { bubbles: true }));
[...document.querySelectorAll('[aria-label=\"Capture kind\"] button')].find((b) => b.textContent === 'Bug').click()" capture >/dev/null
xdo type -- 'login fails'
xdo key Return
closed=$(wait_capture_closed && echo 1 || echo 0)
row=$("$H" sql "select title, feature_id from task where title = 'bug: login fails'" | tail -n 1)
check capture-enter "$([ "$closed" = 1 ] && [ "$row" = "bug: login fails|" ] && echo 1)" \
	"typed 'login fails' and Return via xdotool; window closed=$closed; task row: ${row:-none} ($E/sql.log)"

# Escape closes the window without a task
cdp "(() => { const a = [...document.querySelectorAll('a')].find((a) => a.textContent.trim() === 'Capture'); a && a.click(); })()" >/dev/null
wait_capture >/dev/null
before=$("$H" sql "select count(*) from task" | tail -n 1)
xdo key Escape
closed=$(wait_capture_closed && echo 1 || echo 0)
after=$("$H" sql "select count(*) from task" | tail -n 1)
check capture-escape "$([ "$closed" = 1 ] && [ "$before" = "$after" ] && echo 1)" \
	"Escape via xdotool closed the window=$closed; task count $before before, $after after"

# --- quit: SIGTERM is a quit request; nothing the app started may outlive it ---
descendants "$app" >"$D/procs-before-quit.txt"
kill -TERM "$app"
t0=$(now)
for _ in $(seq 150); do kill -0 "$app" 2>/dev/null || break; sleep 0.1; done
quit_s=$(since "$t0")
kill -0 "$app" 2>/dev/null || wait "$xvfb" 2>/dev/null
left=""
for p in "$app" "$xvfb" $(awk '{ print $1 }' "$D/procs-before-quit.txt"); do
	kill -0 "$p" 2>/dev/null && left="$left $p"
done
{
	echo "quit after ${quit_s} s; left alive:${left:- none}"
	[ -z "$left" ] || ps -o pid=,ppid=,command= -p "${left# }" 2>/dev/null
	ls -la "$DATA"
} >"$D/quit.txt"
check desktop-quit "$([ -z "$left" ] && [ ! -e "$SOCK" ] && echo 1)" \
	"app quit in ${quit_s} s; left alive:${left:- none} of xvfb-run, the app and its $(wc -l <"$D/procs-before-quit.txt" | tr -d ' ') descendants (Electron helpers, reef serve $serve, shell ${shell:-none}); browser.sock $([ -e "$SOCK" ] && echo still there || echo gone) ($D/quit.txt)"

out=$(mcp browser_snapshot)
check browser-unavailable "$(case $out in *'isError=true browser unavailable: the Reef app is not open'*) echo 1 ;; esac)" \
	"after quit: $(grep -m1 '^<' <<<"$out") ($E/reef-mcp.log)"

exit "$failed"
