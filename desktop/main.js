import { app, BrowserWindow, dialog, globalShortcut, ipcMain, nativeTheme } from 'electron';
import { spawn } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { createBrowser } from './browser.js';
import { createTerminals } from './terminal.js';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const PORT = process.env.REEF_PORT || '7700';
const ADDR = `127.0.0.1:${PORT}`;
const SERVER_STOP_TIMEOUT = 5000;
const LOGIN_SHELL_TIMEOUT = 5000;
const STDERR_TAIL = 64 * 1024;
const SPLASH_MIN = 1200;
const SPLASH_FADE = 250;

// The same default data folder reef serve uses: os.UserConfigDir()/Reef in
// Go is app.getPath('appData') + '/Reef' in Electron on both Mac and Linux.
function dataDir() {
  return path.join(app.getPath('appData'), 'Reef');
}

// The web app's own colours, so a window is never another colour while the page loads.
const BACKGROUNDS = { dark: '#212121', light: '#ffffff' };
const THEMES = ['dark', 'light', 'system'];

function themeFile() {
  return path.join(app.getPath('userData'), 'theme');
}

// The app's theme choice lives in the page's localStorage, which the main process cannot
// read before the page loads; the page reports it (theme:set) and the main process keeps it,
// so the splash and every window start in the right colour.
function restoreTheme() {
  try {
    const theme = fs.readFileSync(themeFile(), 'utf8');
    if (THEMES.includes(theme)) return theme;
  } catch {
    // no choice kept yet
  }
  return 'dark';
}

function windowBackground() {
  return BACKGROUNDS[nativeTheme.shouldUseDarkColors ? 'dark' : 'light'];
}

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
    const shell = spawn(process.env.SHELL || '/bin/bash', ['-ilc', `echo ${marker}\${PATH}${marker}`], {
      stdio: ['ignore', 'pipe', 'ignore'],
    });
    let out = '';
    let settled = false;
    const finish = (value) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      resolve(value);
    };
    // An rc file that reads from stdin (e.g. a prompt left in .bash_profile) would hang
    // this forever without a way out but a timeout.
    const timer = setTimeout(() => {
      shell.kill();
      finish(process.env.PATH);
    }, LOGIN_SHELL_TIMEOUT);
    shell.stdout.on('data', (chunk) => { out += chunk; });
    shell.on('error', () => finish(process.env.PATH));
    shell.on('close', (code) => {
      const match = out.match(new RegExp(`${marker}(.*)${marker}`, 's'));
      finish(code === 0 && match ? match[1].trim() : process.env.PATH);
    });
  });
}

let server = null;
let quitting = false;

async function startServer(PATH) {
  server = spawn(reefPath(), ['serve', '--addr', ADDR], {
    env: { ...process.env, PATH },
    stdio: ['ignore', 'ignore', 'pipe'],
  });
  let stderr = '';
  // reef serve logs "listening on" only after it bound ADDR itself, so the window never opens
  // on another process that already answers there.
  const listening = new Promise((resolve) => {
    server.stderr.on('data', (chunk) => {
      const text = chunk.toString();
      process.stderr.write(text);
      stderr = (stderr + text).slice(-STDERR_TAIL);
      if (stderr.includes(`listening on ${ADDR}`)) resolve(true);
    });
    server.once('exit', () => resolve(false));
  });
  server.on('exit', (code) => {
    if (quitting) return;
    const message = stderr.trim() || `reef serve exited (code ${code})`;
    console.error(message);
    dialog.showErrorBox('Reef stopped unexpectedly', message);
    app.exit(1);
  });
  return listening;
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

let browser = null;
let terminals = null;
let captureWindow = null;

function openCaptureWindow() {
  if (captureWindow) {
    captureWindow.show();
    captureWindow.focus();
    return;
  }
  captureWindow = new BrowserWindow({
    width: 600,
    height: 200,
    alwaysOnTop: true,
    backgroundColor: windowBackground(),
    webPreferences: { preload: path.join(__dirname, 'preload.cjs') },
  });
  captureWindow.on('closed', () => {
    captureWindow = null;
  });
  captureWindow.loadURL(`http://${ADDR}/capture`);
}

app.whenReady().then(async () => {
  nativeTheme.themeSource = restoreTheme();
  ipcMain.on('theme:set', (_event, theme) => {
    if (!THEMES.includes(theme)) return;
    nativeTheme.themeSource = theme;
    fs.writeFileSync(themeFile(), theme);
  });
  const win = new BrowserWindow({
    width: 1200,
    height: 800,
    backgroundColor: windowBackground(),
    webPreferences: { preload: path.join(__dirname, 'preload.cjs') },
  });
  win.loadFile(path.join(__dirname, 'splash.html'));
  const splashAt = Date.now();
  const PATH = await loginShellPath();
  if (win.isDestroyed()) return;
  const ok = await startServer(PATH);
  if (!ok) return;
  win.webContents.setWindowOpenHandler(() => ({ action: 'deny' }));
  win.webContents.on('will-navigate', (event, url) => {
    if (new URL(url).origin !== `http://${ADDR}`) event.preventDefault();
  });
  browser = createBrowser(win, PORT);
  browser.listen(dataDir());
  ipcMain.on('browser:bounds', (_event, rect) => browser.setBounds(rect));
  ipcMain.handle('browser:navigate', (_event, url) => browser.navigate(url));
  ipcMain.on('browser:back', () => browser.back());
  ipcMain.on('browser:forward', () => browser.forward());
  ipcMain.on('browser:reload', () => browser.reload());
  ipcMain.handle('browser:take-over', (_event, value) => browser.setTakenOver(value));
  terminals = createTerminals(win, PATH);
  ipcMain.handle('terminal:open', (_event, cwd, cols, rows) => terminals.open(cwd, cols, rows));
  ipcMain.on('terminal:write', (_event, cwd, data) => terminals.write(cwd, data));
  ipcMain.on('terminal:resize', (_event, cwd, cols, rows) => terminals.resize(cwd, cols, rows));
  ipcMain.handle('capture:open', () => openCaptureWindow());
  globalShortcut.register('CommandOrControl+Shift+Space', openCaptureWindow);
  await new Promise((resolve) => setTimeout(resolve, Math.max(0, splashAt + SPLASH_MIN - Date.now())));
  if (win.isDestroyed()) return;
  await win.webContents.executeJavaScript("document.body.classList.add('leaving')");
  await new Promise((resolve) => setTimeout(resolve, SPLASH_FADE));
  if (win.isDestroyed()) return;
  win.loadURL(`http://${ADDR}`);
});

app.on('window-all-closed', () => app.quit());

app.on('will-quit', () => globalShortcut.unregisterAll());

// Electron turns a quit request (menu, Cmd+Q, window close, SIGTERM) into
// 'before-quit' without running Node's own signal handlers, so this is the
// one place that must stop the server before the app exits.
app.on('before-quit', (event) => {
  if (quitting) return;
  quitting = true;
  event.preventDefault();
  if (browser) browser.close();
  if (terminals) terminals.killAll();
  stopServer().then(() => app.quit());
});
