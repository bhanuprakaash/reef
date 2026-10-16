# Desktop

The Mac app is an Electron shell around `reef serve` and the [web UI](../web). The code is
[`desktop/`](../desktop), plain JavaScript ES modules, no bundler. The code is the source of
truth. This doc says what it does and why.

## Run

```
cd web && npm ci && npm run build
cd ../desktop && npm install
npm start
```

`npm start` builds `reef` with `go build -o bin/reef ../cmd/reef`, then runs Electron. In a
packaged app the `reef` binary ships inside the app's resources instead.

## What it does at start

- Reads `PATH` from the login shell (`$SHELL -ilc 'echo $PATH'`), so `claude` and `codex` are
  found even when the app is opened from Finder, not a terminal. A shell that fails to run, or
  takes more than 5 seconds (an rc file waiting on stdin), falls back to the PATH Electron
  already has.
- Starts its own `reef serve` on `127.0.0.1:7700` (the installed app) or `127.0.0.1:7701` (the
  dev copy, `npm start`) with that `PATH`, in the dev binary (`desktop/bin/reef`) or, once
  packaged, the bundled one. It loads the app only after that server logs
  `listening on 127.0.0.1:<port>`, so it never trusts another process that already holds the
  port. `REEF_PORT` sets another port; the pane refuses that port instead of 7700.
- Passes `--data` to `reef serve`: `REEF_DATA` if set, else `~/Library/Application Support/Reef`
  (the installed app) or `~/Library/Application Support/Reef Dev` (the dev copy). So the dev copy
  runs beside the installed app with its own port, data and lock, and a second app (a test run)
  can run beside either one with its own `REEF_PORT` and `REEF_DATA`.
