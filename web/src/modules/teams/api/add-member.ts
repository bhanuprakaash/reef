import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { Role } from '@/types/api';

export function useAddMember() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ team, botId }: { team: string; botId: number }) =>
      api<Role>(`/teams/${encodeURIComponent(team)}/members`, {
        method: 'POST',
        body: JSON.stringify({ botId, lead: false }),
      }),
    // Not awaited: the editor adds the member to its draft before the refetch lands.
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: ['teams'] }),
  });
}
