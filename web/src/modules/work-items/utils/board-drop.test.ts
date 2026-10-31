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
  teamId: 1,
  proposedBy: '',
  suggestedTeamId: null,
  queuePosition: null,
  tokenBudget: 1000000,
  createdAt: '2026-10-27T10:00:00Z',
  closedAt: null,
  acceptanceLines: [],
  ...over,
});

describe('dropAction', () => {
  it('moves between Backlog and Queued', () => {
    expect(dropAction(item({ status: 'backlog' }), 'queued')).toEqual({
      kind: 'move',
      status: 'queued',
    });
    expect(dropAction(item({ status: 'queued', queuePosition: 2 }), 'backlog')).toEqual({
      kind: 'move',
      status: 'backlog',
    });
  });

  it('refuses Queued for an item with no Crew', () => {
    expect(dropAction(item({ teamId: null }), 'queued')).toEqual({
      kind: 'refuse',
      reason: 'Set a Crew on this item before queuing it.',
    });
  });

  it('takes the position of the queued item it lands on, in the same Crew', () => {
    const dragged = item({ id: 1, status: 'queued', queuePosition: 1 });
    const onto = item({ id: 2, status: 'queued', queuePosition: 3 });
    expect(dropAction(dragged, 'queued', onto)).toEqual({ kind: 'reorder', position: 3 });
    expect(dropAction(dragged, 'queued', { ...onto, teamId: 2 })).toMatchObject({
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
