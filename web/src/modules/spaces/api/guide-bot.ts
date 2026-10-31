import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { Exchange } from '@/types/api';

export function useGuideBot(spaceId: number, botId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: string) =>
      api<Exchange>(`/spaces/${spaceId}/bots/${botId}/guidance`, {
        method: 'POST',
        body: JSON.stringify({ body, key: crypto.randomUUID() }),
      }),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: ['spaces', spaceId, 'bots', botId, 'activity'] }),
  });
}
