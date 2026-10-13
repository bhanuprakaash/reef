import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { Task } from '@/types/api';

type TaskFilter = { project?: number; feature?: number };

export function useTasks(filter: TaskFilter = {}) {
  const params = new URLSearchParams();
  if (filter.project !== undefined) params.set('project', String(filter.project));
  if (filter.feature !== undefined) params.set('feature', String(filter.feature));
  return useQuery({
    queryKey: ['tasks', filter],
    queryFn: () => api<Task[]>(`/tasks?${params}`),
  });
}
