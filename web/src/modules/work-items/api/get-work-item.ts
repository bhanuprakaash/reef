import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { WorkItem } from '@/types/api';

export function useWorkItem(id: number) {
  return useQuery({
    queryKey: ['work-items', id],
    queryFn: () => api<WorkItem>(`/work-items/${id}`),
  });
}
