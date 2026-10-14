import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';

export function useRollbackTurn(channelId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (turnId: number) =>
      api<void>(`/turns/${turnId}/rollback`, { method: 'POST', body: JSON.stringify({}) }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['messages', channelId] }),
  });
}
