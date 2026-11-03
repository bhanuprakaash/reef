import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { DetectedRemote, Project } from '@/types/api';

export function useDetectedRemote(projectId: number) {
  return useQuery({
    queryKey: ['project-remote', projectId],
    queryFn: () => api<DetectedRemote>(`/projects/${projectId}/remote`),
    retry: false,
  });
}

export function useBindRemote(projectId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: { connectionId: number; path: string }) =>
      api<Project>(`/projects/${projectId}/remote`, {
        method: 'PUT',
        body: JSON.stringify(input),
      }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['projects'] }),
  });
}
