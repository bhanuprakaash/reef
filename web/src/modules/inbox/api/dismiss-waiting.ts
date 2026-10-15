import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';

export function useDismissWaiting() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ channelId, messageId }: { channelId: number; messageId: number }) =>
      api<void>(`/channels/${channelId}/dismiss`, {
        method: 'POST',
        body: JSON.stringify({ messageId }),
      }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['inbox'] }),
  });
}
