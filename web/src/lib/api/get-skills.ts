import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import { skillsKey } from '@/lib/query-keys';
import type { Skills } from '@/types/api';

export function skillsQuery(projectId?: number) {
  return {
    queryKey: skillsKey(projectId),
    queryFn: () =>
      api<Skills>(projectId === undefined ? '/skills' : `/skills?project=${projectId}`),
  };
}

export function useSkills(projectId: number | undefined) {
  return useQuery(skillsQuery(projectId));
}
