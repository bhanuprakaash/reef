import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { Team } from '@/types/api';

export const teamsQuery = { queryKey: ['teams'], queryFn: () => api<Team[]>('/teams') };

export function useTeams() {
  return useQuery(teamsQuery);
}
