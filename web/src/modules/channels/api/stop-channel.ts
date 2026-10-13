import { useMutation } from '@tanstack/react-query';
import { api } from '@/lib/api-client';

export function useStopChannel(channelId: number) {
  return useMutation({
    mutationFn: () => api<void>(`/channels/${channelId}/stop`, { method: 'POST', body: '{}' }),
  });
}
