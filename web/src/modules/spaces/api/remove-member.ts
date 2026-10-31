import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';

type RemoveMemberInput = { botId: number; coordinator?: number };

export function useRemoveMember(spaceId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ botId, coordinator }: RemoveMemberInput) => {
      const res = await api<{ pending: boolean } | undefined>(
        `/spaces/${spaceId}/members/${botId}`,
        { method: 'DELETE', body: JSON.stringify(coordinator ? { coordinator } : {}) },
      );
      // The api client returns no body for 202, the reply of a removal that waits for the turn.
      return { botId, pending: res === undefined };
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['spaces', spaceId] }),
  });
}
