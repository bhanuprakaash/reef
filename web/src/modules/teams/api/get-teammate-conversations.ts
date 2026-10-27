import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { SpacePreview } from '@/types/api';

export function useTeammateConversations(id: number) {
  return useQuery({
    queryKey: ['spaces', 'teammate', id],
    queryFn: () => api<SpacePreview[]>(`/teammates/${id}/conversations`),
    refetchInterval: 5000,
  });
}
