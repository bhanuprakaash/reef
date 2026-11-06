import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { Project } from '@/types/api';

export function useProjects() {
  return useQuery({
    queryKey: ['projects'],
    queryFn: () => api<Project[]>('/projects'),
    refetchInterval: 3000,
    retry: false,
  });
}

export function useDeletedProjects() {
  return useQuery({
    queryKey: ['projects', 'deleted'],
    queryFn: () => api<Project[]>('/projects?deleted=1'),
    refetchInterval: 3000,
  });
}
