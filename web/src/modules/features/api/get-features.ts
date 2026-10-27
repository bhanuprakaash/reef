import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { WorkItem } from '@/types/api';

export function useFeatures() {
  return useQuery({
    queryKey: ['features'],
    queryFn: () => api<WorkItem[]>('/work-items'),
    select: (items) => items.filter((i) => i.parentId === null),
  });
}
