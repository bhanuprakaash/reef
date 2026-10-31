import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';

export type RevokeResult = { sessions: number; tokens: number; toolCalls: number };

export function useRevokeBot() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: number) =>
      api<RevokeResult>(`/bots/${id}/revoke`, { method: 'POST', body: JSON.stringify({}) }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['spaces'] });
    },
  });
}
