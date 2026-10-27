import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import { briefsKey } from '@/lib/query-keys';
import type { Brief } from '@/types/api';

export function useBriefVersions(workItemId: number) {
  return useQuery({
    queryKey: briefsKey(workItemId),
    queryFn: () => api<Brief[]>(`/work-items/${workItemId}/brief/versions`),
  });
}
