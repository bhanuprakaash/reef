import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { SpacePreview } from '@/types/api';

export const CHAT_PAGE = 20;

const path = (id: number, projectId: number | undefined, rest: string) =>
  `/bots/${id}/conversations?${projectId === undefined ? '' : `project=${projectId}&`}${rest}`;

export function useBotLatestChat(id: number, projectId: number | undefined, enabled = true) {
  return useQuery({
    queryKey: ['spaces', 'bot', id, 'latest', projectId ?? 'all'],
    queryFn: async () =>
      (await api<SpacePreview[]>(path(id, projectId, 'resumable=1&limit=1')))[0] ?? null,
    enabled,
  });
}

export function useBotConversations(id: number, projectId: number | undefined) {
  return useInfiniteQuery({
    queryKey: ['spaces', 'bot', id, 'list', projectId ?? 'all'],
    initialPageParam: undefined as number | undefined,
    queryFn: ({ pageParam }) =>
      api<SpacePreview[]>(
        path(id, projectId, `limit=${CHAT_PAGE}${pageParam ? `&before=${pageParam}` : ''}`),
      ),
    getNextPageParam: (last) => (last.length < CHAT_PAGE ? undefined : last[last.length - 1].id),
  });
}
