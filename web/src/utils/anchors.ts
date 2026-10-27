import type { InboxItem } from '@/types/api';

export const failedTurnAnchor = (id: number) => `failed-turn-${id}`;
export const pausedAnchor = 'paused';
export const latestMessageAnchor = 'latest-message';

function itemAnchor(item: InboxItem): string {
  if (item.kind === 'failed' && item.turnId !== undefined) return failedTurnAnchor(item.turnId);
  return item.kind === 'paused' || item.kind === 'held' ? pausedAnchor : latestMessageAnchor;
}

export function itemPath(item: InboxItem): string {
  if (item.spaceId === 0) return `/work-items/${item.workItemId}`;
  return `/spaces/${item.spaceId}#${itemAnchor(item)}`;
}
