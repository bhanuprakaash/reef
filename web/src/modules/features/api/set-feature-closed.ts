import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';

export function useSetFeatureClosed(id: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (closed: boolean) =>
      api<void>(`/features/${id}/${closed ? 'close' : 'reopen'}`, { method: 'POST', body: '{}' }),
    onSuccess: () =>
      Promise.all([
        queryClient.invalidateQueries({ queryKey: ['features'] }),
        queryClient.invalidateQueries({ queryKey: ['channels'] }),
      ]),
  });
}
