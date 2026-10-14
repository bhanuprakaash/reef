import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import { inspectorKey } from '@/lib/query-keys';
import type { Inspector } from '@/types/api';

export function useInspector(channelId: number, enabled: boolean) {
  return useQuery({
    queryKey: inspectorKey(channelId),
    queryFn: () => api<Inspector>(`/channels/${channelId}/inspector`),
    enabled,
  });
}
