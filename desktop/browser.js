import { WebContentsView } from 'electron';
import fs from 'node:fs';
import net from 'node:net';
import path from 'node:path';

const SOCKET_NAME = 'browser.sock';
const LOAD_TIMEOUT = 5000;
const MAX_ENTRIES = 500;
const REEF_HOSTS = new Set(['127.0.0.1', 'localhost', '[::1]']);

// refError is returned for a ref that is not in the latest snapshot's map,
// or a backendDOMNodeId that CDP no longer recognises (the node is gone).
function refError(ref) {
  return new Error(`ref ${ref} is not on the page; take a new snapshot`);
}

// isMissingNodeError matches CDP's error for a backendDOMNodeId it no longer
// recognises, so only that case maps to refError; any other failure passes
// through with its own message.
function isMissingNodeError(err) {
  return /no node (with given id )?found|could not find node/i.test(err.message);
}

// checkOrigin throws for anything the pane must not open: a non-http(s)
// scheme, or Reef's own server, which would let an agent approve its own
// proposals.
function checkOrigin(raw, reefPort) {
  let url;
  try {
    url = new URL(raw);
  } catch {
    throw new Error(`refused: ${raw} is not a valid URL`);
  }
  if (url.protocol !== 'http:' && url.protocol !== 'https:') {
    throw new Error(`refused: ${url.protocol} is not allowed; only http and https`);
  }
  if (REEF_HOSTS.has(url.hostname) && url.port === reefPort) {
    throw new Error('refused: the browser pane cannot open Reef itself');
  }
}

function isAllowed(url, reefPort) {
  try {
    checkOrigin(url, reefPort);
    return true;
  } catch {
    return false;
  }
}

function pushCapped(list, item) {
  list.push(item);
  if (list.length > MAX_ENTRIES) list.shift();
}

