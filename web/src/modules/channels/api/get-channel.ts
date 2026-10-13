import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { Channel } from '@/types/api';

export function useChannel(id: number) {
  return useQuery({ queryKey: ['channels', id], queryFn: () => api<Channel>(`/channels/${id}`) });
}
