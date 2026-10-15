import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { Team } from '@/types/api';

export function useUpdateTeam() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ name, team, from }: { name: string; team: Team; from?: string }) => {
      const query = from ? `?from=${encodeURIComponent(from)}` : '';
      return api<Team>(`/teams/${encodeURIComponent(name)}${query}`, {
        method: 'PUT',
        body: JSON.stringify({
          ...team,
          roles: team.roles.map((role) => ({ ...role, mcp: undefined })),
        }),
      });
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['teams'] }),
  });
}
