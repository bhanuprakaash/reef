import { describe, expect, it } from 'vitest';
import type { Message } from '@/types/api';
import { groupThreads } from './group-threads';

const msg = (id: number, parentId: number | null, author: string): Message => ({
  id,
  channelId: 1,
  parentId,
  author,
  body: `m${id}`,
  createdAt: '',
});

describe('groupThreads', () => {
  it('collapses replies under their root in order', () => {
    const got = groupThreads([
      msg(1, null, 'user'),
      msg(2, null, 'lead'),
      msg(3, 2, 'architect'),
      msg(4, 2, 'user'),
    ]);
    expect(got.map((t) => [t.root.id, t.replies.map((r) => r.id)])).toEqual([
      [1, []],
      [2, [3, 4]],
    ]);
  });

  it('keeps a reply whose root is missing as its own top-level message', () => {
    const got = groupThreads([msg(5, 9, 'architect')]);
    expect(got.map((t) => [t.root.id, t.replies.length])).toEqual([[5, 0]]);
  });
});
