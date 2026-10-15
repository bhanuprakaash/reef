import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { Channel } from '@/types/api';

export const channelsQuery = { queryKey: ['channels'], queryFn: () => api<Channel[]>('/channels') };

export function useChannels() {
  return useQuery(channelsQuery);
}
