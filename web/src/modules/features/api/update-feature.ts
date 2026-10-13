import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { Feature } from '@/types/api';

type FeatureEdit = { name: string; goal: string; stage: string };

export function useUpdateFeature(id: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: FeatureEdit) =>
      api<Feature>(`/features/${id}`, { method: 'PUT', body: JSON.stringify(input) }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['features'] }),
  });
}
