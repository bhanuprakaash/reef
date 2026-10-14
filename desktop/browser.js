import { WebContentsView } from 'electron';
import fs from 'node:fs';
import net from 'node:net';
import path from 'node:path';

const SOCKET_NAME = 'browser.sock';
const LOAD_TIMEOUT = 5000;

// createBrowser makes the one browser pane for the window: a WebContentsView
// driven over the Chrome DevTools Protocol, and a socket server that `reef
// mcp` dials to run tools against it. handlers maps a tool name from the
// socket wire to a function of its args; later tickets add entries here.
export function createBrowser(win) {
  const view = new WebContentsView();
  win.contentView.addChildView(view);
  view.webContents.loadURL('about:blank');
  view.webContents.debugger.attach('1.3');
  view.webContents.debugger.sendCommand('Page.enable');
  view.webContents.on('did-navigate', (_event, url) => {
    win.webContents.send('browser:navigated', url);
  });

  async function navigate(url) {
    const loaded = new Promise((resolve) => {
      const onMessage = (_event, method) => {
        if (method !== 'Page.loadEventFired') return;
        view.webContents.debugger.removeListener('message', onMessage);
        clearTimeout(timer);
        resolve();
      };
      const timer = setTimeout(() => {
        view.webContents.debugger.removeListener('message', onMessage);
        resolve();
      }, LOAD_TIMEOUT);
      view.webContents.debugger.on('message', onMessage);
    });
    await view.webContents.debugger.sendCommand('Page.navigate', { url });
    await loaded;
    return { text: `navigated to ${url}` };
  }

  async function screenshot() {
    const { data } = await view.webContents.debugger.sendCommand('Page.captureScreenshot', {
      format: 'png',
    });
    return { image: data };
  }

  const handlers = { navigate: (args) => navigate(args.url), screenshot };

  let server = null;
  let socketPath = null;

  async function handleLine(line, conn) {
    let reply;
    try {
      const { tool, args } = JSON.parse(line);
      const handler = handlers[tool];
      reply = handler ? await handler(args ?? {}) : { error: `unknown tool "${tool}"` };
    } catch (err) {
      reply = { error: err.message };
    }
    conn.end(JSON.stringify(reply) + '\n');
  }

  function listen(dataDir) {
    socketPath = path.join(dataDir, SOCKET_NAME);
    fs.rmSync(socketPath, { force: true });
    server = net.createServer((conn) => {
      let buf = '';
      conn.on('data', (chunk) => {
        buf += chunk;
        const nl = buf.indexOf('\n');
        if (nl === -1) return;
        handleLine(buf.slice(0, nl), conn);
      });
    });
    server.listen(socketPath, () => fs.chmodSync(socketPath, 0o600));
  }

  function close() {
    if (server) server.close();
    if (socketPath) fs.rmSync(socketPath, { force: true });
  }

  function setBounds(rect) {
    view.setBounds({
      x: Math.round(rect.x),
      y: Math.round(rect.y),
      width: Math.round(rect.width),
      height: Math.round(rect.height),
    });
  }

  return { listen, close, setBounds, navigate, screenshot };
}
