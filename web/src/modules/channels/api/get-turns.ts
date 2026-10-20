import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { Turn } from '@/types/api';

export function useTurns(channelId: number) {
  return useQuery({
    queryKey: ['turns', channelId, 'rollbackable'],
    queryFn: () => api<Turn[]>(`/turns?channel=${channelId}&rollbackable=1`),
  });
}

export function useRunningTurns() {
  return useQuery({
    queryKey: ['turns', 'running'],
    queryFn: () => api<Turn[]>('/turns?status=running'),
    refetchInterval: 3000,
  });
}
