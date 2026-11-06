// node main-eval.mjs INSPECT_PORT EXPRESSION: evaluate EXPRESSION (awaited) in the Reef app's main
// process over Electron's --inspect port and print its value as JSON. `electron` is in scope as `e`,
// the main window as `w`. A hung app fails after 10 s.
const [port, expression] = process.argv.slice(2);
setTimeout(() => {
  console.error(`no answer from the main process in 10 s: ${expression}`);
  process.exit(1);
}, 10_000).unref();

const [target] = await (await fetch(`http://127.0.0.1:${port}/json/list`, { signal: AbortSignal.timeout(10_000) })).json();
const ws = new WebSocket(target.webSocketDebuggerUrl);
await new Promise((resolve, reject) => {
  ws.onopen = resolve;
  ws.onerror = () => reject(new Error(`cannot open ${target.webSocketDebuggerUrl}`));
});
const wrapped = `(() => { const e = process.mainModule.require('electron'); const w = e.BrowserWindow.getAllWindows().find((x) => x.contentView.children.length); return ${expression}; })()`;
ws.send(JSON.stringify({ id: 1, method: 'Runtime.evaluate', params: { expression: wrapped, awaitPromise: true, returnByValue: true } }));
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
