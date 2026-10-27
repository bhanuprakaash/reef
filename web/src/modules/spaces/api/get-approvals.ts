import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { Approval } from '@/types/api';

export function useApprovals(spaceId: number) {
  return useQuery({
    queryKey: ['approvals', spaceId],
    queryFn: () => api<Approval[]>(`/spaces/${spaceId}/approvals`),
  });
}
