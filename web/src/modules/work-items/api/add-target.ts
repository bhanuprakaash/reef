import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { Target } from '@/types/api';

export function useAddTarget(id: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (projectId: number) =>
      api<Target>(`/work-items/${id}/targets`, {
        method: 'POST',
        body: JSON.stringify({ projectId }),
      }),
    onSuccess: () =>
      Promise.all([
        queryClient.invalidateQueries({ queryKey: ['work-items'] }),
        queryClient.invalidateQueries({ queryKey: ['messages'] }),
      ]),
  });
}
