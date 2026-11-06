import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { InboxItem } from '@/types/api';

const dismiss = (item: InboxItem) =>
  api<void>('/inbox/dismiss', { method: 'POST', body: JSON.stringify(item) });

export function useDismissItem() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: dismiss,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['inbox'] }),
  });
}

export function useDismissItems() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (items: InboxItem[]) => Promise.all(items.map(dismiss)),
    onSettled: () => queryClient.invalidateQueries({ queryKey: ['inbox'] }),
  });
}
