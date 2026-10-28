import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import { workItemPrKey } from '@/lib/query-keys';

export function useMergeWorkItemPR(workItemId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () =>
      api<void>(`/work-items/${workItemId}/pr/merge`, {
        method: 'POST',
        body: JSON.stringify({}),
      }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: workItemPrKey(workItemId) }),
  });
}
