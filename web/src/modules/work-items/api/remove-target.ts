import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';

export function useRemoveTarget(id: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (projectId: number) =>
      api<void>(`/work-items/${id}/targets/${projectId}`, { method: 'DELETE' }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['work-items'] }),
  });
}
