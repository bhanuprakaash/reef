import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import { memoryKey } from '@/lib/query-keys';
import type { MemoryItem } from '@/types/api';

export function useMemory(scope: string) {
  return useQuery({
    queryKey: memoryKey(scope),
    queryFn: () => api<MemoryItem[]>(`/memory?scope=${scope}`),
  });
}
