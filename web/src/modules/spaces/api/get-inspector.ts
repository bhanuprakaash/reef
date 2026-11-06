import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import { inspectorKey } from '@/lib/query-keys';
import type { Inspector } from '@/types/api';

export function useInspector(spaceId: number, enabled: boolean) {
  return useQuery({
    queryKey: inspectorKey(spaceId),
    queryFn: () => api<Inspector>(`/spaces/${spaceId}/inspector`),
    enabled,
  });
}
