import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { Turn } from '@/types/api';

export function useTurns(spaceId: number) {
  return useQuery({
    queryKey: ['turns', spaceId, 'rollbackable'],
    queryFn: () => api<Turn[]>(`/turns?space=${spaceId}&rollbackable=1`),
  });
}

export function useSpaceTurns(spaceId: number) {
  return useQuery({
    queryKey: ['turns', spaceId],
    queryFn: () => api<Turn[]>(`/turns?space=${spaceId}`),
  });
}

export function useRunningTurns() {
  return useQuery({
    queryKey: ['turns', 'running'],
    queryFn: () => api<Turn[]>('/turns?status=running'),
    refetchInterval: 3000,
  });
}

export function useSpaceRunningTurns(spaceId: number, enabled: boolean) {
  return useQuery({
    queryKey: ['turns', spaceId, 'running'],
    queryFn: () => api<Turn[]>(`/turns?space=${spaceId}&status=running`),
    enabled,
    refetchInterval: (query) => (query.state.data?.length ? false : 1000),
  });
}
