import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';

export type Project = {
  id: number;
  name: string;
  path: string;
  createdAt: string;
};

export function useProjects() {
  return useQuery({ queryKey: ['projects'], queryFn: () => api<Project[]>('/projects') });
}
