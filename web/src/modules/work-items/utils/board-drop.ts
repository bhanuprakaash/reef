import type { WorkItem, WorkItemStatus } from '@/types/api';

export const COLUMNS: { status: WorkItemStatus; label: string }[] = [
  { status: 'backlog', label: 'Backlog' },
  { status: 'queued', label: 'Queued' },
  { status: 'in_progress', label: 'In progress' },
  { status: 'needs_you', label: 'Needs you' },
  { status: 'review', label: 'Review' },
  { status: 'done', label: 'Done' },
  { status: 'dropped', label: 'Dropped' },
];

export type Drop =
  | { kind: 'move'; status: WorkItemStatus }
  | { kind: 'reorder'; position: number }
  | { kind: 'refuse'; reason: string }
  | null;

export function dropAction(dragged: WorkItem, column: WorkItemStatus, onto?: WorkItem): Drop {
  if (column === dragged.status) {
    if (column !== 'queued' || !onto || onto.id === dragged.id) return null;
    if (onto.queuePosition === null) {
      return { kind: 'refuse', reason: 'An item moves only within the queue.' };
    }
    return { kind: 'reorder', position: onto.queuePosition };
  }
  if (column === 'queued' && dragged.status === 'backlog') {
    return { kind: 'refuse', reason: 'Open the item and approve it to queue it.' };
  }
  if (column === 'backlog' && dragged.status === 'queued') return { kind: 'move', status: column };
  return {
    kind: 'refuse',
    reason:
      'Only a Queued item takes a drag, back to Backlog. An item reaches the other columns through its own actions.',
  };
}
