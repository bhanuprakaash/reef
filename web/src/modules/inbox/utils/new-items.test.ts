import { describe, expect, it } from 'vitest';
import type { InboxItem } from '@/types/api';
import { newItems } from './new-items';

const reply: InboxItem = {
  kind: 'reply',
  group: 'fyi',
  spaceId: 5,
  workItemId: 2,
  text: 'All done.',
  at: '2026-10-14T09:00:10Z',
  messageId: 1,
};
const failed: InboxItem = {
  kind: 'failed',
  group: 'needs_you',
  spaceId: 5,
  workItemId: 2,
  text: 'architect failed: interrupted',
  at: '2026-10-14T09:05:00Z',
  turnId: 7,
};

describe('newItems', () => {
  it('lists only the items the earlier list did not have', () => {
    expect(newItems([reply], [failed, reply])).toEqual([failed]);
  });

  it('lists a new reply of the same Space again', () => {
    const again = { ...reply, messageId: 2, at: '2026-10-14T09:10:00Z' };
    expect(newItems([reply], [again])).toEqual([again]);
  });

  it('lists nothing when the list is the same', () => {
    expect(newItems([reply, failed], [failed, reply])).toEqual([]);
  });
});
