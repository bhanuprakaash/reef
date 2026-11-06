import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';

export function useDeleteSpace(spaceId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => api<void>(`/spaces/${spaceId}`, { method: 'DELETE' }),
    // Not awaited: the row unmounts once the list refetches, and the caller's onSuccess is
    // skipped for an unmounted component.
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['spaces'] });
      void queryClient.invalidateQueries({ queryKey: ['pins'] });
    },
  });
}
