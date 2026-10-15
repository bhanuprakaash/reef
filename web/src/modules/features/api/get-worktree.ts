import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';

export const worktreeKey = (featureId: number) => ['features', featureId, 'worktree'];

// path is empty while the feature has no worktree.
export function useWorktree(featureId: number) {
  return useQuery({
    queryKey: worktreeKey(featureId),
    queryFn: () => api<{ path: string }>(`/features/${featureId}/worktree`),
  });
}
