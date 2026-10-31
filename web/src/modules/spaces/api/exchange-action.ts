import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';

export function useExchangeAction(action: 'retry' | 'cancel', id: number, spaceId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => api<unknown>(`/exchanges/${id}/${action}`, { method: 'POST', body: '{}' }),
    onSuccess: () =>
      Promise.all([
        queryClient.invalidateQueries({ queryKey: ['exchanges', id] }),
        queryClient.invalidateQueries({ queryKey: ['spaces', spaceId] }),
      ]),
  });
}
