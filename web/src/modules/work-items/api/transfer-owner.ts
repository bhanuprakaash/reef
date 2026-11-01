import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { WorkItem } from '@/types/api';

export function useTransferOwner(id: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (botId: number) =>
      api<{ workItem: WorkItem; pending: boolean }>(`/work-items/${id}/owner`, {
        method: 'POST',
        body: JSON.stringify({ botId }),
      }),
    onSettled: () =>
      Promise.all([
        queryClient.invalidateQueries({ queryKey: ['work-items'] }),
        queryClient.invalidateQueries({ queryKey: ['spaces'] }),
        queryClient.invalidateQueries({ queryKey: ['messages'] }),
      ]),
  });
}
