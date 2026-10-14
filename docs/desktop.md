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
  found even when the app is opened from Finder, not a terminal. A shell that fails to run
  falls back to the PATH Electron already has.
- Starts its own `reef serve` on `127.0.0.1:7700` with that `PATH`, in the dev binary
  (`desktop/bin/reef`) or, once packaged, the bundled one. It waits for `GET /health` before
  it opens the window.
- Opens a window on `http://127.0.0.1:7700`, which serves the same UI as a browser pointed at
  a `reef serve` started by hand (see [server.md](server.md#web-ui)).
- If the data folder is already in use by another `reef serve`, the server exits with
  `data folder ... is in use by another reef` on stderr. The app shows that error in a dialog,
  then quits.

## Browser pane

One `WebContentsView` ([`desktop/browser.js`](../desktop/browser.js)), for the whole window,
shared by every channel. It sits to the right of the main area with an address bar; a preload
script reports the pane's size to the main process so the native view stays lined up with it.
The main process drives the view over the Chrome DevTools Protocol
(`webContents.debugger`), and listens on `browser.sock` in the data folder (mode 0600) for
calls from `reef mcp` (see [memory.md](memory.md#reef-mcp)): one JSON line request
(`{"tool":"navigate","args":{"url":"..."}}`), one JSON line reply (`{"text":"..."}`,
`{"image":"<base64 png>"}` or `{"error":"..."}`), per connection. `reef mcp` dials the socket
directly, not through `reef serve`. A stale socket from a crash is unlinked before the app
listens again.

## Quit

Quitting the app (the window, the menu, or a signal) stops its `reef serve` first: the server
gets SIGTERM and the app waits for it to exit, about a second, before it finishes quitting. No
`reef serve` is left running after the app closes. It also closes the browser socket and
removes `browser.sock`.
