import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { Message } from '@/types/api';

export const messagesQuery = (channelId: number) => ({
  queryKey: ['messages', channelId],
  queryFn: () => api<Message[]>(`/messages?channel=${channelId}`),
});

export function useMessages(channelId: number) {
  return useQuery(messagesQuery(channelId));
}
