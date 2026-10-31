import { expect, it } from 'vitest';
import type { Message } from '@/types/api';
import { mergeMessages } from './get-messages';

const msg = (id: number, parentId: number | null = null, body = `m${id}`): Message => ({
  id,
  spaceId: 1,
  parentId,
  author: 'user',
  body,
  createdAt: '',
});

it('merges an earlier page and a refetched newest page by id, oldest first', () => {
  const earlier = [msg(1), msg(2), msg(3)];
  const newest = [msg(3, null, 'edited'), msg(4), msg(5)];

  expect(mergeMessages(earlier, newest).map((m) => `${m.id}:${m.body}`)).toEqual([
    '1:m1',
    '2:m2',
    '3:edited',
    '4:m4',
    '5:m5',
  ]);
});

it('holds back a reply until its root page is loaded', () => {
  expect(mergeMessages([], [msg(7, 2), msg(8)]).map((m) => m.id)).toEqual([8]);
  expect(mergeMessages([msg(2)], [msg(7, 2), msg(8)]).map((m) => m.id)).toEqual([2, 7, 8]);
});
