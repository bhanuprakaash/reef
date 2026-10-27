import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { InboxItem } from '@/types/api';

export function useDismissItem() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (item: InboxItem) =>
      api<void>('/inbox/dismiss', { method: 'POST', body: JSON.stringify(item) }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['inbox'] }),
  });
}
