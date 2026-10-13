import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';

type Promotion = { slug: string; name: string } | { featureId: number };

export function usePromoteChannel(channelId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: Promotion) =>
      api<void>(`/channels/${channelId}/promote`, { method: 'POST', body: JSON.stringify(input) }),
    onSuccess: () =>
      Promise.all([
        queryClient.invalidateQueries({ queryKey: ['channels'] }),
        queryClient.invalidateQueries({ queryKey: ['features'] }),
        queryClient.invalidateQueries({ queryKey: ['messages', channelId] }),
      ]),
  });
}
