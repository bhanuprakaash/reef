import { useQuery } from '@tanstack/react-query';
import { useMemo, useState } from 'react';
import { api } from '@/lib/api-client';
import type { Message } from '@/types/api';

export const MESSAGE_PAGE = 50;

export const messagesQuery = (spaceId: number) => ({
  queryKey: ['messages', spaceId],
  queryFn: () => api<Message[]>(`/messages?space=${spaceId}&limit=${MESSAGE_PAGE}`),
});

export function mergeMessages(earlier: Message[], newest: Message[]) {
  const byId = new Map([...earlier, ...newest].map((m) => [m.id, m]));
  const all = [...byId.values()].sort((a, b) => a.id - b.id);
  return all.filter((m) => m.parentId === null || byId.has(m.parentId));
}

// The newest page is the live query, refetched whenever a turn streams or ends. Earlier pages are
// fetched once on demand, together with the newest page as it was then, and merged by id, so a
// refetch that slides the newest window never repeats, drops or reorders messages.
export function useMessages(spaceId: number) {
  const query = useQuery(messagesQuery(spaceId));
  const [earlier, setEarlier] = useState<Message[]>([]);
  const [atStart, setAtStart] = useState(false);
  const [loadingEarlier, setLoadingEarlier] = useState(false);
  const [earlierError, setEarlierError] = useState<Error>();

  const loaded = useMemo(() => mergeMessages(earlier, query.data ?? []), [earlier, query.data]);

  async function loadEarlier() {
    const before = Math.min(...[...earlier, ...(query.data ?? [])].map((m) => m.id));
    setLoadingEarlier(true);
    setEarlierError(undefined);
    try {
      const page = await api<Message[]>(
        `/messages?space=${spaceId}&limit=${MESSAGE_PAGE}&before=${before}`,
      );
      setEarlier((prev) => [...page, ...prev, ...(query.data ?? [])]);
      if (page.length < MESSAGE_PAGE) setAtStart(true);
    } catch (e) {
      setEarlierError(e as Error);
    } finally {
      setLoadingEarlier(false);
    }
  }

  return {
    ...query,
    data: query.isSuccess ? loaded : undefined,
    hasEarlier: !atStart && (query.data?.length ?? 0) >= MESSAGE_PAGE,
    loadingEarlier,
    earlierError,
    loadEarlier,
  };
}
