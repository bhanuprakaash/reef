import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { WorkItem } from '@/types/api';

type NewFeature = Pick<WorkItem, 'projectId' | 'title'> &
  Partial<Pick<WorkItem, 'description' | 'type' | 'parentId'>>;

export function useCreateFeature() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: NewFeature) =>
      api<WorkItem>('/work-items', { method: 'POST', body: JSON.stringify(input) }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['features'] }),
  });
}
