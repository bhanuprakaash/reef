import pty from 'node-pty';

const SCROLLBACK = 200_000;

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
