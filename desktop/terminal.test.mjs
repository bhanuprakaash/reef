import assert from 'node:assert/strict';
import fs from 'node:fs';
import { registerHooks } from 'node:module';
import os from 'node:os';
import path from 'node:path';
import { test } from 'node:test';

registerHooks({
  resolve(specifier, context, next) {
    if (specifier === 'node-pty') return { url: 'data:text/javascript,export default {}', shortCircuit: true };
    return next(specifier, context);
  },
});
const { launchTerminalApp } = await import('./terminal.js');

const root = fs.mkdtempSync(path.join(os.tmpdir(), 'reef-terminal-'));
const bin = path.join(root, 'bin');
const log = path.join(root, 'open.log');
const folder = path.join(root, 'work');
fs.mkdirSync(bin);
fs.mkdirSync(folder);
fs.writeFileSync(path.join(bin, 'open'), `#!/bin/sh\necho "$@" >> ${log}\n`, { mode: 0o755 });
fs.writeFileSync(path.join(root, 'file.txt'), '');
const calls = () => (fs.existsSync(log) ? fs.readFileSync(log, 'utf8').trim().split('\n') : []);

test('opens an existing absolute folder', async () => {
  assert.equal(await launchTerminalApp('Ghostty', folder, bin), '');
  assert.deepEqual(calls(), [`-a Ghostty ${folder}`]);
});

test('rejects a cwd that open could take for a flag or that is not a folder', async () => {
  for (const cwd of ['-a', '-n', 'work', path.join(root, 'missing'), path.join(root, 'file.txt'), undefined]) {
    assert.match(await launchTerminalApp('Ghostty', cwd, bin), /is not an absolute path to an existing folder$/);
  }
  assert.equal(calls().length, 1);
});

test.after(() => fs.rmSync(root, { recursive: true, force: true }));
