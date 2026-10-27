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
    if (onto.teamId === null || onto.teamId !== dragged.teamId || onto.queuePosition === null) {
      return { kind: 'refuse', reason: 'An item moves only within the queue of its own Team.' };
    }
    return { kind: 'reorder', position: onto.queuePosition };
  }
  const between = ['backlog', 'queued'];
  if (between.includes(column) && between.includes(dragged.status)) {
    return { kind: 'move', status: column };
  }
  return {
    kind: 'refuse',
    reason:
      'Only Backlog and Queued take a drag. An item reaches the other columns through its own actions.',
  };
}
