import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';

type Promotion = { slug: string; name: string } | { featureId: number };

export function usePromoteSpace(spaceId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: Promotion) =>
      api<void>(`/spaces/${spaceId}/promote`, { method: 'POST', body: JSON.stringify(input) }),
    onSuccess: () =>
      Promise.all([
        queryClient.invalidateQueries({ queryKey: ['spaces'] }),
        queryClient.invalidateQueries({ queryKey: ['features'] }),
        queryClient.invalidateQueries({ queryKey: ['messages', spaceId] }),
        queryClient.invalidateQueries({ queryKey: ['turns', spaceId] }),
      ]),
  });
}
