import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import { botRequestBody } from '@/modules/teams/api/bot-request-body';
import type { Role, Bot } from '@/types/api';

export function useUpdateBot() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, role }: { id: number; role: Omit<Role, 'botId'> }) =>
      api<Bot>(`/bots/${id}`, {
        method: 'PUT',
        body: JSON.stringify(botRequestBody(role)),
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['bots'] });
      void queryClient.invalidateQueries({ queryKey: ['teams'] });
      void queryClient.invalidateQueries({ queryKey: ['spaces'] });
    },
  });
}
