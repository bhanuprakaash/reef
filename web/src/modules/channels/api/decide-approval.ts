import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';

export function useDecideApproval(channelId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, approve }: { id: number; approve: boolean }) =>
      api<void>(`/channels/${channelId}/approvals/${id}`, {
        method: 'POST',
        body: JSON.stringify({ approve }),
      }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['approvals', channelId] }),
  });
}
