import type { InboxItem } from '@/types/api';

export const kindLabels: Record<InboxItem['kind'], string> = {
  approval: 'Approval',
  paused: 'Paused',
  failed: 'Failed',
  waiting: 'Waiting',
};

export function itemKey(i: InboxItem): string {
  return [i.kind, i.channelId, i.approvalId, i.turnId, i.at].join(':');
}

export function newItems(before: InboxItem[], after: InboxItem[]): InboxItem[] {
  const seen = new Set(before.map(itemKey));
  return after.filter((i) => !seen.has(itemKey(i)));
}
