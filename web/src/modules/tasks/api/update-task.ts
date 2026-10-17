import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { Task } from '@/types/api';

type TaskEdit = { task: Task } & Partial<Pick<Task, 'title' | 'body' | 'featureId' | 'status'>>;

export function useUpdateTask() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ task, ...edit }: TaskEdit) =>
      api<Task>(`/tasks/${task.id}`, {
        method: 'PUT',
        body: JSON.stringify({
          title: task.title,
          body: task.body,
          featureId: task.featureId,
          status: task.status,
          ...edit,
        }),
      }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['tasks'] }),
  });
}
