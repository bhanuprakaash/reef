import { describe, expect, it } from 'vitest';
import type { FileDiff } from '@/types/api';
import { diffCounts } from './diff-counts';

describe('diffCounts', () => {
  it('counts added and removed lines across files', () => {
    const files: FileDiff[] = [
      {
        path: 'a.txt',
        status: 'modified',
        hunks: [
          {
            header: '@@ -1,2 +1,3 @@',
            lines: [
              { kind: 'ctx', old: 1, new: 1, text: 'keep' },
              { kind: 'del', old: 2, text: 'old' },
              { kind: 'add', new: 2, text: 'new' },
              { kind: 'add', new: 3, text: 'more' },
            ],
          },
        ],
      },
      {
        path: 'b.txt',
        status: 'added',
        hunks: [{ header: '@@ -0,0 +1 @@', lines: [{ kind: 'add', new: 1, text: 'x' }] }],
      },
    ];
    expect(diffCounts(files)).toEqual({ added: 3, removed: 1 });
  });
});
