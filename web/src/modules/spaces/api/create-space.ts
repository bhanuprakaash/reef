import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { Space } from '@/types/api';

type NewSpace = {
  projectId: number;
  featureId: number | null;
  name: string;
  team: string;
  backend: string;
  permissionMode: string;
};

export function useCreateSpace() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: NewSpace) =>
      api<Space>('/spaces', { method: 'POST', body: JSON.stringify(input) }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['spaces'] }),
  });
}
