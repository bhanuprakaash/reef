import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import { diffKey } from '@/lib/query-keys';
import type { FileDiff } from '@/types/api';

export type DiffScope = 'current' | 'full';

// refetchInterval keeps the panel current while open: it has no Space to listen to a turn's
// done event on, since a work item can have several Spaces writing to its worktree.
export function useDiff(workItemId: number, scope: DiffScope = 'full') {
  return useQuery({
    queryKey: diffKey(workItemId, scope),
    queryFn: () =>
      api<FileDiff[]>(
        `/work-items/${workItemId}/diff${scope === 'current' ? '?scope=current' : ''}`,
      ),
    placeholderData: keepPreviousData,
    refetchInterval: 5000,
  });
}
