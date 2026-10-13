import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import { decisionsKey } from '@/lib/query-keys';
import type { Decision } from '@/types/api';

export function useDecisions(featureId: number) {
  return useQuery({
    queryKey: decisionsKey(featureId),
    queryFn: () => api<Decision[]>(`/decisions?feature=${featureId}`),
  });
}
