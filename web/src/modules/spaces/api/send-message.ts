import { useMutation, useQueryClient } from '@tanstack/react-query';
import { sendMessage } from '@/lib/api/send-message';

export function useSendMessage(spaceId: number, parent?: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (text: string) => sendMessage(spaceId, text, parent),
    onSuccess: () =>
      Promise.all([
        queryClient.invalidateQueries({ queryKey: ['messages', spaceId] }),
        queryClient.invalidateQueries({ queryKey: ['turns'] }),
      ]),
  });
}
