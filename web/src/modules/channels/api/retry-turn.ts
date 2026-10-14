import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';

export function useRetryTurn(channelId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (turnId: number) =>
      api<void>(`/turns/${turnId}/retry`, { method: 'POST', body: JSON.stringify({}) }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['turns', channelId] }),
  });
}
