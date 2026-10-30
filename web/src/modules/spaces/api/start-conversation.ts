import { useMutation, useQueryClient } from '@tanstack/react-query';
import { sendMessage } from '@/lib/api/send-message';
import { createSpace } from '@/modules/spaces/api/create-space';

type Input = { projectId: number; bot: { id: number; name: string }; text?: string };

export function useStartConversation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ projectId, bot, text }: Input) => {
      const time = new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
      const created = await createSpace({
        projectId,
        name: `${bot.name} ${time}`,
        botId: bot.id,
        kind: 'conversation',
      });
      if (text) await sendMessage(created.id, text);
      return created;
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['spaces'] }),
  });
}
