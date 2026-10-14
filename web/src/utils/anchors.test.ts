import { describe, expect, it } from 'vitest';
import type { InboxItem } from '@/types/api';
import { itemPath } from './anchors';

const base = { channelId: 4, featureId: null, text: '', at: '2026-10-14T08:00:00Z' };

describe('itemPath', () => {
  it('points each kind of item at its element in the channel', () => {
    const items: InboxItem[] = [
      { ...base, kind: 'approval', approvalId: 9 },
      { ...base, kind: 'failed', turnId: 12 },
      { ...base, kind: 'paused' },
      { ...base, kind: 'waiting' },
    ];

    expect(items.map(itemPath)).toEqual([
      '/channels/4#approval-9',
      '/channels/4#failed-turn-12',
      '/channels/4#paused',
      '/channels/4#latest-message',
    ]);
  });
});
