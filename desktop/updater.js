import { app, dialog, ipcMain } from 'electron';
import { execFile, spawn } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import { promisify } from 'node:util';

const RELEASE_URL = 'https://api.github.com/repos/bhanuprakaash/reef/releases/latest';
const ASSET = 'Reef-mac-arm64.zip';
const CHECK_EVERY = 60 * 60 * 1000;
const IDLE_EVERY = 60 * 1000;
const TIMEOUT = 10 * 60 * 1000;

const run = promisify(execFile);

// Both scripts run after this app has exited, in a helper that outlives it, because a running
// app cannot replace its own bundle. Arguments: pid, bundle, previous.app, then the rest.
const WAIT = String.raw`trap '' HUP TERM
while kill -0 "$1" 2>/dev/null; do sleep 0.2; done
`;
// $4 staged app, $5 pending marker, $6 and $7 old and new version, $8 failed marker, $9 "open".
const SWAP =
  WAIT +
  String.raw`rm -rf "$3"
if mv "$2" "$3"; then
  if ! { printf '%s %s\n' "$6" "$7" > "$5" && mv "$4" "$2"; }; then
    mv "$3" "$2"
    rm -f "$5"
    printf '%s\n' "$7" > "$8"
  fi
else
  printf '%s\n' "$7" > "$8"
fi
rm -rf "$(dirname "$4")"
[ "$9" = open ] && open "$2"
`;
// $4 failed marker, $5 the version that failed.
const ROLLBACK =
  WAIT +
  String.raw`rm -rf "$2" && mv "$3" "$2" && printf '%s\n' "$5" > "$4"
open "$2"
`;

function parse(version) {
  const match = /^v?(\d+)\.(\d+)\.(\d+)$/.exec(version);
  return match && match.slice(1).map(Number);
}

function isNewer(a, b) {
  for (let i = 0; i < 3; i++) if (a[i] !== b[i]) return a[i] > b[i];
  return false;
}

function read(file) {
  try {
    return fs.readFileSync(file, 'utf8').trim();
  } catch {
    return null;
  }
}

export function createUpdater(win, addr) {
  const dir = path.join(app.getPath('userData'), 'update');
  const stagedDir = path.join(dir, 'staged');
  const incoming = path.join(dir, 'incoming');
  const previous = path.join(dir, 'previous.app');
  const pending = path.join(dir, 'pending');
  const failed = path.join(dir, 'failed');
  const idleFile = path.join(app.getPath('userData'), 'update-when-idle');
  const bundle = path.resolve(process.execPath, '../../..');
  let staged = null;
  let skipped = null;
  let checking = false;
  let unwritableLogged = false;
  let swapping = false;
  let whenIdle = read(idleFile) === '1';

  function helper(script, args) {
    spawn('/bin/sh', ['-c', script, 'sh', String(process.pid), bundle, previous, ...args], {
      detached: true,
      stdio: 'ignore',
    }).unref();
  }

  function stagedApp() {
    try {
      const name = fs.readdirSync(stagedDir).find((n) => n.endsWith('.app'));
      return name && path.join(stagedDir, name);
    } catch {
      return null;
    }
  }

  function canReplaceBundle() {
    try {
      fs.accessSync(path.dirname(bundle), fs.constants.W_OK);
    } catch {
      return false;
    }
    return !bundle.includes('/AppTranslocation/');
  }

  function notify() {
    if (staged && !win.isDestroyed()) win.webContents.send('update:ready', staged);
  }

  function swap(open) {
    const stagedPath = stagedApp();
    if (!staged || !stagedPath || swapping) return false;
    swapping = true;
    helper(SWAP, [stagedPath, pending, app.getVersion(), staged, failed, open ? 'open' : '']);
    return true;
  }

  function restart() {
    if (swap(true)) app.quit();
  }

  async function stage(version, url) {
    fs.rmSync(incoming, { recursive: true, force: true });
    fs.mkdirSync(incoming, { recursive: true });
    const zip = path.join(dir, ASSET);
    await run('curl', ['-fsSL', '--max-time', String(TIMEOUT / 1000), '-o', zip, url]);
    await run('ditto', ['-x', '-k', zip, incoming]);
    await run('xattr', ['-cr', incoming]);
    fs.rmSync(zip);
    fs.writeFileSync(path.join(incoming, 'version'), version);
    if (swapping) return;
    fs.rmSync(stagedDir, { recursive: true, force: true });
    fs.renameSync(incoming, stagedDir);
    staged = version;
  }

  async function check() {
    if (checking) return;
    checking = true;
    try {
      const res = await fetch(RELEASE_URL, {
        headers: { Accept: 'application/vnd.github+json' },
        signal: AbortSignal.timeout(TIMEOUT),
      });
      if (!res.ok) throw new Error(`${RELEASE_URL}: ${res.status}`);
      const release = await res.json();
      const latest = parse(release.tag_name);
      const asset = release.assets?.find((a) => a.name === ASSET);
      if (!latest || !asset) throw new Error(`release ${release.tag_name} has no usable ${ASSET}`);
      const version = latest.join('.');
      if (!isNewer(latest, parse(app.getVersion())) || version === staged || version === skipped) return;
      if (!canReplaceBundle()) {
        if (!unwritableLogged) console.error(`update skipped: ${bundle} cannot be replaced from here`);
        unwritableLogged = true;
        return;
      }
      await stage(version, asset.browser_download_url);
      notify();
    } catch (err) {
      console.error(`update check failed: ${err.message}`);
    } finally {
      checking = false;
    }
  }

  async function applyWhenIdle() {
    if (!staged || !whenIdle) return;
    try {
      const res = await fetch(`http://${addr}/status`);
      if ((await res.json()).running === 0) restart();
    } catch (err) {
      console.error(`update idle check failed: ${err.message}`);
    }
  }

  ipcMain.handle('update:staged', () => staged);
  ipcMain.on('update:restart', restart);
  ipcMain.handle('update:get-when-idle', () => whenIdle);
  ipcMain.handle('update:set-when-idle', (_event, value) => {
    whenIdle = value === true;
    fs.mkdirSync(path.dirname(idleFile), { recursive: true });
    fs.writeFileSync(idleFile, whenIdle ? '1' : '0');
  });

  return {
    start() {
      fs.rmSync(pending, { force: true });
      const failedVersion = read(failed);
      if (failedVersion) {
        fs.rmSync(failed);
        skipped = failedVersion;
        dialog.showMessageBox(win, {
          type: 'warning',
          message: `Update to ${failedVersion} failed to start; still on ${app.getVersion()}`,
        });
      }
      if (!app.isPackaged) return;
      const version = read(path.join(stagedDir, 'version'));
      const parsed = parse(version);
      if (parsed && isNewer(parsed, parse(app.getVersion())) && stagedApp()) staged = version;
      else fs.rmSync(stagedDir, { recursive: true, force: true });
      check();
      setInterval(check, CHECK_EVERY);
      setInterval(applyWhenIdle, IDLE_EVERY);
    },
    rollBack() {
      const versions = read(pending)?.split(' ');
      if (!versions || !fs.existsSync(previous)) return false;
      fs.rmSync(pending);
      helper(ROLLBACK, [failed, versions[1]]);
      return true;
    },
    applyOnQuit() {
      swap(false);
    },
  };
}
