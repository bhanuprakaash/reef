import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { Turn } from '@/types/api';

export function useTurns(channelId: number) {
  return useQuery({
    queryKey: ['turns', channelId, 'all'],
    queryFn: () => api<Turn[]>(`/turns?channel=${channelId}`),
  });
}
