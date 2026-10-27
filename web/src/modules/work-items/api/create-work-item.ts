import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { WorkItem } from '@/types/api';

type NewWorkItem = Pick<WorkItem, 'projectId' | 'title'> &
  Partial<Pick<WorkItem, 'description' | 'type' | 'parentId'>>;

export function useCreateWorkItem() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: NewWorkItem) =>
      api<WorkItem>('/work-items', { method: 'POST', body: JSON.stringify(input) }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['work-items'] }),
  });
}
