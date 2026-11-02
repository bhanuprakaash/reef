import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { Prompt } from '@/types/api';

export function usePrompts(spaceId: number) {
  return useQuery({
    queryKey: ['prompts', spaceId],
    queryFn: () => api<Prompt[]>(`/spaces/${spaceId}/prompts`),
  });
}
