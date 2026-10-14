import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';

// useFeatureWorktree ensures the feature's worktree, made the same way a first wake would,
// and returns its path. enabled skips the call where there is no terminal pane to show it to.
export function useFeatureWorktree(featureId: number, enabled: boolean) {
  return useQuery({
    queryKey: ['features', featureId, 'worktree'],
    queryFn: () =>
      api<{ path: string }>(`/features/${featureId}/worktree`, {
        method: 'POST',
        body: JSON.stringify({}),
      }),
    enabled,
  });
}
