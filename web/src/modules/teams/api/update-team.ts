import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import { teamRequestBody } from '@/modules/teams/api/team-request-body';
import type { Team } from '@/types/api';

export function useUpdateTeam() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ name, team }: { name: string; team: Team }) => {
      return api<Team>(`/teams/${encodeURIComponent(name)}`, {
        method: 'PUT',
        body: JSON.stringify(teamRequestBody(team)),
      });
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['teams'] }),
  });
}
