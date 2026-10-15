import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { Team } from '@/types/api';

export function useUpdateTeam() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ name, team, from }: { name: string; team: Team; from?: string }) =>
      api<Team>(`/teams/${name}${from ? `?from=${from}` : ''}`, {
        method: 'PUT',
        body: JSON.stringify(team),
      }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['teams'] }),
  });
}
