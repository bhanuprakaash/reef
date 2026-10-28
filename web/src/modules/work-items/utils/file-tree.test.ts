import { describe, expect, it } from 'vitest';
import type { FileDiff } from '@/types/api';
import { buildFileTree } from './file-tree';

const file = (path: string): FileDiff => ({ path, status: 'modified', hunks: [] });

describe('buildFileTree', () => {
  it('nests files under folders, folders before files, each sorted by name', () => {
    const tree = buildFileTree([
      file('z.md'),
      file('web/src/b.ts'),
      file('web/src/a.ts'),
      file('web/src/lib/x.ts'),
      file('web/src/modules/y.ts'),
      file('README.md'),
    ]);
    expect(tree).toEqual([
      {
        kind: 'dir',
        path: 'web/src',
        name: 'web/src',
        children: [
          {
            kind: 'dir',
            path: 'web/src/lib',
            name: 'lib',
            children: [{ kind: 'file', name: 'x.ts', file: file('web/src/lib/x.ts') }],
          },
          {
            kind: 'dir',
            path: 'web/src/modules',
            name: 'modules',
            children: [{ kind: 'file', name: 'y.ts', file: file('web/src/modules/y.ts') }],
          },
          { kind: 'file', name: 'a.ts', file: file('web/src/a.ts') },
          { kind: 'file', name: 'b.ts', file: file('web/src/b.ts') },
        ],
      },
      { kind: 'file', name: 'README.md', file: file('README.md') },
      { kind: 'file', name: 'z.md', file: file('z.md') },
    ]);
  });

  it('merges a chain of single-folder parents into one row', () => {
    expect(buildFileTree([file('a/b/c/d.ts')])).toEqual([
      {
        kind: 'dir',
        path: 'a/b/c',
        name: 'a/b/c',
        children: [{ kind: 'file', name: 'd.ts', file: file('a/b/c/d.ts') }],
      },
    ]);
  });
});
