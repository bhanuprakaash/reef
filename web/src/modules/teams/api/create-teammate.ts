import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import { teammateRequestBody } from '@/modules/teams/api/teammate-request-body';
import type { Role, Teammate } from '@/types/api';

export function useCreateTeammate() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (role: Omit<Role, 'teammateId'>) =>
      api<Teammate>('/teammates', {
        method: 'POST',
        body: JSON.stringify(teammateRequestBody(role)),
      }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['teammates'] }),
  });
}
