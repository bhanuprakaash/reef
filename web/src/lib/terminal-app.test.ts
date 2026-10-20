import { afterEach, describe, expect, it, vi } from 'vitest';
import { getTerminalApp, setTerminalApp } from './terminal-app';

afterEach(() => vi.unstubAllGlobals());

function stubStorage() {
  const items = new Map<string, string>();
  vi.stubGlobal('localStorage', {
    getItem: (k: string) => items.get(k) ?? null,
    setItem: (k: string, v: string) => items.set(k, v),
    removeItem: (k: string) => items.delete(k),
  });
}

describe('terminal app', () => {
  it('is the built-in pane until an app is chosen, and back after choosing it again', () => {
    stubStorage();
    expect(getTerminalApp()).toBeNull();
    setTerminalApp('Ghostty');
    expect(getTerminalApp()).toBe('Ghostty');
    setTerminalApp(null);
    expect(getTerminalApp()).toBeNull();
  });

  it('is the built-in pane when storage throws', () => {
    vi.stubGlobal('localStorage', {
      getItem: () => {
        throw new Error('blocked');
      },
    });
    expect(getTerminalApp()).toBeNull();
  });
});
