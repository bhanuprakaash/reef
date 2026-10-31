import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { Project } from '@/types/api';

export function useSpaceProjects(spaceId: number) {
  return useQuery({
    queryKey: ['spaces', spaceId, 'projects'],
    queryFn: () => api<Project[]>(`/spaces/${spaceId}/projects`),
  });
}

export function useSetSpaceProject(spaceId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ projectId, associated }: { projectId: number; associated: boolean }) =>
      api<Project[]>(`/spaces/${spaceId}/projects/${projectId}`, {
        method: associated ? 'PUT' : 'DELETE',
        headers: { 'Content-Type': 'application/json' },
      }),
    onSuccess: () =>
      Promise.all([
        queryClient.invalidateQueries({ queryKey: ['spaces'] }),
        queryClient.invalidateQueries({ queryKey: ['messages', spaceId] }),
      ]),
  });
}
