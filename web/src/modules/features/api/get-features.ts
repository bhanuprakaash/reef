import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { Feature } from '@/types/api';

export function useFeatures() {
  return useQuery({ queryKey: ['features'], queryFn: () => api<Feature[]>('/features') });
}
