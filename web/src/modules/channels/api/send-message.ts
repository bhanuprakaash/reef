import { useMutation, useQueryClient } from '@tanstack/react-query';
import { sendMessage } from '@/lib/api/send-message';

export function useSendMessage(channelId: number, parent?: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (text: string) => sendMessage(channelId, text, parent),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['messages', channelId] }),
  });
}
