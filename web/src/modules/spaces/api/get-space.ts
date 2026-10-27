import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { Space } from '@/types/api';

export function useSpace(id: number) {
  return useQuery({ queryKey: ['spaces', id], queryFn: () => api<Space>(`/spaces/${id}`) });
}
