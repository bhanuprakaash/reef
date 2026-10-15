import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { ChannelPreview } from '@/types/api';

export const channelsQuery = {
  queryKey: ['channels'],
  queryFn: () => api<ChannelPreview[]>('/channels'),
};

export function useChannels() {
  return useQuery(channelsQuery);
}
