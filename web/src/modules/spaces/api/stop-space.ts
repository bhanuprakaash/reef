import { useMutation } from '@tanstack/react-query';
import { api } from '@/lib/api-client';

export function useStopSpace(spaceId: number) {
  return useMutation({
    mutationFn: () => api<void>(`/spaces/${spaceId}/stop`, { method: 'POST', body: '{}' }),
  });
}
