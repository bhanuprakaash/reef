import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { Turn } from '@/types/api';

export function useFailedTurns(spaceId: number) {
  return useQuery({
    queryKey: ['turns', spaceId, 'failed'],
    queryFn: () => api<Turn[]>(`/turns?space=${spaceId}&retryable=1`),
  });
}

export function useAllFailedTurns(spaceId: number) {
  return useQuery({
    queryKey: ['turns', spaceId, 'all-failed'],
    queryFn: () => api<Turn[]>(`/turns?space=${spaceId}&status=failed`),
  });
}
