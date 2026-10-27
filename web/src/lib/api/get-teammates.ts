import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { Teammate } from '@/types/api';

export const teammatesQuery = {
  queryKey: ['teammates'],
  queryFn: () => api<Teammate[]>('/teammates'),
};

export function useTeammates() {
  return useQuery(teammatesQuery);
}
