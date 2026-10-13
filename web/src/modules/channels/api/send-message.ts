import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';

export function useSendMessage(channelId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (text: string) =>
      api<void>(`/channels/${channelId}/send`, { method: 'POST', body: JSON.stringify({ text }) }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['messages', channelId] }),
  });
}
