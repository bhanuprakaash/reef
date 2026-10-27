# Desktop app

A user opens the Reef app. It starts its own `reef serve` on `127.0.0.1:7700` (`REEF_PORT` here) and shows the
window. An agent drives the shared browser pane through `reef mcp`: it opens a page, takes a
screenshot, reads the page with refs, types and clicks by ref, and reads the console and network
logs. The user can take the browser over, and the agent's calls are refused until they hand it
back. A feature page has a terminal pane, a shell in the feature's worktree. Quitting the app
stops everything it started. Contract: `docs/desktop.md`.

## Sub-features

- `desktop-start` `/health` answers 200 on the app's port from a `reef serve` whose parent is the app's Electron main process (and which holds the port); `browser.sock` is in the data folder with mode 600.
- `desktop-fake-claude` the app's `reef serve` resolves `claude` to `<scratch>/fake/claude` on the `PATH` it got from the login shell, so any Space turn runs on the scripted claude. Skipped with `REEF_VERIFY_CLAUDE=real`.
- `desktop-window` the main window is on `http://127.0.0.1:<port>/` with `window.reef` defined and the UI rendered; a screenshot of the screen is not blank.
- `browser-navigate` `browser_navigate` to the test page answers `navigated to <url>`.
- `browser-screenshot` `browser_screenshot` returns a PNG of the pane that is not blank.
- `browser-snapshot` `browser_snapshot` lists `textbox "Name "` and `button "Send"` with refs.
- `browser-type-click` `browser_type` into the textbox ref and `browser_click` on the button ref answer `typed into eN` and `clicked eM`; the next snapshot shows `StaticText "hello reef-typed"`.
- `browser-console` `browser_console` lists `log: reef-page-loaded`, `error: reef-page-error` and `log: submitted reef-typed`.
- `browser-network` `browser_network` lists `GET <url> 200` and `GET <url>missing.json 404`.
- `browser-take-over` with `window.reef.setTakeOver(true)` (what Take over does) `browser_snapshot` fails with `the user has the browser`; after `setTakeOver(false)` it works again.
- `browser-stale-ref` after a new navigation, `browser_click` on a ref of the old snapshot fails with `ref eM is not on the page; take a new snapshot`.
- `browser-bounds` the native view's bounds (read from the main process over `--inspect`) equal the pane container's rect times the page zoom, within 1 px, at start, after the window is resized to 1000x700, at page zoom 1.25 (the view's bounds are window pixels, the page's are zoomed CSS pixels), and back at 1200x800.
- `terminal-feature` opening `/features/<id>` and turning on the window's Terminal toggle shows "No worktree yet" (neither viewing the page nor the toggle makes a worktree), and its Open terminal button starts a login shell, a child of the app, whose cwd is the feature's worktree `<data>/worktrees/<project id>/term`; after `pwd; git branch --show-current` the pane shows that path then `feature/term`.
- `terminal-app-settings` Settings > Terminal lists Built-in pane, then each of Ghostty, iTerm, Terminal and Warp that is installed (an `<Name>.app` in `/Applications`, `~/Applications` or `/System/Applications/Utilities`), then Custom; choosing Custom and typing `FakeTerm` stores `reef.terminalApp`.
- `terminal-app-launch` with that app chosen, the Terminal toggle on the feature page makes main run `open -a FakeTerm <worktree>` (main's `execFile` is swapped for a recorder over `--inspect`, so no real app starts) and no xterm pane opens.
- `terminal-app-reload` after a reload the choice is still stored and the toggle runs `open` again, once (each count waits 1.5 s for a late second call, so a launch run twice by React StrictMode fails it; run `desktop` with `NODE_ENV=development` for React's development build, where StrictMode is on).
- `terminal-app-no-worktree` on a second feature with no worktree the toggle shows "No worktree yet", runs no `open` and makes no worktree.
- `terminal-app-twice` with `open` answering after 1 s, the toggle turned on then off at once runs `open` once and stays off.
- `terminal-app-error` with `open` failing, the toggle shows its error in the pane; moving to another feature and back runs no `open` again and the toggle is off.
- `terminal-builtin` choosing Built-in pane clears the choice and the feature page shows the xterm pane again, with no further `open`.
- `desktop-quit` after SIGTERM to the app, xvfb-run, the app and every process it had (Electron helpers, `reef serve`, the shell) are gone, and so is `browser.sock`.
- `browser-unavailable` with the app closed, `browser_snapshot` fails with `browser unavailable: the Reef app is not open`.
- `capture-shortcut` `CommandOrControl+Shift+Space`, sent for real with `xdotool key`, opens the capture window (a screenshot of it is not blank). Skipped on a Mac (user check) or noted when `xdotool` is not installed.
- `capture-button` the sidebar's Capture link opens the same capture window (for environments and tests without the global shortcut).
- `capture-enter` with a project and kind set and `login fails` typed and Return pressed (also real `xdotool` keys), the window closes and `task` gets a row `bug: login fails` with no feature.
- `capture-escape` reopening the window and pressing Escape closes it with no new task row.

Acceptance lines of the slice's verify-reef ticket: "starts the app, screenshots the window, and stops it with no process
left" is `desktop-start`, `desktop-window`, `desktop-quit`; "drives each browser tool and the
terminal pane" is the `browser-*` ids and `terminal-feature`; "runs on the scripted claude for
any Space turn" is `desktop-fake-claude` (the run makes no Space turn). Not driven here: the
Take over button's own click and label, the address bar, a quick session's terminal, the
packaged `.deb` and the Mac app (open those by hand).

Acceptance lines of the quick capture ticket: "the shortcut opens the capture window" is
`capture-shortcut`; "typing ... and Enter makes a task row ... with no feature" is `capture-enter`;
"Escape closes the window without a task" is `capture-escape`; the sidebar button is `capture-button`.

## How to get to it (user POV)

- Open the app (`cd desktop && npm start`, the `.deb`'s `reef-desktop`, or Reef.app).
- An agent in a Space calls the `mcp__reef__browser_*` tools; the pane on the right follows.
- Take over / Hand back is the button in the banner under the address bar while an agent drives, else next to it. The Browser toggle at the window's top right shows or hides the pane, and the pane's own close button hides it; Back, Forward and Reload sit left of the address bar.
- On a Space or feature page the Terminal toggle at the window's top right shows the Terminal section (bottom of the page on a feature).
- `CommandOrControl+Shift+Space`, or the sidebar's Capture link, opens the quick capture window.
- Quit from the menu, Cmd+Q, closing the window, or a signal.

## Driving it with verify-reef

- `id=$($H build)`, `REEF_VERIFY_RUN=$id $H doctor`, then `REEF_VERIFY_RUN=$id $H desktop`. It prints one line per id above and exits 1 on any `FAIL`. Then `REEF_VERIFY_RUN=$id $H cleanup`.
- Quote each `PASS`/`FAIL` line with its file: `desktop/start.txt`, the `reef-mcp.log` lines, `desktop/snapshot-*.txt`, `console.txt`, `network.txt`, `pane-bounds.txt`, `terminal.txt`, `terminal-app-open.txt`, `quit.txt`; open `window.png`, `window-page.png` (the pane shows `hello reef-typed`), `terminal.png`, `capture-shortcut.png`/`capture-button.png` and the `mcp-browser_screenshot-*.png`.
- To see a `FAIL` is real, `kill -KILL` the app's pid (`app=` in `desktop/start.txt`) once `desktop-start` has passed: the browser lines fail with `browser unavailable`, `desktop-quit` lists the `reef serve` left alive and `browser.sock` still there. `cleanup` then kills that `reef serve`.

## Gotchas

- The run sets a profile name through `PUT /profile` and reloads the window, because the first-run page has no browser pane.
- The run sets `REEF_PORT` to a free port, so it never waits for or touches an app on 7700.
- xterm paints only while the pane is on screen, so the script scrolls it into view before reading `.xterm-rows`. A long path wraps across rows; the check joins rows without line breaks.
- The test page's console also gets Electron's `Insecure Content-Security-Policy` warning in an unpackaged app.
- `desktop` writes the scratch HOME once; a second `desktop` in the same run refuses. Build a new run.
