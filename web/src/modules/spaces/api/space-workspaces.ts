import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';

export type SpaceWorkspace = {
  id: number;
  projectId: number;
  projectName: string;
  name: string;
  branch: string;
  base: string;
  baseSource: string;
  state: 'creating' | 'ready' | 'paused' | 'removed';
  note: string;
  head: string;
  dirty: boolean;
};

const key = (spaceId: number) => ['spaces', spaceId, 'workspaces'];

// The fixes of a conversation change while its Bot works, so the list is polled while open.
export function useSpaceWorkspaces(spaceId: number) {
  return useQuery({
    queryKey: key(spaceId),
    queryFn: () => api<SpaceWorkspace[]>(`/spaces/${spaceId}/workspaces`),
    refetchInterval: 5000,
  });
}

export function useAbandonSpaceWorkspace(spaceId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (workspaceId: number) =>
      api<void>(`/spaces/${spaceId}/workspaces/${workspaceId}/abandon`, { method: 'POST' }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: key(spaceId) }),
  });
}
