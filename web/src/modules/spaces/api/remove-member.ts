import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';

type RemoveMemberInput = { botId: number; coordinator?: number };

export function useRemoveMember(spaceId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ botId, coordinator }: RemoveMemberInput) => {
      await api(`/spaces/${spaceId}/members/${botId}`, {
        method: 'DELETE',
        body: JSON.stringify(coordinator ? { coordinator } : {}),
      });
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['spaces', spaceId] }),
  });
}
