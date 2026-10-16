import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { Project } from '@/types/api';

export function useUpdateProject(project: Project) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (baseBranch: string) =>
      api<Project>(`/projects/${project.id}`, {
        method: 'PUT',
        body: JSON.stringify({ name: project.name, path: project.path, baseBranch }),
      }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['projects'] }),
  });
}
