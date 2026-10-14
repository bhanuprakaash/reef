import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import { diffKey } from '@/lib/query-keys';
import type { FileDiff } from '@/types/api';

export function useDiff(featureId: number) {
  return useQuery({
    queryKey: diffKey(featureId),
    queryFn: () => api<FileDiff[]>(`/features/${featureId}/diff`),
  });
}
