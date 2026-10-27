import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { Message } from '@/types/api';

export const messagesQuery = (spaceId: number) => ({
  queryKey: ['messages', spaceId],
  queryFn: () => api<Message[]>(`/messages?space=${spaceId}`),
});

export function useMessages(spaceId: number) {
  return useQuery(messagesQuery(spaceId));
}
