import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { SpacePreview } from '@/types/api';

export const spacesQuery = {
  queryKey: ['spaces'],
  queryFn: () => api<SpacePreview[]>('/spaces'),
};

export function useSpaces() {
  return useQuery(spacesQuery);
}
