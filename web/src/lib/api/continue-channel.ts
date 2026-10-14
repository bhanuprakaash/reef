import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';

export function useContinueChannel() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (channelId: number) =>
      api<void>(`/channels/${channelId}/continue`, { method: 'POST', body: JSON.stringify({}) }),
    onSuccess: (_, channelId) =>
      Promise.all([
        queryClient.invalidateQueries({ queryKey: ['channels', channelId] }),
        queryClient.invalidateQueries({ queryKey: ['inbox'] }),
      ]),
  });
}
