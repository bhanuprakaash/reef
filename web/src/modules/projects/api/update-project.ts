import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { Project } from '@/types/api';

export function useUpdateProject(project: Project) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (patch: { baseBranch?: string; purpose?: string }) =>
      api<Project>(`/projects/${project.id}`, {
        method: 'PUT',
        body: JSON.stringify({
          name: project.name,
          path: project.path,
          purpose: project.purpose,
          baseBranch: project.baseBranch,
          ...patch,
        }),
      }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['projects'] }),
  });
}
