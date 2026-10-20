import { execFile } from 'node:child_process';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import pty from 'node-pty';

const SCROLLBACK = 200_000;
const TERMINAL_APPS = ['Ghostty', 'iTerm', 'Terminal', 'Warp'];

// The terminal apps the settings page offers, the ones installed. Terminal itself lives in Utilities.
export function installedTerminalApps() {
  const dirs = ['/Applications', path.join(os.homedir(), 'Applications'), '/System/Applications/Utilities'];
  return TERMINAL_APPS.filter((name) => dirs.some((dir) => fs.existsSync(path.join(dir, `${name}.app`))));
}

// Opens cwd in a terminal app with `open -a`. Resolves to the error text, empty when it launched.
export function launchTerminalApp(app, cwd, PATH) {
  if (typeof cwd !== 'string' || !path.isAbsolute(cwd) || !fs.statSync(cwd, { throwIfNoEntry: false })?.isDirectory()) {
    return Promise.resolve(`${cwd} is not an absolute path to an existing folder`);
  }
  return new Promise((resolve) => {
    execFile('open', ['-a', app, cwd], { env: { ...process.env, PATH } }, (error, _stdout, stderr) => {
      resolve(error ? stderr.trim() || error.message : '');
    });
  });
}

// createTerminals makes the one shell per working tree the window opens: a feature's
// worktree, or a project checkout for a quick session. cwd is the key, so a pane that
// reopens on the same tree reuses its shell and sees the output it missed. Shells run
// until killAll, called on quit.
export function createTerminals(win, path) {
  const shells = new Map();

  function shell(cwd, cols, rows) {
    let s = shells.get(cwd);
    if (s) return s;
    const proc = pty.spawn(process.env.SHELL || '/bin/bash', ['-l'], {
      name: 'xterm-color',
      cols,
      rows,
      cwd,
      env: { ...process.env, PATH: path },
    });
    s = { proc, buffer: '' };
    proc.onData((data) => {
      s.buffer = (s.buffer + data).slice(-SCROLLBACK);
      win.webContents.send('terminal:data', cwd, data);
    });
    // Without this, exit (or a crash) leaves a dead shell in the map and the next open
    // replays its last buffer instead of starting a new one.
    proc.onExit(() => shells.delete(cwd));
    shells.set(cwd, s);
    return s;
  }

  function open(cwd, cols, rows) {
    return shell(cwd, cols, rows).buffer;
  }

  function write(cwd, data) {
    shells.get(cwd)?.proc.write(data);
  }

  function resize(cwd, cols, rows) {
    shells.get(cwd)?.proc.resize(cols, rows);
  }

  function killAll() {
    for (const { proc } of shells.values()) proc.kill();
    shells.clear();
  }

  return { open, write, resize, killAll };
}
