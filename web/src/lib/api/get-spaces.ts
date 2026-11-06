import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { SpacePreview } from '@/types/api';

export const spacesQuery = (projectId?: number) => ({
  queryKey: projectId === undefined ? ['spaces'] : ['spaces', 'project', projectId],
  queryFn: () =>
    api<SpacePreview[]>(projectId === undefined ? '/spaces' : `/spaces?project=${projectId}`),
});

export function useSpaces(projectId?: number) {
  return useQuery(spacesQuery(projectId));
}
