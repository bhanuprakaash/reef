import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import { providersKey } from '@/lib/query-keys';
import type { Provider } from '@/types/api';

type NewProvider = { name: string; kind: string; folder: string };

export function useAddProvider() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: NewProvider) =>
      api<Provider>('/providers', { method: 'POST', body: JSON.stringify(input) }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: providersKey }),
  });
}
