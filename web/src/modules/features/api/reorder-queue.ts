import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { WorkItem } from '@/types/api';

export function useReorderQueue() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, position }: { id: number; position: number }) =>
      api<WorkItem>(`/work-items/${id}/queue-position`, {
        method: 'PUT',
        body: JSON.stringify({ position }),
      }),
    onSettled: () => queryClient.invalidateQueries({ queryKey: ['features'] }),
  });
}
