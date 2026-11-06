import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import { pendingMemoryKey } from '@/lib/query-keys';
import type { PendingMemory } from '@/types/api';

export function usePendingMemory() {
  return useQuery({
    queryKey: pendingMemoryKey,
    queryFn: () => api<PendingMemory[]>('/memory/pending'),
  });
}
