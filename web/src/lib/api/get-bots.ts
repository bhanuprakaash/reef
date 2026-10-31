import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { Bot } from '@/types/api';

export const botsQuery = {
  queryKey: ['bots'],
  queryFn: () => api<Bot[]>('/bots'),
};

export function useBots() {
  return useQuery(botsQuery);
}
