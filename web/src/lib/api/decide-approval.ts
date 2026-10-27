import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';

export function useDecideApproval() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ spaceId, id, approve }: { spaceId: number; id: number; approve: boolean }) =>
      api<void>(`/spaces/${spaceId}/approvals/${id}`, {
        method: 'POST',
        body: JSON.stringify({ approve }),
      }),
    onSuccess: (_, { spaceId }) =>
      Promise.all([
        queryClient.invalidateQueries({ queryKey: ['approvals', spaceId] }),
        queryClient.invalidateQueries({ queryKey: ['inbox'] }),
      ]),
  });
}
