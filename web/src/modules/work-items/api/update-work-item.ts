import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { WorkItem } from '@/types/api';

type Input = Omit<WorkItem, 'acceptanceLines'> & Partial<Pick<WorkItem, 'acceptanceLines'>>;

export function useUpdateWorkItem(id: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: Input) =>
      api<WorkItem>(`/work-items/${id}`, { method: 'PUT', body: JSON.stringify(input) }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['work-items'] }),
  });
}
