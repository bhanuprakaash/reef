import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';

export function useSetWorkItemClosed(id: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (closed: boolean) =>
      api<void>(`/work-items/${id}/${closed ? 'close' : 'reopen'}`, { method: 'POST', body: '{}' }),
    onSuccess: () =>
      Promise.all([
        queryClient.invalidateQueries({ queryKey: ['work-items'] }),
        queryClient.invalidateQueries({ queryKey: ['spaces'] }),
      ]),
  });
}
