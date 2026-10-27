import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import { briefsKey, memoryRootKey } from '@/lib/query-keys';
import type { WorkItem } from '@/types/api';

type Decision = { status: 'done' } | { status: 'queued'; note: string };

export function useDecideReview(workItemId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (decision: Decision) =>
      api<WorkItem>(`/work-items/${workItemId}`, {
        method: 'PATCH',
        body: JSON.stringify(decision),
      }),
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: ['features'] });
      void queryClient.invalidateQueries({ queryKey: ['proposal'] });
      void queryClient.invalidateQueries({ queryKey: ['spaces'] });
      void queryClient.invalidateQueries({ queryKey: memoryRootKey });
      void queryClient.invalidateQueries({ queryKey: briefsKey(workItemId) });
    },
  });
}
