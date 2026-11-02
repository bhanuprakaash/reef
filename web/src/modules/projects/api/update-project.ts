import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { Project } from '@/types/api';

export function useUpdateProject(project: Project) {
  const queryClient = useQueryClient();
  return useMutation({
    scope: { id: `project-${project.id}` },
    mutationFn: (patch: {
      baseBranch?: string;
      purpose?: string;
      icon?: string;
      colour?: string;
    }) => {
      const latest =
        queryClient.getQueryData<Project[]>(['projects'])?.find((p) => p.id === project.id) ??
        project;
      return api<Project>(`/projects/${project.id}`, {
        method: 'PUT',
        body: JSON.stringify({
          name: latest.name,
          path: latest.path,
          purpose: latest.purpose,
          baseBranch: latest.baseBranch,
          icon: latest.icon,
          colour: latest.colour,
          ...patch,
        }),
      });
    },
    onSuccess: (saved) => {
      queryClient.setQueryData<Project[]>(['projects'], (all) =>
        all?.map((p) => (p.id === saved.id ? saved : p)),
      );
      return queryClient.invalidateQueries({ queryKey: ['projects'] });
    },
  });
}
