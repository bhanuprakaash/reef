import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import { memoryKey } from '@/lib/query-keys';
import type { MemoryItem } from '@/types/api';

export function useAddMemory(scope: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: { kind: string; body: string }) =>
      api<MemoryItem>('/memory', { method: 'POST', body: JSON.stringify({ ...input, scope }) }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: memoryKey(scope) }),
  });
}
