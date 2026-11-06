import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import { botRequestBody } from '@/lib/api/bot-request-body';
import type { Role } from '@/types/api';

type Add = { role: Role; pattern: string; spaceId: number };

export function useAddBotTool() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ role, pattern, spaceId }: Add) => {
      await api<void>(`/bots/${role.botId}`, {
        method: 'PUT',
        body: JSON.stringify(botRequestBody({ ...role, tools: [...role.tools, pattern] })),
      });
      await api<unknown>(`/spaces/${spaceId}/bots/${role.botId}/refresh`, {
        method: 'POST',
        body: '{}',
      });
    },
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: ['bots'] });
      void queryClient.invalidateQueries({ queryKey: ['teams'] });
      void queryClient.invalidateQueries({ queryKey: ['spaces'] });
    },
  });
}
