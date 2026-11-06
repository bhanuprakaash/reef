import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import { repoConnectionsKey } from '@/lib/query-keys';
import type { RepoConnection } from '@/types/api';

export function useRepoConnections() {
  return useQuery({
    queryKey: repoConnectionsKey,
    queryFn: () => api<RepoConnection[]>('/repo-connections'),
  });
}
