import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { RepoRequest } from '@/types/api';

// Requests are read by the server from GitHub and GitLab every minute, so the list is polled.
export function useRepoRequests(path: string) {
  return useQuery({
    queryKey: ['repo-requests', path],
    queryFn: () => api<RepoRequest[]>(path),
    refetchInterval: 15000,
  });
}
