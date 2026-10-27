import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';

export function useAttachSpace(spaceId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (workItemId: number) =>
      api<void>(`/spaces/${spaceId}/attach`, {
        method: 'POST',
        body: JSON.stringify({ workItemId }),
      }),
    onSuccess: () =>
      Promise.all([
        queryClient.invalidateQueries({ queryKey: ['spaces'] }),
        queryClient.invalidateQueries({ queryKey: ['work-items'] }),
        queryClient.invalidateQueries({ queryKey: ['messages', spaceId] }),
      ]),
  });
}
