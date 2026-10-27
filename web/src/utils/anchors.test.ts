import { describe, expect, it } from 'vitest';
import type { InboxItem } from '@/types/api';
import { itemPath } from './anchors';

const base = { spaceId: 4, workItemId: null, text: '', at: '2026-10-14T08:00:00Z' };

describe('itemPath', () => {
  it('points each kind of item at its element in the Space', () => {
    const items: InboxItem[] = [
      { ...base, kind: 'failed', group: 'needs_you', turnId: 12 },
      { ...base, kind: 'paused', group: 'needs_you' },
      { ...base, kind: 'held', group: 'needs_you' },
      { ...base, kind: 'waiting', group: 'fyi' },
    ];

    expect(items.map(itemPath)).toEqual([
      '/spaces/4#failed-turn-12',
      '/spaces/4#paused',
      '/spaces/4#paused',
      '/spaces/4#latest-message',
    ]);
  });

  it('points an item of a Work item at its page', () => {
    const review: InboxItem = {
      ...base,
      kind: 'review',
      group: 'review',
      spaceId: 0,
      workItemId: 7,
    };

    expect(itemPath(review)).toBe('/features/7');
  });
});
