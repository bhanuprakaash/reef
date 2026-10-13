import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { Message } from '@/types/api';

export function useMessages(channelId: number) {
  return useQuery({
    queryKey: ['messages', channelId],
    queryFn: () => api<Message[]>(`/messages?channel=${channelId}`),
  });
}
