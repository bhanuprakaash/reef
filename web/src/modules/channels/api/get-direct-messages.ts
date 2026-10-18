import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { DirectMessage } from '@/types/api';

export function useDirectMessages(channelId: number) {
  return useQuery({
    queryKey: ['direct-messages', channelId],
    queryFn: () => api<DirectMessage[]>(`/channels/${channelId}/direct-messages`),
  });
}
