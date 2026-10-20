import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import { providersKey } from '@/lib/query-keys';

export function useRemoveProvider() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (name: string) =>
      api<void>(`/providers/${encodeURIComponent(name)}`, { method: 'DELETE' }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: providersKey }),
  });
}
