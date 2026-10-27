import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import { teamRequestBody } from '@/modules/teams/api/team-request-body';
import type { Team } from '@/types/api';

export function useCreateTeam() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ team, from }: { team: Omit<Team, 'id'>; from?: string }) => {
      const query = from ? `?from=${encodeURIComponent(from)}` : '';
      return api<Team>(`/teams${query}`, {
        method: 'POST',
        body: JSON.stringify(teamRequestBody(team)),
      });
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['teams'] }),
  });
}
