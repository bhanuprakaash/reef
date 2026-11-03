import type { InboxItem } from '@/types/api';

export const kindLabels: Record<InboxItem['kind'], string> = {
  paused: 'Paused',
  held: 'Held',
  failed: 'Failed',
  review: 'Review',
  question: 'Question',
  reply: 'Replied',
  sub_item: 'Sub-item',
  proposed: 'Proposed',
  started: 'Started',
  delivery: 'Delivery',
  space_proposed: 'Space proposed',
};

export function itemKey(i: InboxItem): string {
  return [i.kind, i.spaceId, i.workItemId, i.turnId, i.id, i.at].join(':');
}

export function newItems(before: InboxItem[], after: InboxItem[]): InboxItem[] {
  const seen = new Set(before.map(itemKey));
  return after.filter((i) => !seen.has(itemKey(i)));
}

export const noticeKinds: InboxItem['kind'][] = ['sub_item', 'proposed', 'started'];

export function dismissableSeen(items: InboxItem[], seen: string[]): InboxItem[] {
  return items.filter(
    (i) => i.group === 'fyi' && i.kind !== 'space_proposed' && seen.includes(itemKey(i)),
  );
}
