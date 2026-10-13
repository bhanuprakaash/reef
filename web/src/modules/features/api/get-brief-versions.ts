import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import { briefsKey } from '@/lib/query-keys';
import type { Brief } from '@/types/api';

export function useBriefVersions(featureId: number) {
  return useQuery({
    queryKey: briefsKey(featureId),
    queryFn: () => api<Brief[]>(`/features/${featureId}/brief/versions`),
  });
}
