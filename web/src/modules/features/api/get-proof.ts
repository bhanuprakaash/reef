import { useQuery } from '@tanstack/react-query';
import { ApiError, api } from '@/lib/api-client';
import type { Proof } from '@/types/api';

export function useProof(workItemId: number) {
  return useQuery({
    queryKey: ['features', workItemId, 'proof'],
    queryFn: () =>
      api<Proof>(`/work-items/${workItemId}/proof`).catch((err: unknown) => {
        if (err instanceof ApiError && err.status === 404) return null;
        throw err;
      }),
  });
}
