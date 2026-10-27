import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import { workItemPrKey } from '@/lib/query-keys';
import type { WorkItemPR } from '@/types/api';

export function useWorkItemPR(workItemId: number) {
  return useQuery({
    queryKey: workItemPrKey(workItemId),
    queryFn: () => api<{ pr: WorkItemPR | null }>(`/work-items/${workItemId}/pr`),
    refetchInterval: 15000,
  });
}
