import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';

export function useRetryTurn() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (turnId: number) =>
      api<void>(`/turns/${turnId}/retry`, { method: 'POST', body: JSON.stringify({}) }),
    onSuccess: () =>
      Promise.all([
        queryClient.invalidateQueries({ queryKey: ['turns'] }),
        queryClient.invalidateQueries({ queryKey: ['inbox'] }),
      ]),
  });
}
