import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { WorkItem } from '@/types/api';

export function useSubItems(parentId: number) {
  return useQuery({
    queryKey: ['features', parentId, 'sub-items'],
    queryFn: () => api<WorkItem[]>(`/work-items?parent=${parentId}`),
  });
}
