import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';

export function useAddTeammate(spaceId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (teammateId: number) =>
      api<void>(`/spaces/${spaceId}/members`, {
        method: 'POST',
        body: JSON.stringify({ teammateId }),
      }),
    onSuccess: () =>
      Promise.all([
        queryClient.invalidateQueries({ queryKey: ['spaces', spaceId] }),
        queryClient.invalidateQueries({ queryKey: ['messages', spaceId] }),
      ]),
  });
}
