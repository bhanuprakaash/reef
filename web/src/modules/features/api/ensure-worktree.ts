import { useMutation } from '@tanstack/react-query';
import { api } from '@/lib/api-client';

// useEnsureWorktree makes the feature's worktree, the same way a first wake would, and
// returns its path. It runs only when the user opens the terminal, never on page view.
export function useEnsureWorktree(featureId: number) {
  return useMutation({
    mutationFn: () =>
      api<{ path: string }>(`/features/${featureId}/worktree`, {
        method: 'POST',
        body: JSON.stringify({}),
      }),
  });
}
