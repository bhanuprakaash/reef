import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { Decision } from '@/types/api';

export function useDecisions(featureId: number) {
  return useQuery({
    queryKey: ['decisions', featureId],
    queryFn: () => api<Decision[]>(`/decisions?feature=${featureId}`),
  });
}
