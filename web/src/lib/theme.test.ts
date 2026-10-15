import { afterEach, describe, expect, it, vi } from 'vitest';
import { getTheme, setTheme } from './theme';

afterEach(() => vi.unstubAllGlobals());

describe('getTheme', () => {
  it('reads light and system, and falls back to dark for anything else', () => {
    for (const [stored, want] of [
      ['light', 'light'],
      ['system', 'system'],
      ['dark', 'dark'],
      ['sepia', 'dark'],
      [null, 'dark'],
    ] as const) {
      vi.stubGlobal('localStorage', { getItem: () => stored });
      expect(getTheme()).toBe(want);
    }
  });

  it('falls back to dark when storage throws', () => {
    vi.stubGlobal('localStorage', {
      getItem: () => {
        throw new Error('blocked');
      },
    });
    expect(getTheme()).toBe('dark');
  });
});

describe('setTheme', () => {
  it('sets the attribute even when storage throws', () => {
    const root = { dataset: {} as Record<string, string> };
    vi.stubGlobal('document', { documentElement: root });
    vi.stubGlobal('localStorage', {
      setItem: () => {
        throw new Error('blocked');
      },
    });
    setTheme('light');
    expect(root.dataset.theme).toBe('light');
  });
});