- Opens the window at once on a local splash page ([`desktop/splash.html`](../desktop/splash.html),
  no server needed: the jelly, "reef" and "Starting reef serve..."). When the server is
  listening and the splash has shown for at least 1.2 s, the jelly and text fade out and the
  window loads `http://127.0.0.1:<port>`, which serves the same UI as a browser pointed at
  a `reef serve` started by hand (see [server.md](server.md#web-ui)). The splash, the window and
  the app share one background colour (the app's main colour for its theme), and the app fades
  in on its first paint, so the handoff has no jump in colour. The web app reports its theme
  (dark, light or system) to the main process (`theme:set`); the main process keeps it in
  `theme` in Electron's user data folder and applies it before the first window opens.
- If the data folder is already in use by another `reef serve`, the server exits with
  `data folder ... is in use by another reef` on stderr. The app shows that error in a dialog,
  then quits.
- The app trusts only the `reef serve` it spawned, never whatever else answers on the port: if
  that process exits while the app is not quitting on purpose (a port clash, a crash, any time
  before or after the window opens), the app shows a dialog with the last 64 KB of its stderr
  and quits, instead of loading or continuing to show a foreign server. The child's stderr is
  also forwarded to the app's own stderr, so it is not lost.

## Title bar

The main window has no native title bar (`titleBarStyle: 'hidden'`): the traffic lights sit in the
sidebar's top row. Dragging that row, a thin top strip or the browser pane's top row moves the
window (`app-drag` in the web app; buttons and inputs inside stay clickable). The New project
dialog's Choose button opens the native folder picker (`folder:choose`).

## Browser pane

One `WebContentsView` ([`desktop/browser.js`](../desktop/browser.js)), for the whole window,
shared by every channel. It sits to the right of the main area with Back, Forward and Reload
buttons and an address bar; a preload script reports the pane's size to the main process so the
native view stays lined up with it. The Browser toggle at the window's top right (every page)
shows or hides the pane, as does the pane's own close button; the choice is remembered and the
pane starts closed, and opens by itself while an agent drives it (the stored choice does not change). A hidden pane reports a zero size, which hides the view. Back, Forward and
Reload go over IPC (`browser:back`, `browser:forward`, `browser:reload`) to the view's navigation
history; every navigation sends the renderer the URL and whether Back and Forward have anywhere
to go, and the buttons are disabled when they do not.
The main process drives the view over the Chrome DevTools Protocol
(`webContents.debugger`), and listens on `browser.sock` in the data folder (mode 0600 from the
moment it is created) for calls from `reef mcp` (see [memory.md](memory.md#reef-mcp)): one JSON
line request (`{"tool":"navigate","args":{"url":"..."}}`), one JSON line reply (`{"text":"..."}`,
`{"image":"<base64 png>"}` or `{"error":"..."}`), per connection. `reef mcp` dials the socket
directly, not through `reef serve`. A stale socket from a crash is unlinked before the app
listens again.

`navigate` only opens `http:` and `https:` URLs, and refuses Reef's own server (host
`127.0.0.1`, `localhost` or `[::1]`, Reef's own port: 7700, or 7701 in the dev copy), with `refused: the browser pane cannot open
Reef itself`: otherwise an agent could open Reef's own UI and approve its own proposals. The same
check blocks a link or a redirect inside the page that targets Reef's origin, and denies any
`window.open`, opening the URL in the pane itself instead of a new window. The main window has
its own, simpler guard: no new windows, and navigation stays on Reef's own origin.

A `navigate` that ends with CDP `net::ERR_ABORTED` is not an error and does not wait for a load:
another navigation replaced it (a second `navigate`, a page redirect), or the response was not a
page (204, a download, a redirect to Reef's own origin that the pane blocks). It answers that it
was aborted. Real failures (DNS, refused) still fail.

`snapshot` reads the page's accessibility tree (CDP `Accessibility.getFullAXTree`) and renders
it as text, one line per element, indented to match the tree, each with a ref such as `e12`
(`- button "Submit" [ref=e12]`). The ref maps to the element's `backendDOMNodeId`, in a map kept
only for the latest snapshot: a new snapshot, or a navigation, clears it. `click` and `type` take
a ref from that map: `click` scrolls the node into view, finds its box's centre and dispatches a
mouse press and release there; `type` focuses the node and inserts the text. A ref not in the
map, or a node CDP no longer finds, fails with `ref e12 is not on the page; take a new snapshot`.
Any other failure to click or type fails with `cannot click e12: <message>` or `cannot type into
e12: <message>`.

`console` and `network` report what happened since the last navigation of the page itself: CDP
`Runtime.consoleAPICalled`, `Runtime.exceptionThrown` and `Log.entryAdded` fill the console list,
one entry per call with its level (`log`, `error`, ...); `Network.requestWillBeSent`,
`Network.responseReceived` and `Network.loadingFailed` fill the network list with each request's
method, URL and status. Both lists are cleared on `Page.frameStartedLoading` for the page's main
frame, so a new navigation starts them empty, and each is capped at the last 500 entries.

While a browser tool call (`mcp__reef__browser_*`) runs in the open channel's live turn, a banner
under the address bar shows that role's creature and `<Role> is driving · <what it is doing>`.
The banner comes from the channel's live events, not from the socket, and goes when the call
ends. A "Take over" button (in the banner, else next to the address bar) flips a boolean in the
main process. While it is on, every tool call over `browser.sock` fails with `the user has the
browser`, the banner reads "You have the browser", and the button reads "Hand back" to release it. The button's own address-bar navigation goes over IPC, not the socket,
so it still works while taken over. Reloading the main window (for example after an update) turns
take-over back off, so the button and the main process agree.

## Terminal pane

One shell per working tree ([`desktop/terminal.js`](../desktop/terminal.js)): a feature's
worktree, or a project checkout for a quick session. Neither the toggle nor viewing a page ever
makes a worktree. With the toggle on, a feature page or feature channel reads
`GET /features/{id}/worktree`: when the feature has one, the pane opens the shell there; when it
has none, the pane shows "No worktree yet" and an Open terminal button, and only that button
calls `POST /features/{id}/worktree` (making the worktree the same way a first wake would), then
opens the shell in the returned path; a failed call shows its error in the pane. A quick session
opens in the project path. The pane is keyed by feature, so moving to another feature never keeps
the previous one's shell.
The Terminal toggle sits next to the Browser toggle at the window's top right, only on a channel or
feature page (a page with a folder); one on/off choice for the window (default off, quick sessions included), remembered. On a channel
page the terminal is a drawer under the messages, on a feature page a panel pinned to the bottom,
with a header (folder, Clear, hide); the toggle or the hide button turns it off.
The pane renders nothing in a plain browser. `node-pty` spawns the login shell with the `PATH`
the app already resolved at start, sized to xterm's own columns and rows so the shell's first
output isn't wrapped to the wrong width. The shell's output is buffered in the main process per
directory, so a pane that closes and reopens on the same directory sees what it missed. A shell
that exits (the user typed `exit`, or it crashed) is dropped from that buffer, so reopening the
same directory starts a fresh one instead of replaying a dead shell's last output; otherwise it
keeps running until the app quits.

## Back and forward

The window has no browser toolbar, so detail pages (feature, review, project) start with a Back
button: it goes back one page, or to the page's parent when the window has no earlier page.
`Cmd+[` goes back and `Cmd+]` forward, except while a text field is focused.

## Quick capture

`CommandOrControl+Shift+Space`, registered as a global shortcut while the app runs, opens a
small always-on-top window on `/capture`: the same web app, no sidebar or browser pane. The
main window's sidebar has a Capture button that calls the same open through `window.reef.capture()`
over IPC, so capture also works where the global shortcut does not reach, and in tests; in a plain
browser (no `window.reef`) the button just navigates to `/capture`. The capture window has a
project picker (the project chosen last time, kept in `localStorage`, is the default), a kind
(idea, todo, bug) and a text box. Enter creates a task with no feature, titled `<kind>: <text>`,
through `POST /tasks`, and closes the window; Escape closes it without creating a task. The window also hides when it loses focus, so its transparent room never blocks the app below. It is placed on the main window that opened it, by its own width. The
shortcut is unregistered when the app quits. The window is frameless and transparent: the
rounded translucent panel on `/capture` is the window (the OS draws its shadow), and you drag it
by the panel. It is 400px tall, about 150px of panel and transparent room below for the project
listbox to open into; there is no blur (vibrancy would paint the transparent corners and room).

## Quit

Quitting the app (the window, the menu, or a signal) stops its `reef serve` first: the server
gets SIGTERM and the app waits for it to exit, about a second, before it finishes quitting. No
`reef serve` is left running after the app closes. It also closes the browser socket and
removes `browser.sock`, and kills every shell the terminal pane started.

## Updates

Only the packaged app updates itself ([`desktop/updater.js`](../desktop/updater.js)). Once `reef serve`
is listening, and then every hour, it reads `releases/latest` of the GitHub repo and compares the
tag (`v0.1.N`) with `app.getVersion()`. A newer one is downloaded (`Reef-mac-arm64.zip`) to
`update/` in the user data folder, unpacked with `ditto` into `update/incoming/`, cleared of
quarantine with `xattr -cr`, and only then moved over `update/staged/`: a failed or unfinished
download never removes an update that is already staged. Any failure is logged to stderr and
tried again at the next check. The app skips updating, with one line on stderr and no card, when
the folder that holds the app is not writable or the app runs from a Gatekeeper translocation
path (`/AppTranslocation/`): it could not replace itself there.

A staged update shows a card at the bottom of the sidebar: a dot, "Update ready", "Reef 0.1.N"
and two buttons. Restart applies the update; Later hides the card for that version until the next
launch, and a newer staged version shows it again. The web app asks the main process for the
staged version when it starts, so a reload shows the card too. Restart does nothing while no
update is staged (a download may still be running).

"Update when idle" (a toggle on the settings page, kept in `update-when-idle` in the user data
folder) applies a staged update by itself: every minute the app reads `GET /status` of its
`reef serve` and applies when `running` is 0.

Applying (Restart, idle, or quitting with an update staged) starts a detached `/bin/sh` that waits
for the app to exit, moves the running bundle to `update/previous.app` (replacing the old one: one
backup only), moves the staged app into the bundle's path, writes `update/pending` (old and new
version) and opens the bundle. Quitting applies the swap but does not open the app again.

If a move fails (the bundle cannot be moved aside, or the staged app cannot be moved into
place), the helper puts the old bundle back, writes the new version to `update/failed` and opens
the old app, which shows the same failed dialog and skips that version until it restarts.

If the new app's `reef serve` exits before it listens, or has not listened after 60 s, while
`pending` exists, the app kills the server and starts a helper that puts `previous.app` back,
writes `update/failed` and opens it, then exits; the old app shows "Update to 0.1.N failed to
start; still on 0.1.M" and does not try that version again until it restarts. When the server
does listen, `pending` is deleted.

A new app that never gets that far (its main process throws at start) cannot roll itself back.
After `open`, the swap helper waits up to 90 s for `pending` to disappear. If it stays and
`previous.app` exists, the helper kills the processes whose command line starts with the new
bundle's path and rolls back the same way. Both paths first claim the rollback with an atomic
`mv pending pending.taken`, so only one of them runs.

## Package

`electron-builder` (a `desktop/` devDependency, config in `desktop/package.json`'s `build`
field) makes an installable app. Each target cross-builds `reef` for the target's OS and arch
with `go build`, puts it in `desktop/resources/<target>/reef`, and electron-builder copies it
into the app as an extra resource. `npm install`'s `postinstall` makes `node-pty`'s darwin
prebuilt `spawn-helper` executable (it ships without the execute bit), so a Mac build does not
fail at run time with `posix_spawnp failed`; it is a no-op where there is no darwin prebuild.

### Mac

```
cd desktop
npm install
npm run dist:mac
```

Makes `dist/mac-arm64/Reef.app` (and a zipped copy) for Apple Silicon. It is ad-hoc signed
(`mac.identity: "-"`), not notarized, so first open needs a right-click, Open, Open to get past
Gatekeeper; after that it opens normally. Data folder: `~/Library/Application Support/Reef`.

### Ubuntu

```
cd desktop
npm install
npm run dist:linux
```

Makes `dist/reef-desktop_<version>_<arch>.deb` for the machine's own arch (amd64 or arm64), with
the `reef` binary built for it. It has a desktop entry (`Reef`, category Development). Build on the
arch you install on: `node-pty` is a native module, built for the machine that runs `npm install`.
Install and remove like any `.deb`:

```
sudo apt install ./dist/reef-desktop_<version>_<arch>.deb
# launch from the app grid, or:
reef-desktop
sudo apt remove reef-desktop
```

`apt remove` leaves the data folder, `~/.config/Reef`, alone.
