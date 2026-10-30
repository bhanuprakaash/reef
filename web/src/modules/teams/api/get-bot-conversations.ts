import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { SpacePreview } from '@/types/api';

export function useBotConversations(id: number) {
  return useQuery({
    queryKey: ['spaces', 'bot', id],
    queryFn: () => api<SpacePreview[]>(`/bots/${id}/conversations`),
    refetchInterval: 5000,
  });
}
