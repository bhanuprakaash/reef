import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { WorkItem, WorkItemStatus } from '@/types/api';

export function useMoveWorkItem() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, status }: { id: number; status: WorkItemStatus }) =>
      api<WorkItem>(`/work-items/${id}`, { method: 'PATCH', body: JSON.stringify({ status }) }),
    onSettled: () => queryClient.invalidateQueries({ queryKey: ['work-items'] }),
  });
}
