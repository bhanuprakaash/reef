import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';

export const worktreeKey = (workItemId: number) => ['features', workItemId, 'worktree'];

// path is empty while the work item has no worktree.
export function useWorktree(workItemId: number) {
  return useQuery({
    queryKey: worktreeKey(workItemId),
    queryFn: () => api<{ path: string }>(`/work-items/${workItemId}/worktree`),
  });
}
