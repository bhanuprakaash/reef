import { app, BrowserWindow, dialog } from 'electron';
import { spawn } from 'node:child_process';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const ADDR = '127.0.0.1:7700';
const SERVER_STOP_TIMEOUT = 5000;

function reefPath() {
  return app.isPackaged
    ? path.join(process.resourcesPath, 'reef')
    : path.join(__dirname, 'bin', 'reef');
}

// The login shell's rc files set up PATH (nvm, Homebrew, ...); Electron opened
// from Finder or a launcher does not run them, so reef would not find claude
// or codex on PATH without this.
function loginShellPath() {
  return new Promise((resolve) => {
    const marker = '__REEF_PATH__';
    const shell = spawn(process.env.SHELL || '/bin/bash', ['-ilc', `echo ${marker}\${PATH}${marker}`]);
    let out = '';
    shell.stdout.on('data', (chunk) => { out += chunk; });
    shell.on('error', () => resolve(process.env.PATH));
    shell.on('close', (code) => {
      const match = out.match(new RegExp(`${marker}(.*)${marker}`, 's'));
      resolve(code === 0 && match ? match[1].trim() : process.env.PATH);
    });
  });
}

function waitForHealth(url, child) {
  return new Promise((resolve, reject) => {
    let settled = false;
    child.once('exit', (code) => {
      if (settled) return;
      settled = true;
      reject(new Error(`reef serve exited before it was healthy (code ${code})`));
    });
    const poll = async () => {
      if (settled) return;
      try {
        const res = await fetch(url);
        if (res.ok) {
          settled = true;
          resolve();
          return;
        }
      } catch {
        // not listening yet
      }
      setTimeout(poll, 100);
    };
    poll();
  });
}

let server = null;

async function startServer() {
  const PATH = await loginShellPath();
  server = spawn(reefPath(), ['serve', '--addr', ADDR], { env: { ...process.env, PATH } });
  let stderr = '';
  server.stderr.on('data', (chunk) => { stderr += chunk.toString(); });
  try {
    await waitForHealth(`http://${ADDR}/health`, server);
  } catch (err) {
    const message = stderr.trim() || err.message;
    console.error(message);
    dialog.showErrorBox('Reef could not start', message);
    app.exit(1);
    return false;
  }
  return true;
}

function stopServer() {
  if (!server || server.exitCode !== null) return Promise.resolve();
  return new Promise((resolve) => {
    const timer = setTimeout(() => server.kill('SIGKILL'), SERVER_STOP_TIMEOUT);
    server.once('exit', () => {
      clearTimeout(timer);
      resolve();
    });
    server.kill('SIGTERM');
  });
}

app.whenReady().then(async () => {
  const ok = await startServer();
  if (!ok) return;
  const win = new BrowserWindow({ width: 1200, height: 800 });
  win.loadURL(`http://${ADDR}`);
});

app.on('window-all-closed', () => app.quit());

// Electron turns a quit request (menu, Cmd+Q, window close, SIGTERM) into
// 'before-quit' without running Node's own signal handlers, so this is the
// one place that must stop the server before the app exits.
let quitting = false;
app.on('before-quit', (event) => {
  if (quitting) return;
  quitting = true;
  event.preventDefault();
  stopServer().then(() => app.quit());
});
