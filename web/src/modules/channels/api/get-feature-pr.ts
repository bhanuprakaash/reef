import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import { featurePrKey } from '@/lib/query-keys';
import type { FeaturePR } from '@/types/api';

export function useFeaturePR(featureId: number) {
  return useQuery({
    queryKey: featurePrKey(featureId),
    queryFn: () => api<{ pr: FeaturePR | null }>(`/features/${featureId}/pr`),
    refetchInterval: 15000,
  });
}
