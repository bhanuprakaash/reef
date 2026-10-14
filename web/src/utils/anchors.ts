import type { InboxItem } from '@/types/api';

export const approvalAnchor = (id: number) => `approval-${id}`;
export const failedTurnAnchor = (id: number) => `failed-turn-${id}`;
export const pausedAnchor = 'paused';
export const latestMessageAnchor = 'latest-message';

function itemAnchor(item: InboxItem): string {
  if (item.kind === 'approval' && item.approvalId !== undefined)
    return approvalAnchor(item.approvalId);
  if (item.kind === 'failed' && item.turnId !== undefined) return failedTurnAnchor(item.turnId);
  return item.kind === 'paused' ? pausedAnchor : latestMessageAnchor;
}

export function itemPath(item: InboxItem): string {
  return `/channels/${item.channelId}#${itemAnchor(item)}`;
}
