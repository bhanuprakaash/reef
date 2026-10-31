import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import { connectionsKey } from '@/lib/query-keys';
import type { Connection } from '@/types/api';

export type ConnectionInput = Omit<Connection, 'id' | 'createdAt'>;

export function useConnections() {
  return useQuery({ queryKey: connectionsKey, queryFn: () => api<Connection[]>('/connections') });
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

export function useDeleteConnection() {
  const refresh = useRefreshConnections();
  return useMutation({
    mutationFn: (id: number) => api<void>(`/connections/${id}`, { method: 'DELETE' }),
    onSuccess: refresh,
  });
}
