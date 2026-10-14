import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { Team } from '@/types/api';

export function useTeams() {
  return useQuery({ queryKey: ['teams'], queryFn: () => api<Team[]>('/teams') });
}
