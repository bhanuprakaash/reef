import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api-client';
import type { Pin, PinKind, SpacePreview } from '@/types/api';

export const pinKindOf = (space: Pick<SpacePreview, 'kind'>): PinKind =>
  space.kind === 'conversation' ? 'chat' : 'space';

export function usePins() {
  return useQuery({
    queryKey: ['pins'],
    queryFn: () => api<Pin[]>('/pins'),
    refetchInterval: 5000,
  });
}

export function useTogglePin(kind: PinKind, id: number) {
  const queryClient = useQueryClient();
  const pinned = usePins().data?.some((p) => p.kind === kind && p.id === id) ?? false;
  const toggle = useMutation({
    mutationFn: () =>
      api<void>(
        `/pins/${kind}/${id}`,
        pinned ? { method: 'DELETE' } : { method: 'PUT', body: '{}' },
      ),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['pins'] }),
  });
  return { pinned, toggle };
}
