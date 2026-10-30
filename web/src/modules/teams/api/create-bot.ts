import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import { botRequestBody } from '@/modules/teams/api/bot-request-body';
import type { Role, Bot } from '@/types/api';

export function useCreateBot() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (role: Omit<Role, 'botId'>) =>
      api<Bot>('/bots', {
        method: 'POST',
        body: JSON.stringify(botRequestBody(role)),
      }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['bots'] }),
  });
}
