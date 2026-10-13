import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { Brief } from '@/types/api';

export function useBriefVersions(featureId: number) {
  return useQuery({
    queryKey: ['briefs', featureId],
    queryFn: () => api<Brief[]>(`/features/${featureId}/brief/versions`),
  });
}
