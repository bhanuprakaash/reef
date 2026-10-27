import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';

type Add = { space: number; teammateId: number; pattern: string; stopSpace?: number };

export function useAddRoleTool() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ space, teammateId, pattern, stopSpace }: Add) => {
      await api<void>(`/spaces/${space}/members/${teammateId}/tools`, {
        method: 'POST',
        body: JSON.stringify({ tool: pattern }),
      });
      if (stopSpace !== undefined) {
        await api<void>(`/spaces/${stopSpace}/stop`, { method: 'POST', body: '{}' });
      }
    },
    onSettled: (_data, _error, { space }) =>
      queryClient.invalidateQueries({ queryKey: ['spaces', space, 'members'] }),
  });
}
