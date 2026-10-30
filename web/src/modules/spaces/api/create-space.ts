import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { Space } from '@/types/api';

type NewSpace = {
  projectId: number;
  workItemId?: number | null;
  name: string;
  team?: string;
  botId?: number;
  botIds?: number[];
  coordinatorBotId?: number;
  kind?: Space['kind'];
};

export const createSpace = (input: NewSpace) =>
  api<Space>('/spaces', { method: 'POST', body: JSON.stringify(input) });

export function useCreateSpace() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: createSpace,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['spaces'] }),
  });
}
