import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { BotActivity } from '@/types/api';

export function useBotActivity(spaceId: number, botId: number) {
  return useQuery({
    queryKey: ['spaces', spaceId, 'bots', botId, 'activity'],
    queryFn: () => api<BotActivity>(`/spaces/${spaceId}/bots/${botId}/activity`),
    refetchInterval: 5000,
  });
}
