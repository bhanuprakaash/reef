import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { Feature } from '@/types/api';

export function useFeature(id: number) {
  return useQuery({ queryKey: ['features', id], queryFn: () => api<Feature>(`/features/${id}`) });
}
