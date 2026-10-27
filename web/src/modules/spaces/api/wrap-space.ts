import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';

export function useWrapSpace(spaceId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () =>
      api<void>(`/spaces/${spaceId}/wrap`, { method: 'POST', body: JSON.stringify({}) }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['messages', spaceId] }),
  });
}
