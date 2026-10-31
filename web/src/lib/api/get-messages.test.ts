import { QueryClient } from '@tanstack/react-query';
import { afterEach, expect, it, vi } from 'vitest';
import type { Message } from '@/types/api';
import { mergeMessages, messagesQuery } from './get-messages';

const msg = (id: number, parentId: number | null = null, body = `m${id}`): Message => ({
  id,
  spaceId: 1,
  parentId,
  author: 'user',
  body,
  createdAt: '',
});

afterEach(() => vi.unstubAllGlobals());

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

it('keeps every loaded message while new ones stream in', async () => {
  const all = Array.from({ length: 120 }, (_, i) => msg(i + 1));
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      const q = new URLSearchParams(url.split('?')[1]);
      const after = Number(q.get('after') ?? 0);
      const before = Number(q.get('before') ?? Infinity);
      const out = all.filter((m) => m.id > after && m.id < before);
      return new Response(
        JSON.stringify(q.has('limit') ? out.slice(-Number(q.get('limit'))) : out),
      );
    }),
  );
  const client = new QueryClient();

  const first = await client.fetchQuery(messagesQuery(client, 1));
  expect(first[0].id).toBe(71);
  client.setQueryData<Message[]>(['messages', 1], (have = []) => [...all.slice(20, 70), ...have]);
  all.push(...Array.from({ length: 30 }, (_, i) => msg(121 + i)));
  const next = await client.fetchQuery({ ...messagesQuery(client, 1), staleTime: 0 });

  expect(next.map((m) => m.id)).toEqual(Array.from({ length: 130 }, (_, i) => i + 21));
});
