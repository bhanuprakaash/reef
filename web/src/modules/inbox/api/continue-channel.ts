import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';

export function useContinueChannel() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (channelId: number) =>
      api<void>(`/channels/${channelId}/continue`, { method: 'POST', body: JSON.stringify({}) }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['inbox'] }),
  });
}
