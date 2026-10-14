import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { Turn } from '@/types/api';

export function useFailedTurns(channelId: number) {
  return useQuery({
    queryKey: ['turns', channelId],
    queryFn: () => api<Turn[]>(`/turns?channel=${channelId}&status=failed`),
  });
}
