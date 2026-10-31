import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { Role } from '@/types/api';

export function useSetCoordinator(spaceId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (botId: number) =>
      api<Role[]>(`/spaces/${spaceId}/coordinator`, {
        method: 'PUT',
        body: JSON.stringify({ botId }),
      }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['spaces', spaceId] }),
  });
}
