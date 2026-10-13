import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { Feature } from '@/types/api';

type NewFeature = { projectId: number; slug: string; name: string };

export function useCreateFeature() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: NewFeature) =>
      api<Feature>('/features', { method: 'POST', body: JSON.stringify(input) }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['features'] }),
  });
}
