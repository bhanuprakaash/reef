import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import { featurePrKey } from '@/lib/query-keys';

export function useMergeFeaturePR(featureId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () =>
      api<void>(`/features/${featureId}/pr/merge`, {
        method: 'POST',
        body: JSON.stringify({}),
      }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: featurePrKey(featureId) }),
  });
}
