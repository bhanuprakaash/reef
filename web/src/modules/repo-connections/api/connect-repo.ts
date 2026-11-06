import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import { repoConnectionsKey } from '@/lib/query-keys';
import type { RepoConnection, RepoProvider } from '@/types/api';

export function useConnectRepo() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (provider: RepoProvider) =>
      api<RepoConnection>('/repo-connections', {
        method: 'POST',
        body: JSON.stringify({ provider }),
      }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: repoConnectionsKey }),
  });
}

export function useCheckRepoConnection() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: number) =>
      api<RepoConnection>(`/repo-connections/${id}/check`, { method: 'POST', body: '{}' }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: repoConnectionsKey }),
  });
}
