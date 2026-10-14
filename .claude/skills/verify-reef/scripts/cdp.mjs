// node cdp.mjs PORT APP_PORT EXPRESSION: evaluate EXPRESSION (awaited) in the Reef app's main
// window over Electron's --remote-debugging-port and print its value as JSON. The browser pane is
// another page target, so the main window is picked by its http://127.0.0.1:APP_PORT URL. A hung
// or dead window fails after 10 s instead of blocking the run.
const [port, appPort, expression] = process.argv.slice(2);
setTimeout(() => {
  console.error(`no answer from the main window in 10 s: ${expression}`);
  process.exit(1);
}, 10_000).unref();

const targets = await (await fetch(`http://127.0.0.1:${port}/json/list`, { signal: AbortSignal.timeout(10_000) })).json();
const main = targets.find((t) => t.type === 'page' && t.url.startsWith(`http://127.0.0.1:${appPort}/`));
if (!main) {
  console.error(`no main window among: ${targets.map((t) => t.url).join(' ')}`);
  process.exit(1);
}

const ws = new WebSocket(main.webSocketDebuggerUrl);
await new Promise((resolve, reject) => {
  ws.onopen = resolve;
  ws.onerror = () => reject(new Error(`cannot open ${main.webSocketDebuggerUrl}`));
});
ws.send(JSON.stringify({
  id: 1,
  method: 'Runtime.evaluate',
  params: { expression, awaitPromise: true, returnByValue: true },
}));
const reply = await new Promise((resolve) => {
  ws.onmessage = (event) => {
    const msg = JSON.parse(event.data);
    if (msg.id === 1) resolve(msg);
  };
});
ws.close();

const { result, exceptionDetails } = reply.result ?? {};
if (reply.error || exceptionDetails) {
  console.error(JSON.stringify(reply.error ?? exceptionDetails.exception?.description ?? exceptionDetails.text));
  process.exit(1);
}
console.log(JSON.stringify(result.value ?? null));
