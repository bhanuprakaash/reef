import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import { providersKey } from '@/lib/query-keys';
import type { Provider } from '@/types/api';

export function useProviders() {
  return useQuery({ queryKey: providersKey, queryFn: () => api<Provider[]>('/providers') });
}
