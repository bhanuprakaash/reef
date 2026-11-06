import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { WorkItem } from '@/types/api';

export const workItemsQuery = {
  queryKey: ['work-items'],
  queryFn: () => api<WorkItem[]>('/work-items'),
  select: (items: WorkItem[]) => items.filter((i) => i.parentId === null),
};

export function useWorkItems() {
  return useQuery(workItemsQuery);
}
