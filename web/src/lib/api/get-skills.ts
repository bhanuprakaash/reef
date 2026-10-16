import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import { skillsKey } from '@/lib/query-keys';
import type { Skills } from '@/types/api';

export function useSkills(projectId: number | undefined) {
  return useQuery({
    queryKey: skillsKey(projectId ?? 0),
    queryFn: () => api<Skills>(`/skills?project=${projectId}`),
    enabled: projectId !== undefined,
  });
}
