import { useMutation, useMutationState, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import { availableConnectionsKey, connectionsKey } from '@/lib/query-keys';
import { catalogInput, type CatalogEntry } from '@/modules/teams/utils/catalog';
import type { AvailableConnection, Connection, ConnectionLogin } from '@/types/api';

export type ConnectionInput = Omit<Connection, 'id' | 'createdAt' | 'signIn'>;

export function useConnections() {
  return useQuery({ queryKey: connectionsKey, queryFn: () => api<Connection[]>('/connections') });
}

export function useAvailableConnections(projectId?: number) {
  return useQuery({
    queryKey: availableConnectionsKey(projectId),
    queryFn: () =>
      api<AvailableConnection[]>(
        projectId === undefined
          ? '/connections/available'
          : `/connections/available?projectId=${projectId}`,
      ),
  });
}

function useRefreshConnections() {
  const queryClient = useQueryClient();
  return () => {
    void queryClient.invalidateQueries({ queryKey: connectionsKey });
    void queryClient.invalidateQueries({ queryKey: ['bots'] });
    void queryClient.invalidateQueries({ queryKey: ['teams'] });
  };
}

export function useSaveConnection() {
  const refresh = useRefreshConnections();
  return useMutation({
    mutationFn: ({ id, input }: { id?: number; input: ConnectionInput }) =>
      api<Connection>(id === undefined ? '/connections' : `/connections/${id}`, {
        method: id === undefined ? 'POST' : 'PUT',
        body: JSON.stringify(input),
      }),
    onSuccess: refresh,
  });
}

export function useAddCatalogConnection() {
  const refresh = useRefreshConnections();
  return useMutation({
    mutationFn: async ({
      entry,
      values,
    }: {
      entry: CatalogEntry;
      values: Record<string, string>;
    }) => {
      const created = await api<Connection>('/connections', {
        method: 'POST',
        body: JSON.stringify(catalogInput(entry, values)),
      });
      refresh();
      if (!entry.headers) {
        await api<ConnectionLogin[]>(`/connections/${created.id}/login`, {
          method: 'POST',
          body: '{}',
        });
      }
    },
    onSettled: refresh,
  });
}

export function useDeleteConnection() {
  const refresh = useRefreshConnections();
  return useMutation({
    mutationFn: (id: number) => api<void>(`/connections/${id}`, { method: 'DELETE' }),
    onSuccess: refresh,
  });
}

export function useImportConnection() {
  const refresh = useRefreshConnections();
  return useMutation({
    mutationFn: (input: { source: string; name: string; projectId?: number }) =>
      api<Connection>('/connections/import', { method: 'POST', body: JSON.stringify(input) }),
    onSuccess: refresh,
  });
}

const loginKey = ['connection-login'];

export function useLoginConnection() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: loginKey,
    mutationFn: (id: number) =>
      api<ConnectionLogin[]>(`/connections/${id}/login`, { method: 'POST', body: '{}' }),
    onSettled: () => void queryClient.invalidateQueries({ queryKey: connectionsKey }),
  });
}

export function useSigningIn(id: number) {
  return useMutationState({
    filters: { mutationKey: loginKey, status: 'pending' },
    select: (m) => m.state.variables,
  }).includes(id);
}
