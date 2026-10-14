import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import { projectMemoryKey } from '@/lib/query-keys';
import type { ProjectMemory } from '@/types/api';

export function useUpdateProjectMemory(projectId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: string) =>
      api<ProjectMemory>(`/projects/${projectId}/memory`, {
        method: 'PUT',
        body: JSON.stringify({ body }),
      }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: projectMemoryKey(projectId) }),
  });
}
