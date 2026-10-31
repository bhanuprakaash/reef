import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { Exchange } from '@/types/api';

export function useExchange(id: number) {
  return useQuery({
    queryKey: ['exchanges', id],
    queryFn: () => api<Exchange>(`/exchanges/${id}`),
  });
}
