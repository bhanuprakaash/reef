import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';

export function useRenameSpace(spaceId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (name: string) =>
      api<void>(`/spaces/${spaceId}`, { method: 'PUT', body: JSON.stringify({ name }) }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['spaces'] }),
  });
}
