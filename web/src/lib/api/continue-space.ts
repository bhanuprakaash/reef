import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';

export function useContinueSpace() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (spaceId: number) =>
      api<void>(`/spaces/${spaceId}/continue`, { method: 'POST', body: JSON.stringify({}) }),
    onSuccess: (_, spaceId) =>
      Promise.all([
        queryClient.invalidateQueries({ queryKey: ['spaces', spaceId] }),
        queryClient.invalidateQueries({ queryKey: ['inbox'] }),
        queryClient.invalidateQueries({ queryKey: ['turns'] }),
      ]),
  });
}
