import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { Role } from '@/types/api';

export function useSpaceMembers(spaceId: number) {
  return useQuery({
    queryKey: ['spaces', spaceId, 'members'],
    queryFn: () => api<Role[]>(`/spaces/${spaceId}/members`),
  });
}
