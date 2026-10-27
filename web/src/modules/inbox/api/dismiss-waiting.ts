import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';

export function useDismissWaiting() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ spaceId, messageId }: { spaceId: number; messageId: number }) =>
      api<void>(`/spaces/${spaceId}/dismiss`, {
        method: 'POST',
        body: JSON.stringify({ messageId }),
      }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['inbox'] }),
  });
}
