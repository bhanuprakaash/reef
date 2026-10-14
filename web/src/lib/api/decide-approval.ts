import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';

export function useDecideApproval() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ channelId, id, approve }: { channelId: number; id: number; approve: boolean }) =>
      api<void>(`/channels/${channelId}/approvals/${id}`, {
        method: 'POST',
        body: JSON.stringify({ approve }),
      }),
    onSuccess: (_, { channelId }) =>
      Promise.all([
        queryClient.invalidateQueries({ queryKey: ['approvals', channelId] }),
        queryClient.invalidateQueries({ queryKey: ['inbox'] }),
      ]),
  });
}
