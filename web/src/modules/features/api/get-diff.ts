import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import { diffKey } from '@/lib/query-keys';
import type { FileDiff } from '@/types/api';

// refetchInterval keeps the panel current while open: it has no channel to listen to a turn's
// done event on, since a feature can have several channels writing to its worktree.
export function useDiff(featureId: number) {
  return useQuery({
    queryKey: diffKey(featureId),
    queryFn: () => api<FileDiff[]>(`/features/${featureId}/diff`),
    refetchInterval: 5000,
  });
}
