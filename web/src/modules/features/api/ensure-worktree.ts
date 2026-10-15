import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import { worktreeKey } from '@/modules/features/api/get-worktree';

// useEnsureWorktree makes the feature's worktree, the same way a first wake would, and
// returns its path. It runs only when the user asks for the terminal, never on page view.
export function useEnsureWorktree(featureId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () =>
      api<{ path: string }>(`/features/${featureId}/worktree`, {
        method: 'POST',
        body: JSON.stringify({}),
      }),
    onSuccess: (data) => queryClient.setQueryData(worktreeKey(featureId), data),
  });
}
