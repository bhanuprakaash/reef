import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { Channel } from '@/types/api';

export function useChannels() {
  return useQuery({ queryKey: ['channels'], queryFn: () => api<Channel[]>('/channels') });
}
