import { useQuery } from '@tanstack/react-query';
import { api, ApiError } from '@/lib/api-client';
import type { Proposal } from '@/types/api';

export function useProposal(channelId: number) {
  return useQuery({
    queryKey: ['proposal', channelId],
    queryFn: () =>
      api<Proposal>(`/channels/${channelId}/proposal`).catch((err: unknown) => {
        if (err instanceof ApiError && err.status === 404) return null;
        throw err;
      }),
  });
}
