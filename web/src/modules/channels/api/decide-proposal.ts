import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import { briefsKey, memoryRootKey } from '@/lib/query-keys';
import type { Proposal } from '@/types/api';

type Decision = { proposal: Proposal; action: 'approve' | 'reject' };
type Decided = Proposal & { itemsAdded?: number; warnings?: string[] };

export function useDecideProposal(channelId: number, featureId: number | null) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ proposal, action }: Decision) =>
      api<Decided>(`/proposals/${proposal.id}/${action}`, {
        method: 'POST',
        body: JSON.stringify({}),
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['proposal', channelId] });
      void queryClient.invalidateQueries({ queryKey: ['channels'] });
      void queryClient.invalidateQueries({ queryKey: memoryRootKey });
      if (featureId === null) return;
      void queryClient.invalidateQueries({ queryKey: briefsKey(featureId) });
    },
  });
}
