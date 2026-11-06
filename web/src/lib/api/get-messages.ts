import { useMutation, useQueries, useQuery, useQueryClient } from '@tanstack/react-query';
import type { QueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { api } from '@/lib/api-client';
import type { Message } from '@/types/api';

export const MESSAGE_PAGE = 50;

export function mergeMessages(earlier: Message[], newest: Message[]) {
  const byId = new Map([...earlier, ...newest].map((m) => [m.id, m]));
  const all = [...byId.values()].sort((a, b) => a.id - b.id);
  return all.filter((m) => m.parentId === null || byId.has(m.parentId));
}

// The first fetch is the newest page. Each refetch keeps what is loaded below its newest page and
// replaces the rest, so streaming never slides the window and loaded earlier pages stay.
export const messagesQuery = (queryClient: QueryClient, spaceId: number) => ({
  queryKey: ['messages', spaceId],
  queryFn: async () => {
    const have = queryClient.getQueryData<Message[]>(['messages', spaceId]) ?? [];
    const from = (have.at(-MESSAGE_PAGE) ?? have[0])?.id;
    if (from === undefined) {
      return api<Message[]>(`/messages?space=${spaceId}&limit=${MESSAGE_PAGE}`);
    }
    const tail = await api<Message[]>(`/messages?space=${spaceId}&after=${from - 1}`);
    const now = queryClient.getQueryData<Message[]>(['messages', spaceId]) ?? [];
    return [...now.filter((m) => m.id < from), ...tail];
  },
});

// The newest message below id + 1 is the message with that id.
const rootQuery = (spaceId: number, id: number) => ({
  queryKey: ['message-root', spaceId, id],
  queryFn: async () =>
    (await api<Message[]>(`/messages?space=${spaceId}&limit=1&before=${id + 1}`))[0],
  staleTime: Infinity,
});

export function useMessages(spaceId: number) {
  const queryClient = useQueryClient();
  const query = useQuery(messagesQuery(queryClient, spaceId));
  const [atStart, setAtStart] = useState(false);
  const loaded = query.data;

  const earlier = useMutation({
    mutationFn: async () => {
      const page = await api<Message[]>(
        `/messages?space=${spaceId}&limit=${MESSAGE_PAGE}&before=${loaded![0].id}`,
      );
      queryClient.setQueryData<Message[]>(['messages', spaceId], (have = []) => [...page, ...have]);
      if (page.length < MESSAGE_PAGE) setAtStart(true);
    },
  });

  const ids = new Set(loaded?.map((m) => m.id));
  const missing = [
    ...new Set(
      loaded?.flatMap((m) => (m.parentId !== null && !ids.has(m.parentId) ? [m.parentId] : [])),
    ),
  ];
  const roots = useQueries({ queries: missing.map((id) => rootQuery(spaceId, id)) });

  return {
    ...query,
    data:
      loaded &&
      mergeMessages(
        roots.flatMap((r) => (r.data ? [r.data] : [])),
        loaded,
      ),
    hasEarlier: !atStart && (loaded?.length ?? 0) >= MESSAGE_PAGE,
    loadingEarlier: earlier.isPending,
    earlierError: earlier.error ?? undefined,
    loadEarlier: earlier.mutate,
  };
}
