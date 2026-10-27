import { useQuery } from '@tanstack/react-query';
import { api, ApiError } from '@/lib/api-client';
import type { Proposal } from '@/types/api';

export function useProposal(spaceId: number) {
  return useQuery({
    queryKey: ['proposal', spaceId],
    queryFn: () =>
      api<Proposal>(`/spaces/${spaceId}/proposal`).catch((err: unknown) => {
        if (err instanceof ApiError && err.status === 404) return null;
        throw err;
      }),
  });
}
