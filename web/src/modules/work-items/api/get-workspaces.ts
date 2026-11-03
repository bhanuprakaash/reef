import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';

export type Workspace = {
  id: number;
  projectId: number;
  dir: string;
  state: string;
  snapshotRef: string;
  cleanupError: string;
};

export const workspacesKey = (workItemId: number) => ['work-items', workItemId, 'workspaces'];

// The list is empty while the work item has no workspace.
export function useWorkspaces(workItemId: number) {
  return useQuery({
    queryKey: workspacesKey(workItemId),
    queryFn: () => api<Workspace[]>(`/work-items/${workItemId}/workspaces`),
    refetchInterval: 5000,
  });
}
