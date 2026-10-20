import assert from 'node:assert/strict';
import { registerHooks } from 'node:module';
import { test } from 'node:test';

registerHooks({
  resolve(specifier, context, next) {
    if (specifier === 'electron') {
      const source = 'export const clipboard = { read: async () => globalThis.clipboardItems }';
      return { url: `data:text/javascript,${encodeURIComponent(source)}`, shortCircuit: true };
    }
    return next(specifier, context);
  },
});
const { clipboardFile } = await import('./clipboard-file.js');

async function copied(uriList) {
  globalThis.clipboardItems = [
    { types: ['text/uri-list'], getType: async () => ({ text: async () => uriList }) },
  ];
  return clipboardFile();
}

test('gives the path of a copied file', async () => {
  assert.equal(await copied('file:///tmp/a%20b.txt\r\n'), '/tmp/a b.txt');
});

test('gives null for a URL that is not a local file path', async () => {
  for (const url of ['https://example.com/a.txt', 'file://host/a.txt', 'file:///a%2Fb']) {
    assert.equal(await copied(url), null, url);
  }
});

test('gives null for a text copy', async () => {
  globalThis.clipboardItems = [{ types: ['text/plain'] }];
  assert.equal(await clipboardFile(), null);
});
