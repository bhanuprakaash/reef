import { describe, expect, it, vi } from 'vitest';
import { insertFiles, withPaths } from './file-path';

describe('withPaths', () => {
  it('asks the desktop bridge for each file path', () => {
    const pathForFile = vi.fn((file: File) => `/Users/ada/${file.name}`);
    const files = [new File([''], 'a.png'), new File([''], 'b.txt')];
    expect(withPaths(files, { pathForFile })).toEqual([
      { name: 'a.png', path: '/Users/ada/a.png' },
      { name: 'b.txt', path: '/Users/ada/b.txt' },
    ]);
  });

  it('has no path without the bridge', () => {
    expect(withPaths([new File([''], 'a.png')], undefined)).toEqual([{ name: 'a.png', path: '' }]);
  });

  it('has no path for a file that is not on disk', () => {
    const files = [new File([''], 'image.png')];
    expect(withPaths(files, { pathForFile: () => '' })).toEqual([{ name: 'image.png', path: '' }]);
  });
});

describe('insertFiles', () => {
  it('puts the path at the caret and moves the caret after it', () => {
    const next = insertFiles('look at  now', 8, [{ name: 'a.png', path: '/Users/ada/a.png' }]);
    expect(next).toEqual({ text: 'look at /Users/ada/a.png  now', caret: 25, nameOnly: false });
  });

  it('starts a new word after text and quotes a path with spaces', () => {
    const next = insertFiles('see', 3, [{ name: 'my shot.png', path: '/Users/ada/my shot.png' }]);
    expect(next.text).toBe('see "/Users/ada/my shot.png" ');
  });

  it('falls back to the name and says so when there is no path', () => {
    const next = insertFiles('', 0, [
      { name: 'a.png', path: '/Users/ada/a.png' },
      { name: 'b.png', path: '' },
    ]);
    expect(next).toEqual({ text: '/Users/ada/a.png b.png ', caret: 23, nameOnly: true });
  });
});
