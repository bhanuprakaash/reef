import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';

type RefreshBotInput = { botId: number; model?: string };

export function useRefreshBot(spaceId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ botId, model }: RefreshBotInput) => {
      const res = await api<{ pending: boolean } | undefined>(
        `/spaces/${spaceId}/bots/${botId}/refresh`,
        { method: 'POST', body: JSON.stringify(model ? { model } : {}) },
      );
      // The api client returns no body for 202, the reply of a refresh that waits for a turn.
      return { pending: res === undefined };
    },
    onSuccess: () =>
      Promise.all([
        queryClient.invalidateQueries({ queryKey: ['spaces', spaceId] }),
        queryClient.invalidateQueries({ queryKey: ['bots'] }),
      ]),
  });
}
