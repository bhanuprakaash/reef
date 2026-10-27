import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import { sendMessage } from '@/lib/api/send-message';
import type { Space } from '@/types/api';

type Input = { projectId: number; teammateId: number; name: string; text: string };

export function useStartConversation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ text, ...space }: Input) => {
      const created = await api<Space>('/spaces', {
        method: 'POST',
        body: JSON.stringify({ ...space, kind: 'conversation' }),
      });
      await sendMessage(created.id, text);
      return created;
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['spaces'] }),
  });
}
