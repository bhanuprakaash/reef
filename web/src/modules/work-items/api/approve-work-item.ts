import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { WorkItem } from '@/types/api';

type Approval = { ownerBotId: number; participantBotIds: number[]; key: string };

export function useApproveWorkItem(id: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (approval: Approval) =>
      api<WorkItem>(`/work-items/${id}/approve`, {
        method: 'POST',
        body: JSON.stringify(approval),
      }),
    onSettled: () =>
      Promise.all([
        queryClient.invalidateQueries({ queryKey: ['work-items'] }),
        queryClient.invalidateQueries({ queryKey: ['spaces'] }),
      ]),
  });
}
