import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';

export function useReopenSpace(spaceId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => api<void>(`/spaces/${spaceId}/reopen`, { method: 'POST', body: '{}' }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['spaces'] }),
  });
}
