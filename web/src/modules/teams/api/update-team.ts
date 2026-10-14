import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { Team } from '@/types/api';

export function useUpdateTeam() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ name, team }: { name: string; team: Team }) =>
      api<Team>(`/teams/${name}`, { method: 'PUT', body: JSON.stringify(team) }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['teams'] }),
  });
}
