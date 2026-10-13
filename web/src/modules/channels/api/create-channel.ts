import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { Channel } from '@/types/api';

type NewChannel = {
  projectId: number;
  featureId: number | null;
  name: string;
  team: string;
  backend: string;
  permissionMode: string;
};

export function useCreateChannel() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: NewChannel) =>
      api<Channel>('/channels', { method: 'POST', body: JSON.stringify(input) }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['channels'] }),
  });
}
