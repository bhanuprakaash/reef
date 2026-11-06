import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { DirectMessage } from '@/types/api';

export function useDirectMessages(spaceId: number) {
  return useQuery({
    queryKey: ['direct-messages', spaceId],
    queryFn: () => api<DirectMessage[]>(`/spaces/${spaceId}/direct-messages`),
  });
}
