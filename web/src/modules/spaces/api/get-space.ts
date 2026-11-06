import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { SpaceDetail } from '@/types/api';

export function useSpace(id: number) {
  return useQuery({
    queryKey: ['spaces', id],
    queryFn: () => api<SpaceDetail>(`/spaces/${id}`),
  });
}
