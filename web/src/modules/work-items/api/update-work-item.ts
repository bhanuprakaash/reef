import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { WorkItem } from '@/types/api';

export function useUpdateWorkItem(id: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: WorkItem) =>
      api<WorkItem>(`/work-items/${id}`, { method: 'PUT', body: JSON.stringify(input) }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['work-items'] }),
  });
}
