import { describe, expect, it } from 'vitest';
import type { InboxItem } from '@/types/api';
import { itemPath } from './anchors';

const base = { spaceId: 4, featureId: null, text: '', at: '2026-10-14T08:00:00Z' };

describe('itemPath', () => {
  it('points each kind of item at its element in the Space', () => {
    const items: InboxItem[] = [
      { ...base, kind: 'approval', approvalId: 9 },
      { ...base, kind: 'failed', turnId: 12 },
      { ...base, kind: 'paused' },
      { ...base, kind: 'waiting' },
    ];

    expect(items.map(itemPath)).toEqual([
      '/spaces/4#approval-9',
      '/spaces/4#failed-turn-12',
      '/spaces/4#paused',
      '/spaces/4#latest-message',
    ]);
  });
});
