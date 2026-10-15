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
- Starts its own `reef serve` on `127.0.0.1:7700` with that `PATH`, in the dev binary
  (`desktop/bin/reef`) or, once packaged, the bundled one. It opens the window only after
  that server logs `listening on 127.0.0.1:7700`, so it never trusts another process that
  already holds the port. `REEF_PORT` sets another port, so a second app (a test run) can run
  beside the first with its own `HOME`; the pane refuses that port instead of 7700.
- Opens a window on `http://127.0.0.1:7700`, which serves the same UI as a browser pointed at
  a `reef serve` started by hand (see [server.md](server.md#web-ui)).
- If the data folder is already in use by another `reef serve`, the server exits with
  `data folder ... is in use by another reef` on stderr. The app shows that error in a dialog,
  then quits.
- The app trusts only the `reef serve` it spawned, never whatever else answers on the port: if
  that process exits while the app is not quitting on purpose (a port clash, a crash, any time
  before or after the window opens), the app shows a dialog with the last 64 KB of its stderr
  and quits, instead of loading or continuing to show a foreign server. The child's stderr is
  also forwarded to the app's own stderr, so it is not lost.

## Browser pane

One `WebContentsView` ([`desktop/browser.js`](../desktop/browser.js)), for the whole window,
shared by every channel. It sits to the right of the main area with an address bar; a preload
script reports the pane's size to the main process so the native view stays lined up with it.
The main process drives the view over the Chrome DevTools Protocol
(`webContents.debugger`), and listens on `browser.sock` in the data folder (mode 0600 from the
moment it is created) for calls from `reef mcp` (see [memory.md](memory.md#reef-mcp)): one JSON
line request (`{"tool":"navigate","args":{"url":"..."}}`), one JSON line reply (`{"text":"..."}`,
`{"image":"<base64 png>"}` or `{"error":"..."}`), per connection. `reef mcp` dials the socket
directly, not through `reef serve`. A stale socket from a crash is unlinked before the app
listens again.

`navigate` only opens `http:` and `https:` URLs, and refuses Reef's own server (host
`127.0.0.1`, `localhost` or `[::1]`, port 7700), with `refused: the browser pane cannot open
Reef itself`: otherwise an agent could open Reef's own UI and approve its own proposals. The same
check blocks a link or a redirect inside the page that targets Reef's origin, and denies any
`window.open`, opening the URL in the pane itself instead of a new window. The main window has
its own, simpler guard: no new windows, and navigation stays on Reef's own origin.

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

A "Take over" button next to the address bar flips a boolean in the main process. While it is on,
every tool call over `browser.sock` fails with `the user has the browser`, and the button reads
"Hand back" to release it. The button's own address-bar navigation goes over IPC, not the socket,
so it still works while taken over. Reloading the main window (for example after an update) turns
take-over back off, so the button and the main process agree.

## Terminal pane

One shell per working tree ([`desktop/terminal.js`](../desktop/terminal.js)): a feature's
worktree, or a project checkout for a quick session. A quick session's pane shows on its channel
page right away, in the project path. A feature's worktree is not made just by viewing the
feature page: an "Open terminal" button calls `POST /features/{id}/worktree` (making the
worktree the same way a first wake would, or reusing it), then shows the pane in the returned
path; a failed call shows its error under the button. The button asks again on every visit to
the feature page in this window session; it does not remember that a tree was already opened.
The pane renders nothing in a plain browser. `node-pty` spawns the login shell with the `PATH`
the app already resolved at start, sized to xterm's own columns and rows so the shell's first
output isn't wrapped to the wrong width. The shell's output is buffered in the main process per
directory, so a pane that closes and reopens on the same directory sees what it missed. A shell
that exits (the user typed `exit`, or it crashed) is dropped from that buffer, so reopening the
same directory starts a fresh one instead of replaying a dead shell's last output; otherwise it
keeps running until the app quits.

## Quick capture

`CommandOrControl+Shift+Space`, registered as a global shortcut while the app runs, opens a
small always-on-top window on `/capture`: the same web app, no sidebar or browser pane. The
main window's sidebar has a Capture button that calls the same open through `window.reef.capture()`
over IPC, so capture also works where the global shortcut does not reach, and in tests; in a plain
browser (no `window.reef`) the button just navigates to `/capture`. The capture window has a
project picker (the project chosen last time, kept in `localStorage`, is the default), a kind
(idea, todo, bug) and a text box. Enter creates a task with no feature, titled `<kind>: <text>`,
through `POST /tasks`, and closes the window; Escape closes it without creating a task. The
shortcut is unregistered when the app quits.

## Quit

Quitting the app (the window, the menu, or a signal) stops its `reef serve` first: the server
gets SIGTERM and the app waits for it to exit, about a second, before it finishes quitting. No
`reef serve` is left running after the app closes. It also closes the browser socket and
removes `browser.sock`, and kills every shell the terminal pane started.

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
