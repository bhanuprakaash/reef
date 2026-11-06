import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { MemberProposal } from '@/types/api';

export function useMemberProposals(spaceId: number) {
  return useQuery({
    queryKey: ['member-proposals', spaceId],
    queryFn: () => api<MemberProposal[]>(`/spaces/${spaceId}/member-proposals`),
  });
}

export function useDecideMemberProposal(spaceId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, action }: { id: number; action: 'approve' | 'reject' }) =>
      api<MemberProposal>(`/member-proposals/${id}/${action}`, { method: 'POST' }),
    onSuccess: () =>
      Promise.all([
        queryClient.invalidateQueries({ queryKey: ['member-proposals', spaceId] }),
        queryClient.invalidateQueries({ queryKey: ['spaces', spaceId] }),
      ]),
  });
}
