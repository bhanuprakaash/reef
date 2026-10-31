import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import { memoryRootKey } from '@/lib/query-keys';

export function useDecideMemory() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, decision }: { id: number; decision: 'approve' | 'reject' }) =>
      api<void>(`/memory/${id}/${decision}`, { method: 'POST', body: JSON.stringify({}) }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: memoryRootKey }),
  });
}
