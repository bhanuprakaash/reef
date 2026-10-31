import { describe, expect, it } from 'vitest';
import type { WorkItem } from '@/types/api';
import { dropAction } from './board-drop';

const item = (over: Partial<WorkItem>): WorkItem => ({
  id: 1,
  projectId: 1,
  parentId: null,
  type: 'feature',
  title: 'item',
  description: '',
  status: 'backlog',
  proposedBy: '',
  queuePosition: null,
  tokenBudget: 1000000,
  createdAt: '2026-10-27T10:00:00Z',
  closedAt: null,
  approvedAt: null,
  approvalKey: null,
  acceptanceLines: [],
  ownerBotId: 1,
  participantBotIds: [1],
  targets: [],
  ...over,
});

describe('dropAction', () => {
  it('moves a queued item back to Backlog and refuses to queue a Backlog one', () => {
    expect(dropAction(item({ status: 'queued', queuePosition: 2 }), 'backlog')).toEqual({
      kind: 'move',
      status: 'backlog',
    });
    expect(dropAction(item({ status: 'backlog' }), 'queued')).toEqual({
      kind: 'refuse',
      reason: 'Open the item and approve it to queue it.',
    });
  });

  it('takes the position of the queued item it lands on', () => {
    const dragged = item({ id: 1, status: 'queued', queuePosition: 1 });
    const onto = item({ id: 2, status: 'queued', queuePosition: 3 });
    expect(dropAction(dragged, 'queued', onto)).toEqual({ kind: 'reorder', position: 3 });
    expect(dropAction(dragged, 'queued', { ...onto, queuePosition: null })).toMatchObject({
      kind: 'refuse',
    });
    expect(dropAction(dragged, 'queued')).toBeNull();
  });

  it('refuses every other column', () => {
    for (const column of ['in_progress', 'needs_you', 'review', 'done', 'dropped'] as const) {
      expect(dropAction(item({ status: 'backlog' }), column)).toMatchObject({ kind: 'refuse' });
    }
    expect(dropAction(item({ status: 'review' }), 'queued')).toMatchObject({ kind: 'refuse' });
  });
});
