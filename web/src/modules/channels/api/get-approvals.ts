import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { Approval } from '@/types/api';

export function useApprovals(channelId: number) {
  return useQuery({
    queryKey: ['approvals', channelId],
    queryFn: () => api<Approval[]>(`/channels/${channelId}/approvals`),
  });
}
