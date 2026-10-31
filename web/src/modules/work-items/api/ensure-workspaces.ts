import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import { workspacesKey, type Workspace } from '@/modules/work-items/api/get-workspaces';

// useEnsureWorkspaces makes the workspace of every target of the work item, the same way a first
// wake would. It runs only when the user asks for the terminal, never on page view.
export function useEnsureWorkspaces(workItemId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () =>
      api<Workspace[]>(`/work-items/${workItemId}/workspaces`, {
        method: 'POST',
        body: JSON.stringify({}),
      }),
    onSuccess: (data) => queryClient.setQueryData(workspacesKey(workItemId), data),
  });
}
