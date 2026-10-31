import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { FindResult } from '@/types/api';

export function useFind(text: string) {
  const q = text.trim();
  return useQuery({
    queryKey: ['find', q],
    queryFn: () => api<FindResult[]>(`/find?q=${encodeURIComponent(q)}`),
    enabled: q !== '',
    placeholderData: keepPreviousData,
  });
}