// createBrowser makes the one browser pane for the window: a WebContentsView
// driven over the Chrome DevTools Protocol, and a socket server that `reef
// mcp` dials to run tools against it. handlers maps a tool name from the
// socket wire to a function of its args.
export function createBrowser(win, reefPort) {
  const view = new WebContentsView();
  win.contentView.addChildView(view);
  view.webContents.loadURL('about:blank');
  view.webContents.debugger.attach('1.3');
  view.webContents.debugger.sendCommand('Page.enable');
  view.webContents.debugger.sendCommand('DOM.enable');
  view.webContents.debugger.sendCommand('Accessibility.enable');
  view.webContents.debugger.sendCommand('Runtime.enable');
  view.webContents.debugger.sendCommand('Network.enable');
  view.webContents.debugger.sendCommand('Log.enable');

  // mainFrameId identifies the top frame, so console/network logging only
  // resets on a navigation of the page itself, not an iframe inside it.
  let mainFrameId = null;
  view.webContents.debugger.sendCommand('Page.getFrameTree')
    .then(({ frameTree }) => { mainFrameId = frameTree.frame.id; });

  // consoleEntries and networkRequests hold what happened since the last
  // main-frame navigation; a new one clears both before its own requests
  // and logs arrive.
  let consoleEntries = [];
  let networkRequests = [];

  function argText(arg) {
    return arg.value !== undefined ? String(arg.value) : (arg.description ?? '');
  }

  function findRequest(requestId) {
    return networkRequests.find((r) => r.requestId === requestId);
  }

  view.webContents.debugger.on('message', (_event, method, params) => {
    switch (method) {
      case 'Page.frameStartedLoading':
        if (params.frameId === mainFrameId) {
          consoleEntries = [];
          networkRequests = [];
        }
        break;
      case 'Runtime.consoleAPICalled':
        pushCapped(consoleEntries, { level: params.type, text: params.args.map(argText).join(' ') });
        break;
      case 'Runtime.exceptionThrown':
        pushCapped(consoleEntries, {
          level: 'error',
          text: params.exceptionDetails.exception?.description ?? params.exceptionDetails.text,
        });
        break;
      case 'Log.entryAdded':
        pushCapped(consoleEntries, { level: params.entry.level, text: params.entry.text });
        break;
      case 'Network.requestWillBeSent':
        pushCapped(networkRequests, {
          requestId: params.requestId,
          method: params.request.method,
          url: params.request.url,
          status: null,
        });
        break;
      case 'Network.responseReceived': {
        const req = findRequest(params.requestId);
        if (req) req.status = params.response.status;
        break;
      }
      case 'Network.loadingFailed': {
        const req = findRequest(params.requestId);
        if (req) req.status = 'failed';
        break;
      }
      default:
        break;
    }
  });

  // refs maps a snapshot ref (e1, e2, ...) to the backendDOMNodeId it named.
  // It is only valid for the snapshot that made it: cleared on each new
  // snapshot and on navigation, so a stale ref errors instead of acting on
  // the wrong node.
  let refs = new Map();
  view.webContents.on('did-navigate', (_event, url) => {
    refs.clear();
    const history = view.webContents.navigationHistory;
    win.webContents.send('browser:navigated', {
      url,
      canGoBack: history.canGoBack(),
      canGoForward: history.canGoForward(),
    });
  });
  // A page can carry an agent to Reef's own origin by a link or a redirect,
  // not only a navigate call, so those paths need the same check.
  view.webContents.on('will-navigate', (event, url) => {
    if (!isAllowed(url, reefPort)) event.preventDefault();
  });
  view.webContents.on('will-redirect', (event, url) => {
    if (!isAllowed(url, reefPort)) event.preventDefault();
  });
  view.webContents.setWindowOpenHandler(({ url }) => {
    navigate(url).catch(() => {});
    return { action: 'deny' };
  });

  async function navigate(url) {
    checkOrigin(url, reefPort);
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
    const { errorText } = await view.webContents.debugger.sendCommand('Page.navigate', { url });
    if (errorText) throw new Error(errorText);
    await loaded;
    return { text: `navigated to ${url}` };
  }

  async function screenshot() {
    const { data } = await view.webContents.debugger.sendCommand('Page.captureScreenshot', {
      format: 'png',
    });
    return { image: data };
  }

  // snapshot reads the page's accessibility tree and renders it as one line
  // per node, indented to match the tree, with a ref for every node that
  // has a backendDOMNodeId: click and type use that ref to find the node.
  // Ignored nodes, and empty generic nodes (plain layout wrappers), are left
  // out of the text but their children are still visited, at the same depth.
  async function snapshot() {
    const { nodes } = await view.webContents.debugger.sendCommand('Accessibility.getFullAXTree');
    const byID = new Map(nodes.map((n) => [n.nodeId, n]));
    const hasParent = new Set(nodes.flatMap((n) => n.childIds ?? []));
    const root = nodes.find((n) => !hasParent.has(n.nodeId)) ?? nodes[0];

    refs = new Map();
    let next = 1;
    const lines = [];

    function visit(node, depth) {
      if (!node) return;
      const role = node.role?.value ?? '';
      const name = node.name?.value ?? '';
      const skip = node.ignored || (role === 'generic' && name === '');
      let childDepth = depth;
      if (!skip && node.backendDOMNodeId != null) {
        const ref = `e${next++}`;
        refs.set(ref, node.backendDOMNodeId);
        lines.push(`${'  '.repeat(depth)}- ${role} "${name}" [ref=${ref}]`);
        childDepth = depth + 1;
      }
      for (const id of node.childIds ?? []) visit(byID.get(id), childDepth);
    }
    visit(root, 0);

    return { text: lines.join('\n') };
  }

  function backendNodeFor(ref) {
    const backendNodeId = refs.get(ref);
    if (backendNodeId == null) throw refError(ref);
    return backendNodeId;
  }

  async function click(ref) {
    const backendNodeId = backendNodeFor(ref);
    try {
      await view.webContents.debugger.sendCommand('DOM.scrollIntoViewIfNeeded', { backendNodeId });
      const { model } = await view.webContents.debugger.sendCommand('DOM.getBoxModel', {
        backendNodeId,
      });
      const [x1, y1, x2, y2, x3, y3, x4, y4] = model.content;
      const x = (x1 + x2 + x3 + x4) / 4;
      const y = (y1 + y2 + y3 + y4) / 4;
      await view.webContents.debugger.sendCommand('Input.dispatchMouseEvent', {
        type: 'mousePressed', x, y, button: 'left', clickCount: 1,
      });
      await view.webContents.debugger.sendCommand('Input.dispatchMouseEvent', {
        type: 'mouseReleased', x, y, button: 'left', clickCount: 1,
      });
    } catch (err) {
      if (isMissingNodeError(err)) throw refError(ref);
      throw new Error(`cannot click ${ref}: ${err.message}`);
    }
    return { text: `clicked ${ref}` };
  }

  async function type(ref, text) {
    const backendNodeId = backendNodeFor(ref);
    try {
      await view.webContents.debugger.sendCommand('DOM.focus', { backendNodeId });
      await view.webContents.debugger.sendCommand('Input.insertText', { text });
    } catch (err) {
      if (isMissingNodeError(err)) throw refError(ref);
      throw new Error(`cannot type into ${ref}: ${err.message}`);
    }
    return { text: `typed into ${ref}` };
  }

  function consoleLog() {
    return { text: consoleEntries.map((e) => `${e.level}: ${e.text}`).join('\n') };
  }

  function network() {
    return {
      text: networkRequests
        .map((r) => `${r.method} ${r.url}${r.status != null ? ` ${r.status}` : ''}`)
        .join('\n'),
    };
  }

  const handlers = {
    navigate: (args) => navigate(args.url),
    screenshot,
    snapshot,
    click: (args) => click(args.ref),
    type: (args) => type(args.ref, args.text),
    console: consoleLog,
    network,
  };

  let server = null;
  let socketPath = null;
  let takenOver = false;

  function setTakenOver(value) {
    takenOver = value;
    return takenOver;
  }

  // The renderer's own take-over state resets on reload along with the rest
  // of its state; this keeps the main process in step with it.
  win.webContents.on('did-finish-load', () => setTakenOver(false));

  async function handleLine(line, conn) {
    let reply;
    try {
      const { tool, args } = JSON.parse(line);
      if (takenOver) {
        reply = { error: 'the user has the browser' };
      } else {
        const handler = handlers[tool];
        reply = handler ? await handler(args ?? {}) : { error: `unknown tool "${tool}"` };
      }
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
    // The umask narrows the window between the socket file's creation and
    // the chmod below, so it is never briefly open to the rest of the group
    // or other users.
    const prevUmask = process.umask(0o077);
    server.listen(socketPath, () => {
      process.umask(prevUmask);
      fs.chmodSync(socketPath, 0o600);
    });
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

  const back = () => view.webContents.navigationHistory.goBack();
  const forward = () => view.webContents.navigationHistory.goForward();
  const reload = () => view.webContents.reload();

  return { listen, close, setBounds, navigate, screenshot, setTakenOver, back, forward, reload };
}
