import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import { teammateRequestBody } from '@/modules/teams/api/teammate-request-body';
import type { Role, Teammate } from '@/types/api';

export function useUpdateTeammate() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, role }: { id: number; role: Omit<Role, 'teammateId'> }) =>
      api<Teammate>(`/teammates/${id}`, {
        method: 'PUT',
        body: JSON.stringify(teammateRequestBody(role)),
      }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['teammates'] }),
  });
}
