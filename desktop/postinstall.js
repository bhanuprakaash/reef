import { chmodSync, readdirSync } from 'node:fs';
import path from 'node:path';

// node-pty's darwin prebuilds ship spawn-helper without the execute bit, so pty.spawn fails
// with "posix_spawnp failed" on a fresh install. There is nothing to fix on other platforms.
const prebuilds = path.join('node_modules', 'node-pty', 'prebuilds');
let entries = [];
try {
  entries = readdirSync(prebuilds, { withFileTypes: true });
} catch {
  entries = [];
}
for (const entry of entries) {
  if (!entry.isDirectory() || !entry.name.startsWith('darwin-')) continue;
  try {
    chmodSync(path.join(prebuilds, entry.name, 'spawn-helper'), 0o755);
  } catch {
    // no spawn-helper in this prebuild
  }
}
