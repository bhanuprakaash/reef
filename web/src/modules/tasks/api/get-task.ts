import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { Task } from '@/types/api';

export function useTask(id: number) {
  return useQuery({ queryKey: ['tasks', id], queryFn: () => api<Task>(`/tasks/${id}`) });
}
