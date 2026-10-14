import { describe, expect, it } from 'vitest';
import type { InboxItem } from '@/types/api';
import { newItems } from './new-items';

const approval: InboxItem = {
  kind: 'approval',
  channelId: 5,
  featureId: 2,
  text: 'lead wants to wake architect',
  at: '2026-10-14T09:00:10Z',
  approvalId: 1,
};
const failed: InboxItem = {
  kind: 'failed',
  channelId: 5,
  featureId: 2,
  text: 'architect failed: interrupted',
  at: '2026-10-14T09:05:00Z',
  turnId: 7,
};

describe('newItems', () => {
  it('lists only the items the earlier list did not have', () => {
    expect(newItems([approval], [failed, approval])).toEqual([failed]);
  });

  it('lists a new approval of the same channel again', () => {
    const again = { ...approval, approvalId: 2, at: '2026-10-14T09:10:00Z' };
    expect(newItems([approval], [again])).toEqual([again]);
  });

  it('lists nothing when the list is the same', () => {
    expect(newItems([approval, failed], [failed, approval])).toEqual([]);
  });
});
