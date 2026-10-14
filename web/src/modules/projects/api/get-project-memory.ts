import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import { projectMemoryKey } from '@/lib/query-keys';
import type { ProjectMemory } from '@/types/api';

export function useProjectMemory(projectId: number) {
  return useQuery({
    queryKey: projectMemoryKey(projectId),
    queryFn: () => api<ProjectMemory>(`/projects/${projectId}/memory`),
  });
}
